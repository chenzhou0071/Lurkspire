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
	// 好友系统（T3：申请栏模式——申请持久化在 store，不在线也保留）
	online map[uint32]*Session // 登录态 uid → 会话（在线表）
	// 房间列表大厅（T4）
	roomRegistry *rooms // 房间元信息（创建者/备注）
}

func NewHub(m *room.Manager, svc *lobby.Service) *Hub {
	return &Hub{
		manager:      m,
		lobby:        svc,
		roomSessions: make(map[*room.Room]map[uint32]*Session),
		online:       make(map[uint32]*Session),
		roomRegistry: newRooms(),
	}
}

// Lobby 账号服务（会话处理登录/大厅业务用）
func (h *Hub) Lobby() *lobby.Service { return h.lobby }

// OnLogin 登录成功：注册在线 + 好友上线推送 + 下发申请栏（待处理申请）+ 好友列表
func (h *Hub) OnLogin(s *Session) {
	h.mu.Lock()
	first := h.online[s.LoginUID] == nil
	h.online[s.LoginUID] = s
	h.mu.Unlock()

	if first {
		h.broadcastOnline(s.LoginUID, true) // 好友看到我上线
	}
	h.SendPendingList(s) // 申请栏：我的待处理申请（别人申请我——离线也保留）
	h.SendFriendList(s)  // 好友列表
	h.SendRoomList(s)    // 房间列表（大厅）
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

// SendPendingList 下发申请栏（别人申请我、未处理的——uid/昵称/在线）
func (h *Hub) SendPendingList(s *Session) {
	pending, err := h.lobby.PendingFriendIDs(s.LoginUID)
	if err != nil {
		return
	}
	list := make([]protocol.FriendInfo, 0, len(pending))
	for _, p := range pending {
		nick, err := h.lobby.Nickname(p)
		if err != nil {
			continue
		}
		list = append(list, protocol.FriendInfo{UID: p, Nickname: nick, Online: h.sessionOf(p) != nil})
	}
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgFriendPendingList,
		Body:  protocol.EncodeFriendList(list),
	}))
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

// HandleLobby 大厅业务消息（好友/房间/背包）——T3 好友 T4 房间 T5 背包
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
	case protocol.MsgRoomList:
		h.SendRoomList(s)
	case protocol.MsgRoomCreate:
		h.handleRoomCreate(s, body)
	case protocol.MsgRoomJoin:
		h.handleRoomJoin(s, body)
	case protocol.MsgBagList:
		h.SendBagList(s)
	case protocol.MsgBagEquip:
		h.handleBagEquip(s, body)
	}
}

// ---- 背包（T5） ----

// SendBagList 下发背包（当前装备 + 物品表）
func (h *Hub) SendBagList(s *Session) {
	cur, _ := h.lobby.EquipID(s.LoginUID)
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgBagList,
		Body:  protocol.EncodeBagList(cur, protocol.BagItems),
	}))
}

// 切换装备（0-4 合法——只能一件；持久化 + 若在房立即生效）
func (h *Hub) handleBagEquip(s *Session, body []byte) {
	equip, ok := protocol.DecodeBagEquip(body)
	if !ok || equip < 0 || equip >= len(protocol.BagItems) {
		s.sendErr(protocol.LobbyErrEquip, "非法装备")
		return
	}
	if err := h.lobby.SetEquip(s.LoginUID, equip); err != nil {
		s.sendErr(protocol.LobbyErrBadInput, "装备失败")
		return
	}
	// 若在房间 → 立即应用（格挡上限）
	if r := s.room(); r != nil {
		r.SetPlayerEquip(s.LoginUID, equip)
	}
	h.SendBagList(s)
}

// ---- 房间列表大厅（T4） ----

// sendJoinOK 入房成功应答（房间名 + 账号 uid + 现有玩家状态）——建房/加入/直连共用
func (h *Hub) sendJoinOK(s *Session, r *room.Room) {
	states := r.StateSnapshot()
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgBattleJoinOK,
		Body:  protocol.EncodeJoinOK(r.ID(), s.LoginUID, states),
	}))
}

// 创建房间（同房名拒绝）：建房 + 注册元信息 + 自己入房 + 广播列表
func (h *Hub) handleRoomCreate(s *Session, body []byte) {
	if s.room() != nil {
		s.sendErr(protocol.LobbyErrBadInput, "已在房间中")
		return
	}
	name, note, ok := protocol.DecodeRoomOp(body)
	if !ok || name == "" || len(name) > 32 || len(note) > 64 {
		s.sendErr(protocol.LobbyErrBadInput, "房间名不合法")
		return
	}
	if h.roomRegistry.has(name) {
		s.sendErr(protocol.LobbyErrRoomExists, "房间名已存在")
		return
	}
	meta := &RoomMeta{Name: name, Creator: s.LoginUID, Note: note}
	if !h.roomRegistry.add(meta) {
		s.sendErr(protocol.LobbyErrRoomExists, "房间名已存在")
		return
	}
	// 入房（room.Manager 建房幂等——meta 已注册）
	r, _, err := h.Join(s, name)
	if err != nil {
		h.roomRegistry.remove(name)
		s.sendErr(protocol.LobbyErrRoomFull, err.Error())
		return
	}
	h.sendJoinOK(s, r) // 建房成功 → 进对局
	h.broadcastRoomList() // 列表变更广播
}

// 加入房间（不存在/满员拒绝）：入房 + 广播列表
func (h *Hub) handleRoomJoin(s *Session, body []byte) {
	if s.room() != nil {
		s.sendErr(protocol.LobbyErrBadInput, "已在房间中")
		return
	}
	name, _, ok := protocol.DecodeRoomOp(body)
	if !ok || name == "" {
		s.sendErr(protocol.LobbyErrBadInput, "bad join")
		return
	}
	if !h.roomRegistry.has(name) {
		s.sendErr(protocol.LobbyErrRoomMissing, "房间不存在")
		return
	}
	r, _, err := h.Join(s, name)
	if err != nil {
		s.sendErr(protocol.LobbyErrRoomFull, "房间已满")
		return
	}
	h.sendJoinOK(s, r) // 加入成功 → 进对局
	h.broadcastRoomList() // 人数变化广播
}

// LeaveRoom 出房（MsgRoomLeave 用——Leave 内部处理空房销毁/meta/广播）
func (h *Hub) LeaveRoom(s *Session) {
	h.Leave(s)
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

// 发申请（落库持久——目标在线实时推送加栏；离线登录时申请栏带出）
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
	if err := h.lobby.InviteFriend(s.LoginUID, target); err != nil {
		s.sendErr(protocol.LobbyErrBadInput, "申请失败")
		return
	}
	// 目标在线 → 实时推送申请条目（加栏）
	if ts := h.sessionOf(target); ts != nil {
		myNick, _ := h.lobby.Nickname(s.LoginUID)
		ts.Send(protocol.Encode(&protocol.Frame{
			MsgID: protocol.MsgFriendInviteN,
			Body:  protocol.EncodeFriendInfo(s.LoginUID, myNick, true),
		}))
	}
	// 离线：无需操作——申请已落库，目标登录时 SendPendingList 带出
}

// 同意：状态转好友 + 双方刷新列表（申请条目由客户端移除）
func (h *Hub) handleFriendAccept(s *Session, body []byte) {
	other, ok := protocol.DecodeUID(body)
	if !ok {
		s.sendErr(protocol.LobbyErrBadInput, "bad accept")
		return
	}
	if err := h.lobby.AcceptFriend(s.LoginUID, other); err != nil {
		s.sendErr(protocol.LobbyErrBadInput, "accept failed")
		return
	}
	h.SendFriendList(s) // 我刷新（申请栏条目客户端本地移除）
	if os := h.sessionOf(other); os != nil {
		h.SendFriendList(os) // 对方刷新
	}
}

// 拒绝：删除申请（条目消失）
func (h *Hub) handleFriendReject(s *Session, body []byte) {
	other, ok := protocol.DecodeUID(body)
	if !ok {
		return
	}
	h.lobby.RejectFriend(s.LoginUID, other)
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
	// 应用玩家装备（装备 4 → 格挡上限 110——房间权威）
	if eid, err := h.lobby.EquipID(s.LoginUID); err == nil {
		r.SetPlayerEquip(s.LoginUID, eid)
	}
	return r, r.StateSnapshot(), nil
}

// Leave 玩家离开：出房 + 空房销毁（同步元信息移除）
func (h *Hub) Leave(s *Session) {
	r := s.room()
	if r == nil {
		return
	}
	roomName := r.ID()
	s.clearRoom()
	r.RemovePlayer(s.LoginUID)
	h.mu.Lock()
	if set, ok := h.roomSessions[r]; ok {
		delete(set, s.UID)
		if len(set) == 0 {
			delete(h.roomSessions, r)
			h.roomRegistry.remove(roomName) // 空房销毁：元信息同步移除
			h.manager.DestroyRoom(roomName)
		}
	}
	h.mu.Unlock()
	h.broadcastRoomList() // 人数/列表变更广播
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
