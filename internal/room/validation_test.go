// validation_test.go — 移动验证：值域/浮点/连续非法/速度模式/斜向归一化
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
	ok := CheckMove(p, protocol.InputReport{MoveX: 1, MoveY: -1, Yaw: 0.5, AimX: 10, AimY: -5})
	if !ok {
		t.Fatal("normal input should pass")
	}
}

func TestCheckMove_Overspeed_Rejected(t *testing.T) {
	p := newTestPlayer()
	// MoveX=2 → 试图 2 倍速 → 拒绝（位置不更新）
	if CheckMove(p, protocol.InputReport{MoveX: 2, MoveY: 0}) {
		t.Fatal("MoveX=2 should be rejected")
	}
	if CheckMove(p, protocol.InputReport{MoveX: 0, MoveY: -127}) {
		t.Fatal("MoveY=-127 should be rejected")
	}
}

func TestCheckMove_NaN_Rejected(t *testing.T) {
	p := newTestPlayer()
	if CheckMove(p, protocol.InputReport{MoveX: 1, MoveY: 0, Yaw: float32(math.NaN())}) {
		t.Fatal("NaN yaw should be rejected")
	}
	if CheckMove(p, protocol.InputReport{MoveX: 1, MoveY: 0, AimX: float32(math.Inf(1))}) {
		t.Fatal("Inf aim should be rejected")
	}
}

func TestMarkViolation_ThreeStrikes_Suspicious(t *testing.T) {
	p := newTestPlayer()
	// 连续 3 次非法 → 踢出候选
	MarkViolation(p, false)
	MarkViolation(p, false)
	if IsSuspicious(p) {
		t.Fatal("2 strikes should not be suspicious yet")
	}
	MarkViolation(p, false)
	if !IsSuspicious(p) {
		t.Fatal("3 strikes should be suspicious")
	}
	// 合法输入清零
	MarkViolation(p, true)
	if IsSuspicious(p) {
		t.Fatal("legal input should reset suspicious")
	}
}

func TestApplyInput_SpeedModes(t *testing.T) {
	dt := float32(1) // 1 秒方便断言
	cases := []struct {
		name    string
		buttons uint8
		want    float32
	}{
		{"run", 0, RunSpeed},
		{"slide", protocol.BtnSlide, SlideSpeed},
		{"wallrun", protocol.BtnWallRun, WallRunSpeed},
	}
	for _, c := range cases {
		p := newTestPlayer()
		p.ApplyInput(protocol.InputReport{MoveX: 0, MoveY: 1, Buttons: c.buttons}, dt)
		if math.Abs(float64(p.Z)) < float64(c.want)-0.01 || math.Abs(float64(p.Z)) > float64(c.want)+0.01 {
			t.Fatalf("%s: want z≈%v, got %v", c.name, c.want, p.Z)
		}
	}
}

func TestApplyInput_Diagonal_Normalized(t *testing.T) {
	p := newTestPlayer()
	dt := float32(1)
	// 斜向 (1,1)：每轴 0.707×speed（斜走不快）
	p.ApplyInput(protocol.InputReport{MoveX: 1, MoveY: 1}, dt)
	wantAxis := RunSpeed / float32(math.Sqrt2)
	if math.Abs(float64(p.X)-float64(wantAxis)) > 0.01 || math.Abs(float64(p.Z)-float64(wantAxis)) > 0.01 {
		t.Fatalf("diagonal: want each axis ≈%v, got x=%v z=%v", wantAxis, p.X, p.Z)
	}
}

func TestApplyInput_YawRotatesMove(t *testing.T) {
	// 面向 +X（yaw=90°）按 W（MoveY=1）→ 应朝 +X 移动（不是 +Z）
	p := newTestPlayer()
	p.Yaw = 90
	p.ApplyInput(protocol.InputReport{MoveX: 0, MoveY: 1}, 1)
	if p.X < RunSpeed-0.1 || p.Z > 0.1 {
		t.Fatalf("yaw=90 W should move +X: x=%v z=%v", p.X, p.Z)
	}
}

func TestRoom_HandleInput_Illegal_NotMoved(t *testing.T) {
	r := NewRoom("rv", testConfig())
	defer r.Stop()
	r.AddPlayer(1)
	before := waitSnapshot(r, 1)[0]

	// 非法输入（MoveX=2 超速）30 帧 → 位置不变
	for i := 0; i < 30; i++ {
		r.HandleInput(1, protocol.InputReport{MoveX: 2, MoveY: 0})
		time.Sleep(10 * time.Millisecond)
	}
	after := waitSnapshot(r, 1)[0]
	if after.X != before.X || after.Z != before.Z {
		t.Fatalf("illegal input must not move: before=%v,%v after=%v,%v",
			before.X, before.Z, after.X, after.Z)
	}
}
