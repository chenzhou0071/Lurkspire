// session.go — 会话：连接 + 帧读循环 + 消息分发 + 安全写
package gateway

import (
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"lurkspire/server/internal/protocol"
	"lurkspire/server/internal/room"
)

// Session 一个客户端连接
type Session struct {
	conn net.Conn
	fr   *protocol.FrameReader
	hub  *Hub
	UID  uint32
	Room *room.Room // 当前房间（nil = 未入房）

	writeCh chan []byte // 写队列（广播/应答入队——写协程消费）
	// 满队丢旧帧：状态广播 30Hz 可丢不可卡（慢客户端只慢自己）
	closed atomic.Bool
	once   sync.Once
}

// NewSession 启动读+写双协程
func NewSession(conn net.Conn, hub *Hub, uid uint32) *Session {
	s := &Session{
		conn:    conn,
		fr:      protocol.NewFrameReader(conn),
		hub:     hub,
		UID:     uid,
		writeCh: make(chan []byte, 128),
	}
	go s.writeLoop() // 写协程：队列 → conn（阻塞只影响自己）
	return s
}

// Serve 阻塞读循环：直到连接断开（每帧分发）
func (s *Session) Serve() {
	defer s.Close()
	for {
		f, err := s.fr.Next()
		if err != nil {
			if err != io.EOF {
				log.Printf("session %d read: %v", s.UID, err)
			}
			return
		}
		s.handle(f)
	}
}

// writeLoop 写协程：取队列 → 写 conn（慢客户端只卡自己的写协程）
func (s *Session) writeLoop() {
	for b := range s.writeCh {
		if _, err := s.conn.Write(b); err != nil {
			log.Printf("session %d write: %v", s.UID, err)
			return // 写失败 = 连接坏 → 触发关闭
		}
	}
}

// Send 入队一帧（非阻塞——队列满丢旧帧，游戏帧可丢不可卡）
func (s *Session) Send(b []byte) {
	if s.closed.Load() {
		return
	}
	select {
	case s.writeCh <- b:
	default:
		// 队列满：丢弃最旧的广播帧（keep newest——30Hz 状态下旧帧无意义）
		select {
		case <-s.writeCh:
		default:
		}
		select {
		case s.writeCh <- b:
		default:
			s.Close() // 真满到写不进 = 对端死亡——断开
		}
	}
}

// Close 关闭连接（幂等）：出房 + 断开
func (s *Session) Close() {
	s.once.Do(func() {
		s.closed.Store(true)
		s.hub.Leave(s)
		close(s.writeCh) // 写协程退出
		s.conn.Close()
	})
}

// handle 消息分发（会话 goroutine 内——房间操作全部走 channel 安全）
func (s *Session) handle(f *protocol.Frame) {
	switch f.MsgID {
	case protocol.MsgHeartbeat:
		s.Send(protocol.Encode(&protocol.Frame{MsgID: protocol.MsgHeartbeat, Seq: f.Seq}))
	case protocol.MsgBattleJoin:
		// body = 房间名（UTF8 字节串）
		roomName := string(f.Body)
		if roomName == "" {
			s.sendError("empty room name")
			return
		}
		r, states, err := s.hub.Join(s, roomName)
		if err != nil {
			s.sendError(err.Error())
			return
		}
		// JoinOK：房间名 + 现有玩家状态
		ok := protocol.EncodeJoinOK(r.ID(), s.UID, states)
		s.Send(protocol.Encode(&protocol.Frame{MsgID: protocol.MsgBattleJoinOK, Body: ok}))
	case protocol.MsgBattleInput:
		if s.Room == nil {
			return // 未入房不处理输入
		}
		s.Room.HandleInput(s.UID, protocol.DecodeInput(f.Body))
	default:
		// 未知消息忽略（不做攻击面）
	}
}

func (s *Session) sendError(msg string) {
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgBattleErr,
		Body:  []byte(msg),
	}))
}
