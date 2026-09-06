// hub.go — 房间分配中心（单进程：内嵌 room.Manager）
// 职责：uid 分配 / Join（同房间名加入/建房）/ 广播路由（房间 → 会话）
package gateway

import (
	"errors"
	"sync"

	"lurkspire/server/internal/lobby"
	"lurkspire/server/internal/protocol"
	"lurkspire/server/internal/room"
)

var (
	ErrRoomFull   = errors.New("gateway: room full")
	ErrInvalidMsg = errors.New("gateway: invalid message")
)

// Hub 会话与房间的中心登记簿
type Hub struct {
	mu      sync.Mutex
	seq     uint32
	manager *room.Manager
	lobby   *lobby.Service
	// 房间 → 会话集合（广播路由用）
	roomSessions map[*room.Room]map[uint32]*Session
	// 好友系统（T3）
	online       map[uint32]*Session                // 登录态 uid → 会话（在线表）
	pendingInv   map[uint32][]protocol.FriendInfo   // 离线邀请（目标登录时推送）
}

func NewHub(m *room.Manager, svc *lobby.Service) *Hub {
	return &Hub{
		manager:      m,
		lobby:        svc,
		roomSessions: make(map[*room.Room]map[uint32]*Session),
		online:       make(map[uint32]*Session),
		pendingInv:   make(map[uint32][]protocol.FriendInfo),
	}
}

// Lobby 账号服务（会话处理登录/大厅业务用）
func (h *Hub) Lobby() *lobby.Service { return h.lobby }

// OnLogin 登录成功：注册在线 + 好友上线推送 + 补发离线邀请 + 下发好友列表
func (h *Hub) OnLogin(s *Session) {
	h.mu.Lock()
	first := h.online[s.LoginUID] == nil
	h.online[s.LoginUID] = s
	// 取离线邀请（登录时补发）
	var invites []protocol.FriendInfo
	if v, ok := h.pendingInv[s.LoginUID]; ok {
		invites = v
		delete(h.pendingInv, s.LoginUID)
	}
	h.mu.Unlock()

	if first {
		h.broadcastOnline(s.LoginUID, true) // 好友看到我上线
	}
	for _, inv := range invites {
		s.Send(protocol.Encode(&protocol.Frame{
			MsgID: protocol.MsgFriendInviteN,
			Body:  protocol.EncodeFriendInfo(inv.UID, inv.Nickname, false),
		}))
	}
	h.SendFriendList(s)
}

// OnDisconnect 连接断开：出房 + 在线移除 + 好友离线推送
func (h *Hub) OnDisconnect(s *Session) {
	h.Leave(s) // 出房（若在房）
	h.mu.Lock()
	if s.LoggedIn {
		delete(h.online, s.LoginUID)
		h.mu.Unlock()
		h.broadcastOnline(s.LoginUID, false)
		return
	}
	h.mu.Unlock()
}

// broadcastOnline 在线状态推送给双方好友（uid 上线/下线）
func (h *Hub) broadcastOnline(uid uint32, online bool) {
	friends, err := h.lobby.FriendIDs(uid)
	if err != nil {
		return
	}
	body := protocol.EncodeOnlinePush(uid, online)
	for _, f := range friends {
		if s := h.sessionOf(f); s != nil {
			s.Send(protocol.Encode(&protocol.Frame{MsgID: protocol.MsgFriendOnline, Body: body}))
		}
	}
}

// sessionOf 在线会话（锁外使用——查在线表）
func (h *Hub) sessionOf(uid uint32) *Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.online[uid]
}

// SendFriendList 下发某用户的好友列表（uid/昵称/在线）
func (h *Hub) SendFriendList(s *Session) {
	friends, err := h.lobby.FriendIDs(s.LoginUID)
	if err != nil {
		return
	}
	list := make([]protocol.FriendInfo, 0, len(friends))
	for _, f := range friends {
		nick, err := h.lobby.Nickname(f)
		if err != nil {
			continue
		}
		list = append(list, protocol.FriendInfo{UID: f, Nickname: nick, Online: h.sessionOf(f) != nil})
	}
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgFriendList,
		Body:  protocol.EncodeFriendList(list),
	}))
}

// HandleLobby 大厅业务消息（好友/房间列表/背包）——T3 好友，T4-T5 续
func (h *Hub) HandleLobby(s *Session, msgID uint16, body []byte) {
	switch msgID {
	case protocol.MsgFriendSearch:
		h.handleFriendSearch(s, body)
	case protocol.MsgFriendInvite:
		h.handleFriendInvite(s, body)
	case protocol.MsgFriendAccept:
		h.handleFriendAccept(s, body)
	case protocol.MsgFriendReject:
		h.handleFriendReject(s, body)
	case protocol.MsgFriendList:
		h.SendFriendList(s)
	}
}

// 好友搜索（按昵称——昵称唯一）
func (h *Hub) handleFriendSearch(s *Session, body []byte) {
	nickname, _, ok := protocol.DecodeRoomOp(body) // 复用字符串解码（name=昵称）
	if !ok || nickname == "" {
		s.sendErr(protocol.LobbyErrBadInput, "bad search")
		return
	}
	u, err := h.lobby.SearchByNickname(nickname)
	if err != nil {
		s.sendErr(protocol.LobbyErrNotFriend, "找不到该昵称")
		return
	}
	if u.ID == s.LoginUID {
		s.sendErr(protocol.LobbyErrSelf, "不能搜索自己")
		return
	}
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgFriendSearchR,
		Body:  protocol.EncodeFriendInfo(u.ID, u.Nickname, h.sessionOf(u.ID) != nil),
	}))
}

// 发邀请（目标在线 → 推送；离线 → 排队待登录补发）
func (h *Hub) handleFriendInvite(s *Session, body []byte) {
	target, ok := protocol.DecodeUID(body)
	if !ok || target == s.LoginUID {
		s.sendErr(protocol.LobbyErrSelf, "不能邀请自己")
		return
	}
	// 已是好友 → 拒绝
	friends, _ := h.lobby.FriendIDs(s.LoginUID)
	for _, f := range friends {
		if f == target {
			s.sendErr(protocol.LobbyErrNotFriend, "已是好友")
			return
		}
	}
	// 自己昵称（邀请方）
	myNick, _ := h.lobby.Nickname(s.LoginUID)
	info := protocol.FriendInfo{UID: s.LoginUID, Nickname: myNick, Online: true}
	if ts := h.sessionOf(target); ts != nil {
		ts.Send(protocol.Encode(&protocol.Frame{
			MsgID: protocol.MsgFriendInviteN,
			Body:  protocol.EncodeFriendInfo(info.UID, info.Nickname, true),
		}))
		return
	}
	// 离线：排队（对方登录时推送）
	h.mu.Lock()
	h.pendingInv[target] = append(h.pendingInv[target], info)
	h.mu.Unlock()
}

// 同意：建双向关系 + 双方刷新列表
func (h *Hub) handleFriendAccept(s *Session, body []byte) {
	other, ok := protocol.DecodeUID(body)
	if !ok {
		s.sendErr(protocol.LobbyErrBadInput, "bad accept")
		return
	}
	if err := h.lobby.AddFriend(s.LoginUID, other); err != nil {
		s.sendErr(protocol.LobbyErrBadInput, "add friend failed")
		return
	}
	// 清 pending（对方邀请行）
	h.mu.Lock()
	if v, ok := h.pendingInv[s.LoginUID]; ok {
		filtered := v[:0]
		for _, inv := range v {
			if inv.UID != other {
				filtered = append(filtered, inv)
			}
		}
		if len(filtered) == 0 {
			delete(h.pendingInv, s.LoginUID)
		} else {
			h.pendingInv[s.LoginUID] = filtered
		}
	}
	h.mu.Unlock()
	h.SendFriendList(s)                       // 我刷新
	if os := h.sessionOf(other); os != nil {
		h.SendFriendList(os)                  // 对方刷新
	}
}

// 拒绝：仅清 pending（不建关系）
func (h *Hub) handleFriendReject(s *Session, body []byte) {
	other, ok := protocol.DecodeUID(body)
	if !ok {
		return
	}
	h.mu.Lock()
	if v, ok := h.pendingInv[s.LoginUID]; ok {
		filtered := v[:0]
		for _, inv := range v {
			if inv.UID != other {
				filtered = append(filtered, inv)
			}
		}
		if len(filtered) == 0 {
			delete(h.pendingInv, s.LoginUID)
		} else {
			h.pendingInv[s.LoginUID] = filtered
		}
	}
	h.mu.Unlock()
}

// AllocUID 分配玩家 ID（M2 无登录——连接即分配）
func (h *Hub) AllocUID() uint32 {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	return h.seq
}

// Join 同房间名加入：有房加入/无房建房；满员拒绝
// 返回房间（成功时）+ 现有玩家状态（JoinOK 用）
func (h *Hub) Join(s *Session, roomName string) (*room.Room, []protocol.PlayerState, error) {
	r, err := h.manager.CreateRoom(roomName)
	if err != nil {
		return nil, nil, err
	}
	// 先入房（阻塞等房间 loop 处理——不持 hub 锁，防锁序死锁）
	// 房间玩家 = 账号 uid（登录后身份——Leave/广播一致）
	if err := r.AddPlayer(s.LoginUID); err != nil {
		return nil, nil, err
	}
	// 入房成功 → 注册路由（短锁）
	h.mu.Lock()
	if h.roomSessions[r] == nil {
		h.roomSessions[r] = make(map[uint32]*Session)
		// 房间广播 → 路由给同房间会话（单进程内闭环）
		r.SetBroadcast(func(msgID uint16, body []byte) {
			h.broadcastToRoom(r, msgID, body)
		})
	}
	h.roomSessions[r][s.UID] = s
	h.mu.Unlock()
	s.setRoom(r) // 安全写房间（与 Leave/输入并发安全）
	return r, r.StateSnapshot(), nil
}

// Leave 玩家离开：出房 + 注销路由
func (h *Hub) Leave(s *Session) {
	r := s.room()
	if r == nil {
		return
	}
	s.clearRoom()
	r.RemovePlayer(s.LoginUID)
	h.mu.Lock()
	if set, ok := h.roomSessions[r]; ok {
		delete(set, s.UID)
		if len(set) == 0 {
			delete(h.roomSessions, r)
			h.manager.DestroyRoom(r.ID())
		}
	}
	h.mu.Unlock()
}

// broadcastToRoom 房间广播 → 房间内全部会话（Hub 内部：调用时已在房间 loop 内）
func (h *Hub) broadcastToRoom(r *room.Room, msgID uint16, body []byte) {
	// 快照会话集合（短锁后释放——不持锁写 conn）
	var targets []*Session
	h.mu.Lock()
	for _, s := range h.roomSessions[r] {
		targets = append(targets, s)
	}
	h.mu.Unlock()
	for _, s := range targets {
		s.Send(protocol.Encode(&protocol.Frame{MsgID: msgID, Body: body}))
	}
}
