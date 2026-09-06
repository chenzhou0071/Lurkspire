// session.go — 会话：连接 + 帧读循环 + 消息分发 + 安全写
package gateway

import (
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"lurkspire/server/internal/lobby"
	"lurkspire/server/internal/protocol"
	"lurkspire/server/internal/room"
)

// Session 一个客户端连接
type Session struct {
	conn net.Conn
	fr   *protocol.FrameReader
	hub  *Hub
	UID  uint32

	roomMu sync.Mutex // 保护 Room 字段（Join 写 / Leave 清 / 输入读）
	Room   *room.Room // 当前房间（nil = 未入房）

	// 登录态（M3：账号 uid + 昵称——未登录只能发登录/注册）
	LoginUID uint32
	Nickname string
	LoggedIn bool

	writeCh chan []byte // 写队列（广播/应答入队——写协程消费）
	// 满队丢旧帧：状态广播 30Hz 可丢不可卡（慢客户端只慢自己）
	closed    atomic.Bool
	closedCh  chan struct{} // 关闭通知（写协程退出——避免 close(writeCh) 与 Send 竞态）
	once      sync.Once
}

// room 安全读当前房间
func (s *Session) room() *room.Room {
	s.roomMu.Lock()
	defer s.roomMu.Unlock()
	return s.Room
}

// setRoom 安全写房间（入房）
func (s *Session) setRoom(r *room.Room) {
	s.roomMu.Lock()
	s.Room = r
	s.roomMu.Unlock()
}

// clearRoom 安全清房间（出房）
func (s *Session) clearRoom() {
	s.roomMu.Lock()
	s.Room = nil
	s.roomMu.Unlock()
}

// NewSession 启动读+写双协程
func NewSession(conn net.Conn, hub *Hub, uid uint32) *Session {
	s := &Session{
		conn:     conn,
		fr:       protocol.NewFrameReader(conn),
		hub:      hub,
		UID:      uid,
		writeCh:  make(chan []byte, 128),
		closedCh: make(chan struct{}),
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
	for {
		select {
		case b := <-s.writeCh:
			if _, err := s.conn.Write(b); err != nil {
				log.Printf("session %d write: %v", s.UID, err)
				return // 写失败 = 连接坏 → 触发关闭
			}
		case <-s.closedCh:
			return // 关闭通知：退出（writeCh 不 close——无 send 竞态）
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

// Close 关闭连接（幂等）：登出（出房+在线移除+好友离线推送） + 断开
func (s *Session) Close() {
	s.once.Do(func() {
		s.closed.Store(true)
		close(s.closedCh) // 写协程退出（writeCh 保持开放——无 send/close 竞态）
		s.hub.OnDisconnect(s)
		s.conn.Close()
	})
}

// handle 消息分发（会话 goroutine 内——房间操作全部走 channel 安全）
func (s *Session) handle(f *protocol.Frame) {
	switch f.MsgID {
	case protocol.MsgHeartbeat:
		s.Send(protocol.Encode(&protocol.Frame{MsgID: protocol.MsgHeartbeat, Seq: f.Seq}))
	case protocol.MsgReg:
		s.handleReg(f.Body)
	case protocol.MsgLogin:
		s.handleLogin(f.Body)
	default:
		// 其余消息需登录态（M3 大厅/房间全部业务在登录后）
		if !s.LoggedIn {
			s.sendErr(protocol.LobbyErrToken, "请先登录")
			return
		}
		s.handleAuthed(f.MsgID, f.Body)
	}
}

// handleAuthed 已登录消息分发（大厅/房间/背包）
func (s *Session) handleAuthed(msgID uint16, body []byte) {
	switch msgID {
	case protocol.MsgBattleJoin:
		roomName := string(body)
		if roomName == "" {
			s.sendErr(protocol.LobbyErrBadInput, "empty room name")
			return
		}
		// M3：房间玩家 = 账号 uid（登录后）
		r, states, err := s.hub.Join(s, roomName)
		if err != nil {
			s.sendErr(protocol.LobbyErrRoomFull, err.Error())
			return
		}
		ok := protocol.EncodeJoinOK(r.ID(), s.LoginUID, states)
		s.Send(protocol.Encode(&protocol.Frame{MsgID: protocol.MsgBattleJoinOK, Body: ok}))
	case protocol.MsgBattleInput:
		if s.room() == nil {
			return // 未入房不处理输入
		}
		s.Room.HandleInput(s.LoginUID, protocol.DecodeInput(body))
	case protocol.MsgRoomLeave:
		s.hub.Leave(s)
		s.Send(protocol.Encode(&protocol.Frame{MsgID: protocol.MsgRoomLeave}))
	default:
		// 大厅其他消息（好友/房间列表/背包）——T3-T5 实现
		s.hub.HandleLobby(s, msgID, body)
	}
}

// handleReg 注册（未登录可发）
func (s *Session) handleReg(body []byte) {
	account, password, nickname, ok := protocol.DecodeReg(body)
	if !ok {
		s.sendErr(protocol.LobbyErrBadInput, "bad register body")
		return
	}
	uid, err := s.hub.Lobby().Register(account, password, nickname)
	switch err {
	case nil:
		s.Send(protocol.Encode(&protocol.Frame{
			MsgID: protocol.MsgRegResp, Body: protocol.EncodeRegResp(protocol.LobbyOK, uid)}))
	case lobby.ErrAccountExists:
		s.sendErr(protocol.LobbyErrAccount, "账号已存在")
	case lobby.ErrNicknameExists:
		s.sendErr(protocol.LobbyErrNickname, "昵称已被使用")
	default:
		s.sendErr(protocol.LobbyErrBadInput, "注册失败")
	}
}

// handleLogin 登录（未登录可发）
func (s *Session) handleLogin(body []byte) {
	if s.LoggedIn {
		s.sendErr(protocol.LobbyErrBadInput, "已登录")
		return
	}
	account, password, ok := protocol.DecodeLogin(body)
	if !ok {
		s.sendErr(protocol.LobbyErrBadInput, "bad login body")
		return
	}
	token, err := s.hub.Lobby().Login(account, password)
	if err != nil {
		switch err {
		case lobby.ErrBadAccount:
			s.sendErr(protocol.LobbyErrNoAccount, "账号不存在")
		case lobby.ErrBadPassword:
			s.sendErr(protocol.LobbyErrPassword, "密码错误")
		default:
			s.sendErr(protocol.LobbyErrBadInput, "登录失败")
		}
		return
	}
	// 登录成功：记录会话身份（uid/昵称）
	uid, _ := s.hub.Lobby().Verify(token)
	nick, _ := s.hub.Lobby().Nickname(uid)
	s.LoginUID = uid
	s.Nickname = nick
	s.LoggedIn = true
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgLoginResp,
		Body:  protocol.EncodeLoginResp(protocol.LobbyOK, token, uid, nick),
	}))
	s.hub.OnLogin(s) // 好友系统：在线注册 + 上线推送 + 补发离线邀请 + 好友列表
}

func (s *Session) sendErr(errCode uint8, msg string) {
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgLoginResp,
		Body:  protocol.EncodeRegResp(errCode, 0),
	}))
}
