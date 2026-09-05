// validation_test.go — 移动验证：值域/浮点/位移合理性/首报例外
package room

import (
	"math"
	"testing"
	"time"

	"lurkspire/server/internal/protocol"
)

func newTestPlayer() *Player { return &Player{UID: 1, HP: 100} }

func TestCheckMove_NormalInput_Passes(t *testing.T) {
	p := newTestPlayer()
	p.FirstReport = false
	p.X, p.Y, p.Z = 0, 0, 0
	ok := CheckMove(p, protocol.InputReport{MoveX: 1, MoveY: -1, Yaw: 0.5,
		AimX: 10, AimY: -5, X: 0.5, Y: 0, Z: 0.5}) // 小位移（正常走一步）
	if !ok {
		t.Fatal("normal input should pass")
	}
}

func TestCheckMove_Overspeed_Rejected(t *testing.T) {
	p := newTestPlayer()
	p.FirstReport = false
	if CheckMove(p, protocol.InputReport{MoveX: 2, MoveY: 0}) {
		t.Fatal("MoveX=2 should be rejected")
	}
	if CheckMove(p, protocol.InputReport{MoveX: 0, MoveY: -127}) {
		t.Fatal("MoveY=-127 should be rejected")
	}
}

func TestCheckMove_Teleport_Rejected(t *testing.T) {
	p := newTestPlayer()
	p.FirstReport = false
	p.X, p.Y, p.Z = 0, 0, 0
	// 单帧位移 10m（瞬移）→ 拒绝
	if CheckMove(p, protocol.InputReport{X: 10, Y: 0, Z: 0}) {
		t.Fatal("10m teleport should be rejected")
	}
	// 垂直瞬移（飞 5m）→ 拒绝
	if CheckMove(p, protocol.InputReport{X: 0, Y: 5, Z: 0}) {
		t.Fatal("5m vertical jump should be rejected")
	}
}

func TestCheckMove_FirstReport_Skips(t *testing.T) {
	p := newTestPlayer()
	p.FirstReport = true // 刚出生：传送合法
	p.X, p.Y, p.Z = 100, 0, 100
	// 出生点瞬移任意远都过（客户端没收到过服务端位置——首次对齐）
	if !CheckMove(p, protocol.InputReport{X: 300, Y: 32, Z: 55}) {
		t.Fatal("first report teleport should pass")
	}
}

func TestCheckMove_NaN_Rejected(t *testing.T) {
	p := newTestPlayer()
	if CheckMove(p, protocol.InputReport{MoveX: 1, MoveY: 0, Yaw: float32(math.NaN())}) {
		t.Fatal("NaN yaw should be rejected")
	}
	if CheckMove(p, protocol.InputReport{X: float32(math.Inf(1))}) {
		t.Fatal("Inf position should be rejected")
	}
}

func TestMarkViolation_ThreeStrikes_Suspicious(t *testing.T) {
	p := newTestPlayer()
	MarkViolation(p, false)
	MarkViolation(p, false)
	if IsSuspicious(p) {
		t.Fatal("2 strikes should not be suspicious yet")
	}
	MarkViolation(p, false)
	if !IsSuspicious(p) {
		t.Fatal("3 strikes should be suspicious")
	}
	MarkViolation(p, true)
	if IsSuspicious(p) {
		t.Fatal("legal input should reset suspicious")
	}
}

func TestApplyInput_AdoptsPosition(t *testing.T) {
	p := newTestPlayer()
	p.ApplyInput(protocol.InputReport{X: 123, Y: 5, Z: -40, Yaw: 90}, 0)
	if p.X != 123 || p.Y != 5 || p.Z != -40 || p.Yaw != 90 {
		t.Fatalf("ApplyInput should adopt reported position: %+v", p)
	}
	if p.FirstReport {
		t.Fatal("FirstReport should be cleared after first adopt")
	}
}

func TestRoom_HandleInput_AdoptsReportedPos(t *testing.T) {
	r := NewRoom("rv", testConfig())
	defer r.Stop()
	r.AddPlayer(1)
	waitSnapshot(r, 1)

	// 上报移动后的位置 → 服务端采纳（不再自己积分）
	for i := 0; i < 10; i++ {
		pos := float32(1 + i) // 每帧走 1m（合法步长内）
		r.HandleInput(1, protocol.InputReport{MoveX: 0, MoveY: 1, X: pos, Y: 0, Z: pos})
		time.Sleep(15 * time.Millisecond)
	}
	after := waitSnapshot(r, 1)[0]
	if after.X != 10 || after.Z != 10 {
		t.Fatalf("room should adopt reported pos: %+v", after)
	}
}

func TestRoom_HandleInput_Illegal_NotMoved(t *testing.T) {
	r := NewRoom("rv", testConfig())
	defer r.Stop()
	r.AddPlayer(1)
	waitSnapshot(r, 1)

	// 非法瞬移（一次跳 100m）30 次 → 位置不变
	before := waitSnapshot(r, 1)[0]
	r.players[1].FirstReport = false // 消费首报例外（出生首报合法传送）
	for i := 0; i < 30; i++ {
		r.HandleInput(1, protocol.InputReport{MoveX: 0, MoveY: 0, X: 100, Y: 0, Z: 100})
		time.Sleep(10 * time.Millisecond)
	}
	after := waitSnapshot(r, 1)[0]
	if after.X != before.X || after.Z != before.Z {
		t.Fatalf("illegal teleport must not move: before=%v,%v after=%v,%v",
			before.X, before.Z, after.X, after.Z)
	}
}
