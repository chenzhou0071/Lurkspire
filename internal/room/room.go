// room.go — 房间核心：4-8 人 30Hz 对局循环（单写者 goroutine + channel 输入）
package room

import (
	"errors"
	"sync"
	"time"

	"lurkspire/server/internal/protocol"
)

var ErrRoomFull = errors.New("room: full")

// Config 房间配置（M2 固定值；M3 可由匹配系统注入）
type Config struct {
	MaxPlayers int
	TickHz     int
	Speed      float32 // T3 简化移动速度（T4 换成验证版）
	SpawnRange float32 // 出生点随机范围
}

// Player 房间内玩家状态（与 protocol.PlayerState 同构，多两个服务器内部字段）
type Player struct {
	UID            uint32
	X, Y, Z        float32
	Yaw            float32
	HP             uint8
	Weapon         uint8
	Alt            uint8
	Block          float32
	Anim           uint8
	LastReportTick int64 // 最后上报帧（超时判定用）
	Suspicious     int   // 非法移动次数（T4 验证用）
}

// Snapshot 转协议状态（广播用）
func (p *Player) Snapshot() protocol.PlayerState {
	return protocol.PlayerState{
		UID: p.UID, X: p.X, Y: p.Y, Z: p.Z, Yaw: p.Yaw,
		HP: p.HP, Weapon: p.Weapon, Alt: p.Alt, Block: p.Block, Anim: p.Anim,
	}
}

// ---- 房间 ----

type inputMsg struct {
	uid  uint32
	in   protocol.InputReport
	snap chan []protocol.PlayerState // 非 nil = 快照请求（复用输入 channel 串行化）
}

type joinMsg struct {
	uid    uint32
	result chan error
}

type leaveMsg struct{ uid uint32 }

// Room 单写者：所有状态变更都在 loop goroutine 内（channel 串行化，免锁）
type Room struct {
	id   string
	cfg  Config
	stop chan struct{}

	players map[uint32]*Player

	inputCh chan inputMsg
	joinCh  chan joinMsg
	leaveCh chan leaveMsg

	tick int64
}

func NewRoom(id string, cfg Config) *Room {
	if cfg.MaxPlayers <= 0 || cfg.MaxPlayers > 8 {
		cfg.MaxPlayers = 8
	}
	if cfg.TickHz <= 0 {
		cfg.TickHz = 30
	}
	r := &Room{
		id:      id,
		cfg:     cfg,
		stop:    make(chan struct{}),
		players: make(map[uint32]*Player),
		inputCh: make(chan inputMsg, 64),
		joinCh:  make(chan joinMsg),
		leaveCh: make(chan leaveMsg),
	}
	go r.loop()
	return r
}

// Stop 停止循环（房间销毁）
func (r *Room) Stop() { close(r.stop) }

// AddPlayer 入房（满员拒绝）；阻塞等待 loop 处理结果
func (r *Room) AddPlayer(uid uint32) error {
	ch := make(chan error, 1)
	r.joinCh <- joinMsg{uid: uid, result: ch}
	return <-ch
}

// RemovePlayer 出房
func (r *Room) RemovePlayer(uid uint32) {
	r.leaveCh <- leaveMsg{uid: uid}
}

// HandleInput 客户端输入上报（非阻塞入队；房间满队列时丢弃——TCP 下不可能）
func (r *Room) HandleInput(uid uint32, in protocol.InputReport) {
	select {
	case r.inputCh <- inputMsg{uid: uid, in: in}:
	default:
	}
}

// StateSnapshot 当前全部玩家状态（广播用——任何 goroutine 可安全调用）
func (r *Room) StateSnapshot() []protocol.PlayerState {
	snapCh := make(chan []protocol.PlayerState, 1)
	r.inputCh <- inputMsg{uid: 0, in: protocol.InputReport{}, snap: snapCh}
	return <-snapCh
}

// loop 30Hz 主循环
func (r *Room) loop() {
	ticker := time.NewTicker(time.Second / time.Duration(r.cfg.TickHz))
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.tick++
		case m := <-r.inputCh:
			r.handleInputMsg(m)
		case m := <-r.joinCh:
			m.result <- r.addPlayer(m.uid)
		case m := <-r.leaveCh:
			delete(r.players, m.uid)
		}
	}
}

func (r *Room) addPlayer(uid uint32) error {
	if _, ok := r.players[uid]; ok {
		return nil // 重复加入：幂等
	}
	if len(r.players) >= r.cfg.MaxPlayers {
		return ErrRoomFull
	}
	// 出生点：随机散布在出生圈内（T3 简化：固定出生点由客户端决定，M2 用圆心）
	r.players[uid] = &Player{UID: uid, HP: 100, Block: 100, LastReportTick: r.tick}
	return nil
}

func (r *Room) handleInputMsg(m inputMsg) {
	if m.snap != nil {
		states := make([]protocol.PlayerState, 0, len(r.players))
		for _, p := range r.players {
			states = append(states, p.Snapshot())
		}
		m.snap <- states
		return
	}
	p := r.players[m.uid]
	if p == nil {
		return
	}
	p.LastReportTick = r.tick
	// T3 简化：直接按输入移动（T4 换成服务端验证版）
	dirX := float32(m.in.MoveX)
	dirZ := float32(m.in.MoveY)
	dt := 1.0 / float32(r.cfg.TickHz)
	p.X += dirX * r.cfg.Speed * dt
	p.Z += dirZ * r.cfg.Speed * dt
	p.Yaw = m.in.Yaw
}

// ---- 管理器 ----

// Manager 建房/找房（容量 8；M2 内存态，M3 挂 MySQL）
type Manager struct {
	mu    sync.Mutex
	rooms map[string]*Room
	cfg   Config
}

func NewManager(cfg Config) *Manager {
	return &Manager{rooms: make(map[string]*Room), cfg: cfg}
}

// CreateRoom 建房；同名已存在返回现有房（幂等）
func (m *Manager) CreateRoom(id string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rooms[id]; ok {
		return r, nil
	}
	r := NewRoom(id, m.cfg)
	m.rooms[id] = r
	return r, nil
}

// GetRoom 找房
func (m *Manager) GetRoom(id string) (*Room, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[id]
	return r, ok
}

// DestroyRoom 销毁（Stop 循环）
func (m *Manager) DestroyRoom(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rooms[id]; ok {
		r.Stop()
		delete(m.rooms, id)
	}
}
