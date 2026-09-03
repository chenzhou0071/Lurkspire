// server.go — 网关 TCP 服务：监听/会话管理/优雅关闭
package gateway

import (
	"log"
	"net"
	"sync"
)

// Server 网关 TCP 服务器（单端口——客户端直连）
type Server struct {
	addr string
	hub  *Hub
	ln   net.Listener

	mu       sync.Mutex
	sessions map[uint32]*Session
	closed   bool
}

func NewServer(addr string, hub *Hub) *Server {
	return &Server{
		addr:     addr,
		hub:      hub,
		sessions: make(map[uint32]*Session),
	}
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

// Close 优雅关闭：停接受 + 断开全部会话
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
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
