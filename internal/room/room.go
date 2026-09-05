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
	MaxPlayers  int
	TickHz      int
	Speed       float32 // T3 简化移动速度（T4 换成验证版）
	SpawnRange  float32 // 出生点随机范围
	SettleScore int     // 先到 N 分结算（0 = 关闭分数结算）
	SettleTicks int64   // 时长上限（tick 数；0 = 无限）
}

// Player 房间内玩家状态（与 protocol.PlayerState 同构，多服务器内部字段）
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
	Suspicious     int   // 非法移动次数（踢出候选）
	FirstReport    bool  // 首次上报（出生传送合法——跳过位移校验）
	// 战斗状态（服务端权威）
	Score       int     // 击杀数
	Deaths      int     // 死亡数（计分板）
	AimX, AimY  float32 // 准星方向角（度）
	Buttons     uint8   // 最近上报按钮（格挡/武器状态判定用）
	FireCd      int     // 射速冷却（tick 数）
	DeadTicks   int     // 死亡中剩余 tick（>0 = 等待重生）
	JustDied    bool    // 本帧刚死（广播 Death 后清除）
	LockCharges int     // 锁头存量（10s 一发，存 3）
	LockTimer   float32 // 锁头充能计时（秒）
	LockCd      int     // 锁头发射冷却（tick——防按钮多帧重复触发）
	SwordCd     int     // 挥砍冷却（0.25s=8 tick——防左键多帧连发秒杀）
	DashCd      int     // 冲刺斩冷却（1.5s=45 tick——防 Shift 多帧连发）
}

// Blocking 是否格挡姿态（按钮格挡位按下且条足够——供命中结算）
func (p *Player) Blocking() bool {
	return p.Buttons&protocol.BtnBlock != 0 && p.Block >= CombatBlockMin
}

// Dead 是否死亡等待重生
func (p *Player) Dead() bool { return p.DeadTicks > 0 }

// Snapshot 转协议状态（广播用）
func (p *Player) Snapshot() protocol.PlayerState {
	return protocol.PlayerState{
		UID: p.UID, X: p.X, Y: p.Y, Z: p.Z, Yaw: p.Yaw,
		HP: p.HP, Weapon: p.Weapon, Alt: p.Alt, Block: p.Block, Anim: p.Anim,
		Score: uint16(p.Score), Deaths: uint16(p.Deaths),
	}
}

// ---- 房间 ----

type inputMsg struct {
	uid  uint32
	in   protocol.InputReport
	snap chan []protocol.PlayerState // 非 nil = 快照请求（复用输入 channel 串行化）
	drain chan []protocol.HitEvent   // 非 nil = 事件取走请求
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
	combat  *Combat

	inputCh chan inputMsg
	joinCh  chan joinMsg
	leaveCh chan leaveMsg
	brCh    chan func(msgID uint16, body []byte)

	tick          int64
	pendingEvents []protocol.HitEvent // 本帧命中事件（T6 广播）
	settled       bool                // 结算已广播（锁存不重复）

	// 广播回调（T6 网关推送用；nil = 无消费者——单测直接读 pendingEvents）
	onBroadcast func(msgID uint16, body []byte)
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
		brCh:    make(chan func(msgID uint16, body []byte), 4),
	}
	r.combat = NewCombat(r.players, MapWalls)
	go r.loop()
	return r
}

// Stop 停止循环（房间销毁）
func (r *Room) Stop() { close(r.stop) }

// ID 房间名
func (r *Room) ID() string { return r.id }

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

// SetBroadcast 注册广播回调（网关推送；单测可留空）
func (r *Room) SetBroadcast(fn func(msgID uint16, body []byte)) {
	r.brCh <- fn
}

// DrainEvents 取走本帧命中/死亡事件（T6 广播源；loop 外调用走 channel）
func (r *Room) DrainEvents() []protocol.HitEvent {
	ch := make(chan []protocol.HitEvent, 1)
	r.inputCh <- inputMsg{uid: 0, in: protocol.InputReport{}, drain: ch}
	return <-ch
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
			r.combat.TickCombat() // 冷却/充能/重生推进
			if r.onBroadcast != nil {
				r.broadcastTick() // 状态 + 事件 + 结算检查（30Hz）
			}
		case m := <-r.inputCh:
			r.handleInputMsg(m)
		case m := <-r.joinCh:
			m.result <- r.addPlayer(m.uid)
		case m := <-r.leaveCh:
			delete(r.players, m.uid)
		case fn := <-r.brCh:
			r.onBroadcast = fn
		}
	}
}

// broadcastTick 每帧推送：State（30Hz）→ 命中事件 → 击杀（Death）→ 结算检查
func (r *Room) broadcastTick() {
	// 状态广播（全部玩家）
	states := make([]protocol.PlayerState, 0, len(r.players))
	for _, p := range r.players {
		states = append(states, p.Snapshot())
	}
	r.onBroadcast(protocol.MsgBattleState, protocol.EncodeState(states))
	// 命中事件（逐条）
	for _, ev := range r.pendingEvents {
		r.onBroadcast(protocol.MsgBattleHit, protocol.EncodeHit(&ev))
		// 击杀广播：本帧刚死的目标（死亡确认/计分提示）
		if t := r.players[ev.Target]; t != nil && t.JustDied {
			r.onBroadcast(protocol.MsgBattleDeath, protocol.EncodeHit(&ev))
			t.JustDied = false
		}
	}
	r.pendingEvents = nil
	// 结算检查（分数/时长——锁存只广播一次）
	if !r.settled {
		if r.settleReason() != "" {
			r.settled = true
			entries := make([]protocol.SettleEntry, 0, len(r.players))
			for _, p := range r.players {
				entries = append(entries, protocol.SettleEntry{UID: p.UID, Score: uint16(p.Score)})
			}
			r.onBroadcast(protocol.MsgBattleSettle, protocol.EncodeSettle(entries))
		}
	}
}

// settleReason 返回结算原因（"" = 未到结算条件）
func (r *Room) settleReason() string {
	if r.cfg.SettleScore > 0 {
		for _, p := range r.players {
			if p.Score >= r.cfg.SettleScore {
				return "score"
			}
		}
	}
	if r.cfg.SettleTicks > 0 && r.tick >= r.cfg.SettleTicks {
		return "time"
	}
	return ""
}

func (r *Room) addPlayer(uid uint32) error {
	if _, ok := r.players[uid]; ok {
		return nil // 重复加入：幂等
	}
	if len(r.players) >= r.cfg.MaxPlayers {
		return ErrRoomFull
	}
	// 出生点：随机出生位（含高层平台——复活不贴脸）
	sp := PickSpawn()
	r.players[uid] = &Player{UID: uid, HP: 100, Block: 100,
		X: sp.X, Y: sp.Y, Z: sp.Z, LastReportTick: r.tick, FirstReport: true}
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
	if m.drain != nil {
		m.drain <- r.pendingEvents
		r.pendingEvents = nil
		return
	}
	p := r.players[m.uid]
	if p == nil {
		return
	}
	p.LastReportTick = r.tick
	// 服务端验证：非法输入不更新位置，连续非法标记（踢出候选）
	ok := CheckMove(p, m.in)
	MarkViolation(p, ok)
	if !ok {
		return
	}
	// 记录战斗相关输入（瞄准角/按钮——命中判定用）
	p.AimX = m.in.AimX
	p.AimY = m.in.AimY
	p.Buttons = m.in.Buttons
	// 武器/动作（广播给其他玩家显示——RemotePlayer 渲染）
	p.Weapon = m.in.Weapon
	p.Anim = m.in.Anim
	p.ApplyInput(m.in, 1.0/float32(r.cfg.TickHz))
	// 武器动作分发（服务端权威命中——事件收集待广播）
	var events []protocol.HitEvent
	switch {
	case m.in.Buttons&protocol.BtnFire != 0:
		events = r.combat.ApplyShot(p)
	case m.in.Buttons&protocol.BtnSword != 0:
		events = r.combat.ApplySword(p)
	case m.in.Buttons&protocol.BtnDashAtk != 0:
		events = r.combat.ApplyDash(p)
	}
	if m.in.Buttons&protocol.BtnLock != 0 {
		events = append(events, r.combat.ApplyLock(p)...)
	}
	if len(events) > 0 {
		r.pendingEvents = append(r.pendingEvents, events...)
	}
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
