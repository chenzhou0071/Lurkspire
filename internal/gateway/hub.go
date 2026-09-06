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
}

func NewHub(m *room.Manager, svc *lobby.Service) *Hub {
	return &Hub{
		manager:      m,
		lobby:        svc,
		roomSessions: make(map[*room.Room]map[uint32]*Session),
	}
}

// Lobby 账号服务（会话处理登录/大厅业务用）
func (h *Hub) Lobby() *lobby.Service { return h.lobby }

// HandleLobby 大厅业务消息（好友/房间列表/背包）——T3-T5 逐个接入
func (h *Hub) HandleLobby(s *Session, msgID uint16, body []byte) {
	// 未实现的消息静默忽略（T3-T5 填充）
	_ = msgID
	_ = body
	_ = s
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
	if err := r.AddPlayer(s.UID); err != nil {
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
	s.Room = r
	return r, r.StateSnapshot(), nil
}

// Leave 玩家离开：出房 + 注销路由
func (h *Hub) Leave(s *Session) {
	r := s.Room
	if r == nil {
		return
	}
	s.Room = nil
	r.RemovePlayer(s.UID)
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
