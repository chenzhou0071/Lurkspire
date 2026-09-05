// validation.go — 移动验证（服务端权威）：客户端上报本地位置（含全部机动）
// 服务端不积分——验证合理性后采纳：单帧位移超上限 = 瞬移/超速 → 拒绝
// 校验：输入值域 / 浮点 NaN·Inf / 位移合理性 / 连续非法标记 suspicious
package room

import (
	"math"

	"lurkspire/server/internal/protocol"
)

// 服务端位移上限（30Hz 单帧，容差内允许）：
// 水平最快 = 跑墙 25.6 → 0.85m/帧；垂直 = 下落上限 60 → 2.0m/帧
// 容差 2 倍（网络抖动/帧率波动）+ 1m 绝对余量（出生/传送用例外另行处理）
const (
	MaxHorzStep = 0.85 * 2 // 单帧水平位移上限 ~1.7m
	MaxVertStep = 2.0 * 2  // 单帧垂直位移上限 ~4m（下落冲刺保留）
	MaxAbsSpeed = 60       // 任何方向瞬时速度上限（防滑铲 60 报告）
	MaxSuspicious = 3      // 连续非法次数达此值 → 踢出候选
)

// SpeedFor 保留：客户端报模式——服务端用其判定位移上限（未用数值积分）
func SpeedFor(buttons uint8) float32 {
	switch {
	case buttons&protocol.BtnWallRun != 0:
		return 16 * 1.6
	case buttons&protocol.BtnSlide != 0:
		return 16 * 1.3
	default:
		return 16
	}
}

// CheckMove 验证输入与位置合法性；非法返回 false（位置不更新）
// 值域：MoveX/MoveY ∈ {-1,0,1}；浮点：Yaw/Aim/位置禁 NaN/Inf
func CheckMove(p *Player, in protocol.InputReport) bool {
	if in.MoveX < -1 || in.MoveX > 1 || in.MoveY < -1 || in.MoveY > 1 {
		return false // 超值域 = 试图超速（如 MoveX=2 想 2 倍速）
	}
	if !validFloat(in.Yaw) || !validFloat(in.AimX) || !validFloat(in.AimY) ||
		!validFloat(in.X) || !validFloat(in.Y) || !validFloat(in.Z) {
		return false // NaN/Inf 坐标会污染广播
	}
	// 首次上报（出生）或死亡等待中：跳过位移校验（传送合法）
	if p.FirstReport || p.Dead() {
		return true
	}
	// 位移合理性：单帧位移超上限 = 瞬移/超速
	dx, dy, dz := in.X-p.X, in.Y-p.Y, in.Z-p.Z
	horz := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	vert := float32(math.Abs(float64(dy)))
	if horz > MaxHorzStep || vert > MaxVertStep {
		return false
	}
	return true
}

func validFloat(f float32) bool {
	return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0)
}

// MarkViolation 非法上报计数（合法输入清零）
func MarkViolation(p *Player, ok bool) {
	if ok {
		p.Suspicious = 0
	} else {
		p.Suspicious++
	}
}

// IsSuspicious 是否达到踢出阈值
func IsSuspicious(p *Player) bool { return p.Suspicious >= MaxSuspicious }

// ApplyInput 合法上报 → 采纳位置（服务端权威位置 = 客户端机动结果，经验证）
func (p *Player) ApplyInput(in protocol.InputReport, _ float32) {
	p.X, p.Y, p.Z = in.X, in.Y, in.Z
	p.Yaw = in.Yaw
	p.FirstReport = false
}
