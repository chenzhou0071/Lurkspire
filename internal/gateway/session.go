// session.go — 会话：连接 + 帧读循环 + 消息分发 + 安全写
package gateway

import (
	"io"
	"log"
	"net"
	"sync"

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

	mu        sync.Mutex // 写锁（防并发写 conn）
	closeOnce sync.Once
	done      chan struct{}
}

func NewSession(conn net.Conn, hub *Hub, uid uint32) *Session {
	return &Session{
		conn: conn,
		fr:   protocol.NewFrameReader(conn),
		hub:  hub,
		UID:  uid,
		done: make(chan struct{}),
	}
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

// Send 安全写一帧（并发安全）
func (s *Session) Send(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return // 已关闭
	default:
	}
	if _, err := s.conn.Write(b); err != nil {
		log.Printf("session %d write: %v", s.UID, err)
	}
}

// Close 关闭连接（幂等）：出房 + 断开
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.hub.Leave(s)
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
