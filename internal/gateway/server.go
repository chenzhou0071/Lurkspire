// server.go — 网关 TCP 服务：监听/会话管理/心跳超时踢出/优雅关闭
package gateway

import (
	"log"
	"net"
	"sync"
	"time"
)

// Server 网关 TCP 服务器（单端口——客户端直连）
type Server struct {
	addr string
	hub  *Hub
	ln   net.Listener

	mu       sync.Mutex
	sessions map[uint32]*Session
	closed   bool

	// 心跳超时（无任何帧 > idleTimeout → 踢出；watchInterval 为扫描周期）
	idleTimeout   time.Duration
	watchInterval time.Duration
	watchStop     chan struct{}
}

func NewServer(addr string, hub *Hub) *Server {
	return &Server{
		addr:          addr,
		hub:           hub,
		sessions:      make(map[uint32]*Session),
		idleTimeout:   15 * time.Second, // 客户端 5s 心跳——15s 无帧 = 断网/僵尸
		watchInterval: 5 * time.Second,
		watchStop:     make(chan struct{}),
	}
}

// SetIdleTimeout 心跳超时与扫描周期（测试用短值）
func (s *Server) SetIdleTimeout(idle, interval time.Duration) {
	s.idleTimeout = idle
	s.watchInterval = interval
}

// Listen 开始监听（addr 支持 127.0.0.1:0 随机端口——测试用）
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	log.Printf("gateway listening on %s", ln.Addr())
	return nil
}

// Addr 实际监听地址（测试拿端口）
func (s *Server) Addr() net.Addr {
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Serve 接受连接循环（阻塞——业务进程在 goroutine 调用）
func (s *Server) Serve() {
	go s.watchdog() // 心跳超时踢出（僵局/断网连接清理——防占位）
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return // 优雅关闭：停止接受
			}
			log.Printf("accept: %v", err)
			continue
		}
		uid := s.hub.AllocUID()
		ss := NewSession(conn, s.hub, uid)
		s.mu.Lock()
		s.sessions[uid] = ss
		s.mu.Unlock()
		go func() {
			defer func() {
				s.mu.Lock()
				delete(s.sessions, uid)
				s.mu.Unlock()
			}()
			ss.Serve()
		}()
	}
}

// watchdog 心跳超时踢出：无任何帧超过 idleTimeout → 断开（触发宽限离线）
func (s *Server) watchdog() {
	ticker := time.NewTicker(s.watchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.watchStop:
			return
		case <-ticker.C:
			now := time.Now().UnixNano()
			limit := int64(s.idleTimeout)
			s.mu.Lock()
			var stale []*Session
			for _, ss := range s.sessions {
				if now-ss.LastActive() > limit {
					stale = append(stale, ss)
				}
			}
			s.mu.Unlock()
			for _, ss := range stale {
				log.Printf("session %d idle timeout (no frame for %v) — close", ss.UID, s.idleTimeout)
				ss.Close()
			}
		}
	}
}

// Close 优雅关闭：停接受 + 断开全部会话
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.watchStop) // 停 watchdog
	s.ln.Close()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, ss := range s.sessions {
		sessions = append(sessions, ss)
	}
	s.mu.Unlock()
	for _, ss := range sessions {
		ss.Close()
	}
}
