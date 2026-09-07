// rooms.go — 房间列表大厅：创建/加入/离开/列表广播
// 元信息（创建者/备注）hub 持有；人数实时取自房间会话表；变更广播全大厅
package gateway

import (
	"sync"

	"lurkspire/server/internal/protocol"
)

// RoomMeta 房间元信息（列表展示用）
type RoomMeta struct {
	Name    string
	Creator uint32 // 创建者账号 uid（昵称展示时查）
	Note    string
}

// rooms 房间注册表（hub 内嵌）
type rooms struct {
	mu    sync.Mutex
	metas map[string]*RoomMeta // 活跃房间（与 room.Manager 同步：空房销毁时移除）
}

func newRooms() *rooms {
	return &rooms{metas: make(map[string]*RoomMeta)}
}

// add 注册（已有返回 false）
func (r *rooms) add(meta *RoomMeta) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.metas[meta.Name]; ok {
		return false
	}
	r.metas[meta.Name] = meta
	return true
}

// remove 注销（出房/空房销毁）
func (r *rooms) remove(name string) {
	r.mu.Lock()
	delete(r.metas, name)
	r.mu.Unlock()
}

// has 是否存在
func (r *rooms) has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.metas[name]
	return ok
}

// list 全部房间（名称快照——组装时动态取人数）
func (r *rooms) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.metas))
	for name := range r.metas {
		out = append(out, name)
	}
	return out
}

// ---- Hub 集成 ----

// broadcastRoomList 房间列表推送给所有大厅在线用户（变更时调用）
func (h *Hub) broadcastRoomList() {
	body := h.buildRoomListBody()
	h.mu.Lock()
	targets := make([]*Session, 0, len(h.online))
	for _, s := range h.online {
		// 在房间里的不推（对局中不需要列表——可简化：全推）
		targets = append(targets, s)
	}
	h.mu.Unlock()
	frame := protocol.Encode(&protocol.Frame{MsgID: protocol.MsgRoomList, Body: body})
	for _, s := range targets {
		s.Send(frame)
	}
}

// SendRoomList 给单个会话下发房间列表（登录时）
func (h *Hub) SendRoomList(s *Session) {
	s.Send(protocol.Encode(&protocol.Frame{
		MsgID: protocol.MsgRoomList,
		Body:  h.buildRoomListBody(),
	}))
}

// buildRoomListBody 组装房间列表（名称/创建者昵称/备注/人数）
func (h *Hub) buildRoomListBody() []byte {
	names := h.roomRegistry.list()
	list := make([]protocol.RoomInfo, 0, len(names))
	for _, name := range names {
		h.roomRegistry.mu.Lock()
		meta := h.roomRegistry.metas[name]
		h.roomRegistry.mu.Unlock()
		if meta == nil {
			continue
		}
		creatorNick, _ := h.lobby.Nickname(meta.Creator)
		// 人数：roomSessions 中该房间的会话数
		r, _ := h.manager.GetRoom(name)
		players := 0
		if r != nil {
			h.mu.Lock()
			players = len(h.roomSessions[r])
			h.mu.Unlock()
		}
		list = append(list, protocol.RoomInfo{
			Name: name, Creator: creatorNick, Note: meta.Note,
			Players: players, Max: 8,
		})
	}
	return protocol.EncodeRoomList(list)
}
