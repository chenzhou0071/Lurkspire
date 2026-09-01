// validation.go — 移动验证（服务端权威）：客户端只报状态不报位置
// 位置 = 服务端按"配置速度 × 输入方向"积分（客户端改速度无用）
// 校验：输入值域 / 浮点 NaN·Inf / 连续非法标记 suspicious（踢出候选）
package room

import (
	"math"

	"lurkspire/server/internal/protocol"
)

// 服务端速度表（对应客户端 GameConfig：Run 16 / Slide ×1.3 / WallRun ×1.6）
const (
	RunSpeed       float32 = 16
	SlideSpeed     float32 = 16 * 1.3
	WallRunSpeed   float32 = 16 * 1.6
	MaxSuspicious          = 3 // 连续非法次数达此值 → 踢出候选
)

// SpeedFor 按钮状态 → 速度（服务端权威；客户端只影响模式选择，数值服务端定）
func SpeedFor(buttons uint8) float32 {
	switch {
	case buttons&protocol.BtnWallRun != 0:
		return WallRunSpeed
	case buttons&protocol.BtnSlide != 0:
		return SlideSpeed
	default:
		return RunSpeed
	}
}

// CheckMove 验证输入合法性；非法返回 false（位置不更新）
// 值域：MoveX/MoveY ∈ {-1,0,1}；浮点：Yaw/Aim 禁 NaN/Inf
func CheckMove(p *Player, in protocol.InputReport) bool {
	if in.MoveX < -1 || in.MoveX > 1 || in.MoveY < -1 || in.MoveY > 1 {
		return false // 超值域 = 试图超速（如 MoveX=2 想 2 倍速）
	}
	if !validFloat(in.Yaw) || !validFloat(in.AimX) || !validFloat(in.AimY) {
		return false // NaN/Inf 坐标会污染广播
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

// ApplyInput 合法输入 → 积分移动（服务端权威位置）
// 斜向归一化：斜走不快（|(1,1)| 归一化后每轴 0.707）
func (p *Player) ApplyInput(in protocol.InputReport, dt float32) {
	dirX := float32(in.MoveX)
	dirZ := float32(in.MoveY)
	l := float32(math.Sqrt(float64(dirX*dirX + dirZ*dirZ)))
	if l > 1 {
		dirX /= l
		dirZ /= l
	}
	speed := SpeedFor(in.Buttons)
	p.X += dirX * speed * dt
	p.Z += dirZ * speed * dt
	p.Yaw = in.Yaw
}
