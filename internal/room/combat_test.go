// combat_test.go — 权威命中：枪射线/墙挡/刀扇形/格挡/锁头必中/击杀重生
package room

import (
	"math"
	"testing"

	"lurkspire/server/internal/protocol"
)

// 测试墙：一堵 20 宽的墙在 x=30（中心）——挡射线用
var testWalls = []WallBox{
	{X: 30, Y: 2, Z: 0, W: 0.5, H: 4, D: 20},
}

func newCombatPair() (*Combat, *Player, *Player) {
	players := map[uint32]*Player{
		1: {UID: 1, X: 0, Y: 0, Z: 0, HP: 100, AimY: 0},
		2: {UID: 2, X: 0, Y: 0, Z: 20, HP: 100},
	}
	return NewCombat(players, testWalls), players[1], players[2]
}

func TestApplyShot_HitsNearestInRange(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p1.AimX = 0 // 朝 +Z
	events := c.ApplyShot(p1)
	if len(events) != 1 {
		t.Fatalf("want 1 hit, got %d", len(events))
	}
	if events[0].Target != 2 || events[0].Damage != GunDamage {
		t.Fatalf("wrong target/damage: %+v", events[0])
	}
	if p2.HP != 100-GunDamage {
		t.Fatalf("hp after hit: want %d, got %d", 100-GunDamage, p2.HP)
	}
}

func TestApplyShot_WallBlocks(t *testing.T) {
	c, p1, p2 := newCombatPair()
	// 射手在墙后（x=-10 朝 +Z 打），目标在墙另一边——墙 x=30 挡不住？重排：
	p1.X = 10 // 射手在墙 x=30 前方
	_ = p2
	// 目标放墙后 x=40
	target := c.players[2]
	target.X = 40
	p1.AimX = 0
	events := c.ApplyShot(p1)
	if len(events) != 0 {
		t.Fatalf("wall should block, got %d events: %+v", len(events), events)
	}
}

func TestApplyShot_NoTarget_Empty(t *testing.T) {
	c, p1, _ := newCombatPair()
	c.players[2].Z = 200 // 目标超射程
	p1.AimX = 0
	if events := c.ApplyShot(p1); len(events) != 0 {
		t.Fatalf("want empty, got %+v", events)
	}
}

func TestApplySword_ArcHitsFront(t *testing.T) {
	c, p1, p2 := newCombatPair()
	// p2 在射手正前方 2m（面向 +Z）
	p2.X = 0
	p2.Z = 2
	p1.AimX = 0
	events := c.ApplySword(p1)
	if len(events) != 1 || events[0].Damage != SwordDamage {
		t.Fatalf("sword front hit fail: %+v", events)
	}
}

func TestApplySword_BehindMisses(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.X = 0
	p2.Z = -2 // 在背后
	p1.AimX = 0
	if events := c.ApplySword(p1); len(events) != 0 {
		t.Fatalf("behind should miss: %+v", events)
	}
}

func TestBlock_ReducesDamage(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.Buttons = protocol.BtnBlock // 格挡姿态
	p2.Block = 100
	p1.AimX = 0
	events := c.ApplyShot(p1)
	if len(events) != 1 {
		t.Fatal("hit should still land")
	}
	// 减伤 50%：25 → 12（uint8 截断 12.5 → 12）
	if events[0].Damage != 12 {
		t.Fatalf("blocked damage: want 12, got %d", events[0].Damage)
	}
	if p2.Block != 90 {
		t.Fatalf("block cost: want 90, got %v", p2.Block)
	}
}

func TestBlock_TooLow_NoBlock(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.Buttons = protocol.BtnBlock
	p2.Block = 5 // 条 <10 不触发格挡
	p1.AimX = 0
	events := c.ApplyShot(p1)
	if events[0].Damage != GunDamage {
		t.Fatalf("no block: want full %d, got %d", GunDamage, events[0].Damage)
	}
}

func TestKill_ScoreAndDeath(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.HP = 20 // 一枪带走（25 ≥ 20）
	p1.AimX = 0
	events := c.ApplyShot(p1)
	if len(events) != 1 {
		t.Fatal("kill hit missing")
	}
	if p1.Score != 1 {
		t.Fatalf("score: want 1, got %d", p1.Score)
	}
	if !p2.Dead() {
		t.Fatal("target should be dead")
	}
	// 死亡后不能被再次命中
	if events2 := c.ApplyShot(p1); len(events2) != 0 {
		t.Fatal("dead target must not be hit")
	}
}

func TestLock_HitsNearest_IgnoreWalls(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p1.LockCharges = 1
	// 目标在墙后 45m（墙 x=30 中间挡着——但锁头无视墙）
	p2.X = 0
	p2.Z = 45
	events := c.ApplyLock(p1)
	if len(events) != 1 || events[0].Damage != LockDamage {
		t.Fatalf("lock should hit through wall: %+v", events)
	}
	if p1.LockCharges != 0 {
		t.Fatal("lock charge should be consumed")
	}
}

func TestLock_NoCharge_Nothing(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.Z = 5
	if events := c.ApplyLock(p1); len(events) != 0 {
		t.Fatalf("no charge: want empty, got %+v", events)
	}
}

func TestLock_Cooldown_NoDoubleShot(t *testing.T) {
	c, p1, _ := newCombatPair()
	p1.LockCharges = 3
	// 第一发（按钮多帧模拟——连续调 5 次）
	if events := c.ApplyLock(p1); len(events) != 1 {
		t.Fatal("first lock should hit")
	}
	for i := 0; i < 5; i++ {
		if events := c.ApplyLock(p1); len(events) != 0 {
			t.Fatalf("lock cooldown violated at %d: %+v", i, events)
		}
	}
	if p1.LockCharges != 2 {
		t.Fatalf("only 1 charge consumed: want 2 left, got %d", p1.LockCharges)
	}
	// 冷却结束后可再发
	for i := 0; i < 30; i++ {
		c.TickCombat()
	}
	if events := c.ApplyLock(p1); len(events) != 1 {
		t.Fatal("lock should fire after cooldown")
	}
}

func TestSword_Cooldown_NoMultiHit(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.X = 0
	p2.Z = 2 // 正前方 2m
	p1.AimX = 0
	// 挥砍一次 + 多帧连按（左键按住模拟）
	if events := c.ApplySword(p1); len(events) != 1 {
		t.Fatal("first swing should hit")
	}
	hpAfterFirst := p2.HP // 50 伤后
	for i := 0; i < 5; i++ {
		if events := c.ApplySword(p1); len(events) != 0 {
			t.Fatalf("sword cooldown violated at %d: %+v", i, events)
		}
	}
	if p2.HP != hpAfterFirst {
		t.Fatalf("multi-frame sword must not repeat damage: hp=%d", p2.HP)
	}
}

func TestDash_Cooldown_NoMultiHit(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.X = 0
	p2.Z = 5 // 路径内
	p1.AimX = 0
	if events := c.ApplyDash(p1); len(events) != 1 {
		t.Fatal("first dash should hit")
	}
	for i := 0; i < 5; i++ {
		if events := c.ApplyDash(p1); len(events) != 0 {
			t.Fatalf("dash cooldown violated at %d: %+v", i, events)
		}
	}
	if p2.HP != 50 {
		t.Fatalf("multi-frame dash must not repeat damage: hp=%d", p2.HP)
	}
}

func TestRespawn_AfterTicks(t *testing.T) {
	c, p1, p2 := newCombatPair()
	p2.HP = 10
	p1.AimX = 0
	c.ApplyShot(p1) // 击杀
	if !p2.Dead() {
		t.Fatal("should be dead")
	}
	// 推进重生计时（60 tick）
	for i := 0; i < RespawnTicks; i++ {
		c.TickCombat()
	}
	if p2.Dead() {
		t.Fatal("should be respawned")
	}
	if p2.HP != 100 {
		t.Fatalf("respawn hp: want 100, got %d", p2.HP)
	}
}

func TestDirFromAngles_StraightAhead(t *testing.T) {
	dx, dy, dz := DirFromAngles(0, 0) // 朝 +Z
	if math.Abs(float64(dx)) > 0.01 || math.Abs(float64(dy)) > 0.01 || math.Abs(float64(dz-1)) > 0.01 {
		t.Fatalf("straight ahead wrong: %v %v %v", dx, dy, dz)
	}
}
