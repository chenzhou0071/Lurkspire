// broadcast_test.go — 广播：State 30Hz / Hit / Death / Settle（分数+时长锁存）
package room

import (
	"sync"
	"testing"
	"time"

	"lurkspire/server/internal/protocol"
)

// 广播收集器（记录 msgID → 次数/内容）
type collector struct {
	mu     sync.Mutex
	state  int
	hits   int
	death  int
	settle int
	last   map[uint16][][]byte
}

func newCollector() *collector {
	return &collector{last: make(map[uint16][][]byte)}
}

func (c *collector) fn(msgID uint16, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last[msgID] = append(c.last[msgID], body)
	switch msgID {
	case protocol.MsgBattleState:
		c.state++
	case protocol.MsgBattleHit:
		c.hits++
	case protocol.MsgBattleDeath:
		c.death++
	case protocol.MsgBattleSettle:
		c.settle++
	}
}

func (c *collector) counts() (int, int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state, c.hits, c.death, c.settle
}

// broadcastConfig：短结算值（分数 20 / 时长 60 tick）——测试用
func broadcastConfig() Config {
	return Config{MaxPlayers: 8, TickHz: 30, Speed: 16, SettleScore: 20, SettleTicks: 0}
}

func TestBroadcast_StateFlows(t *testing.T) {
	r := NewRoom("b1", broadcastConfig())
	defer r.Stop()
	col := newCollector()
	r.SetBroadcast(col.fn)
	r.AddPlayer(1)
	r.AddPlayer(2)
	// 等 2 帧以上广播
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		s, _, _, _ := col.counts()
		if s >= 2 {
			return // 收到 30Hz 状态流 ✓
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no state broadcast received")
}

func TestBroadcast_KillEmitsHitAndDeath(t *testing.T) {
	r := NewRoom("b2", broadcastConfig())
	defer r.Stop()
	col := newCollector()
	r.SetBroadcast(col.fn)
	r.AddPlayer(1)
	r.AddPlayer(2)
	// 定位玩家并摆位：1 在 (0,0,0) 朝 +Z；2 在 (0,0,10) 残血 20
	waitSnapshot(r, 2)
	r.combat.walls = nil // 测试场景无墙（真实 MapWalls 会挡射线）
	p1 := r.players[1]
	p2 := r.players[2]
	p1.X, p1.Y, p1.Z = 0, 0, 0 // 出生点含高层平台——摆位必须连 Y 一起固定
	p2.X, p2.Y, p2.Z = 0, 0, 10
	p2.HP = 20
	p1.AimX, p1.AimY = 0, 0
	p1.FirstReport = false // 已就位（跳过首报例外——正常校验）
	// 开火（上报带位置——否则采纳会把摆位重置）
	r.HandleInput(1, protocol.InputReport{MoveX: 0, MoveY: 0, AimX: 0, AimY: 0, Buttons: protocol.BtnFire, X: 0, Y: 0, Z: 0})
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		_, hits, deaths, _ := col.counts()
		if deaths >= 1 {
			return // 击杀广播到达 ✓
		}
		if hits >= 1 {
			// 等死亡广播（同一帧 tick 推送）
			time.Sleep(50 * time.Millisecond)
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, hits, deaths, _ := col.counts()
	t.Fatalf("kill broadcast missing: hits=%d deaths=%d", hits, deaths)
}

func TestBroadcast_SettleOnScore_Once(t *testing.T) {
	r := NewRoom("b3", broadcastConfig())
	defer r.Stop()
	col := newCollector()
	r.SetBroadcast(col.fn)
	r.AddPlayer(1)
	r.AddPlayer(2)
	waitSnapshot(r, 2)
	r.combat.walls = nil // 测试场景无墙
	// 直接打到 20 分
	r.players[1].Score = 19
	p1 := r.players[1]
	p2 := r.players[2]
	p1.X, p1.Y, p1.Z = 0, 0, 0 // 出生点含高层平台——摆位必须连 Y 一起固定
	p2.X, p2.Y, p2.Z = 0, 0, 10
	p2.HP = 10
	p1.AimX, p1.AimY = 0, 0
	p1.FirstReport = false
	r.HandleInput(1, protocol.InputReport{MoveX: 0, MoveY: 0, AimX: 0, AimY: 0, Buttons: protocol.BtnFire, X: 0, Y: 0, Z: 0})
	// 等结算广播
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		_, _, _, settle := col.counts()
		if settle >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 再等一会儿确认不重复
	time.Sleep(200 * time.Millisecond)
	_, _, _, settle := col.counts()
	if settle != 1 {
		t.Fatalf("settle must broadcast exactly once, got %d", settle)
	}
	// 结算内容含玩家分数
	col.mu.Lock()
	settleBodies := col.last[protocol.MsgBattleSettle]
	col.mu.Unlock()
	if len(settleBodies) == 0 {
		t.Fatal("no settle body")
	}
	entries := protocol.DecodeSettle(settleBodies[len(settleBodies)-1])
	if len(entries) != 2 {
		t.Fatalf("settle entries: want 2, got %+v", entries)
	}
	// map 遍历无序——按 UID 查找断言
	scoreOf := func(uid uint32) uint16 {
		for _, e := range entries {
			if e.UID == uid {
				return e.Score
			}
		}
		return 0xFFFF
	}
	if scoreOf(1) != 20 {
		t.Fatalf("winner(uid1) score wrong: %+v", entries)
	}
	if scoreOf(2) != 0 {
		t.Fatalf("loser(uid2) score wrong: %+v", entries)
	}
}

func TestBroadcast_SettleOnTimeLimit(t *testing.T) {
	cfg := broadcastConfig()
	cfg.SettleTicks = 5 // 5 tick 就超时结算
	r := NewRoom("b4", cfg)
	defer r.Stop()
	col := newCollector()
	r.SetBroadcast(col.fn)
	r.AddPlayer(1)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		_, _, _, settle := col.counts()
		if settle >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("time-limit settle not reached")
}
