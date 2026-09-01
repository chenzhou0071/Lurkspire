// battle.go — 对局协议编解码：玩家状态/输入上报/命中事件（二进制大端）
package protocol

import (
	"encoding/binary"
	"math"
)

func mathFloat32bits(f float32) uint32 { return math.Float32bits(f) }
func mathFloat32frombits(b uint32) float32 { return math.Float32frombits(b) }

// ---- 结构体定义 ----

// PlayerState 玩家状态（服务端广播，EncodeState 序列化）
// 布局：uid u32 | x f32 | y f32 | z f32 | yaw f32 | hp u8 | weapon u8 | alt u8 | block f32 | anim u8 = 28B
type PlayerState struct {
	UID    uint32
	X, Y, Z float32
	Yaw    float32
	HP     uint8
	Weapon uint8 // 0=枪 1=刀
	Alt    uint8 // 交替枪号（表现用）
	Block  float32 // 格挡条
	Anim   uint8 // 动画状态（跑墙/滑铲/空中等）
}

// InputReport 客户端输入上报（移动/瞄准/按钮）
// 布局：moveX i8 | moveY i8 | yaw f32 | buttons u8 | aimX f32 | aimY f32 = 15B
// buttons 位：bit0 开火 bit1 刀挥 bit2 格挡 bit3 滑铲 bit4 跳 bit5 冲刺斩
type InputReport struct {
	MoveX  int8
	MoveY  int8
	Yaw    float32
	Buttons uint8
	AimX   float32 // 准星世界方向（水平角）
	AimY   float32 // 准星俯仰角
}

const (
	BtnFire     uint8 = 1 << 0
	BtnSword    uint8 = 1 << 1
	BtnBlock    uint8 = 1 << 2
	BtnSlide    uint8 = 1 << 3
	BtnJump     uint8 = 1 << 4
	BtnDashAtk  uint8 = 1 << 5
)

// HitEvent 命中通知（服务端 → 客户端表现用）
// 布局：shooter u32 | target u32 | damage u8 | headshot u8 = 10B
type HitEvent struct {
	Shooter  uint32
	Target   uint32
	Damage   uint8
	Headshot bool
}

// ---- 编解码 ----

const (
	stateSize  = 28
	inputSize  = 15
	hitSize    = 10
)

// EncodeState 玩家列表 → 字节流（count u8 + N×28）
func EncodeState(states []PlayerState) []byte {
	b := make([]byte, 1+len(states)*stateSize)
	b[0] = uint8(len(states))
	for i, s := range states {
		o := 1 + i*stateSize
		binary.BigEndian.PutUint32(b[o:], s.UID)
		binary.BigEndian.PutUint32(b[o+4:], mathFloat32bits(s.X))
		binary.BigEndian.PutUint32(b[o+8:], mathFloat32bits(s.Y))
		binary.BigEndian.PutUint32(b[o+12:], mathFloat32bits(s.Z))
		binary.BigEndian.PutUint32(b[o+16:], mathFloat32bits(s.Yaw))
		b[o+20] = s.HP
		b[o+21] = s.Weapon
		b[o+22] = s.Alt
		binary.BigEndian.PutUint32(b[o+23:], mathFloat32bits(s.Block))
		b[o+27] = s.Anim
	}
	return b
}

// DecodeState 字节流 → 玩家列表（EncodeState 逆操作；数据不合法返回 nil）
func DecodeState(b []byte) []PlayerState {
	if len(b) < 1 {
		return nil
	}
	n := int(b[0])
	if len(b) < 1+n*stateSize {
		return nil
	}
	states := make([]PlayerState, n)
	for i := 0; i < n; i++ {
		o := 1 + i*stateSize
		states[i] = PlayerState{
			UID:    binary.BigEndian.Uint32(b[o:]),
			X:      mathFloat32frombits(binary.BigEndian.Uint32(b[o+4:])),
			Y:      mathFloat32frombits(binary.BigEndian.Uint32(b[o+8:])),
			Z:      mathFloat32frombits(binary.BigEndian.Uint32(b[o+12:])),
			Yaw:    mathFloat32frombits(binary.BigEndian.Uint32(b[o+16:])),
			HP:     b[o+20],
			Weapon: b[o+21],
			Alt:    b[o+22],
			Block:  mathFloat32frombits(binary.BigEndian.Uint32(b[o+23:])),
			Anim:   b[o+27],
		}
	}
	return states
}

// EncodeInput 输入上报 → 字节流（15B）
func EncodeInput(in *InputReport) []byte {
	b := make([]byte, inputSize)
	b[0] = uint8(in.MoveX)
	b[1] = uint8(in.MoveY)
	binary.BigEndian.PutUint32(b[2:], mathFloat32bits(in.Yaw))
	b[6] = in.Buttons
	binary.BigEndian.PutUint32(b[7:], mathFloat32bits(in.AimX))
	binary.BigEndian.PutUint32(b[11:], mathFloat32bits(in.AimY))
	return b
}

// DecodeInput 字节流 → 输入上报（长度不足返回零值）
func DecodeInput(b []byte) InputReport {
	if len(b) < inputSize {
		return InputReport{}
	}
	return InputReport{
		MoveX:   int8(b[0]),
		MoveY:   int8(b[1]),
		Yaw:     mathFloat32frombits(binary.BigEndian.Uint32(b[2:])),
		Buttons: b[6],
		AimX:    mathFloat32frombits(binary.BigEndian.Uint32(b[7:])),
		AimY:    mathFloat32frombits(binary.BigEndian.Uint32(b[11:])),
	}
}

// EncodeHit 命中事件 → 字节流（10B）
func EncodeHit(h *HitEvent) []byte {
	b := make([]byte, hitSize)
	binary.BigEndian.PutUint32(b[0:], h.Shooter)
	binary.BigEndian.PutUint32(b[4:], h.Target)
	b[8] = h.Damage
	if h.Headshot {
		b[9] = 1
	}
	return b
}

// DecodeHit 字节流 → 命中事件
func DecodeHit(b []byte) HitEvent {
	if len(b) < hitSize {
		return HitEvent{}
	}
	return HitEvent{
		Shooter:  binary.BigEndian.Uint32(b[0:]),
		Target:   binary.BigEndian.Uint32(b[4:]),
		Damage:   b[8],
		Headshot: b[9] != 0,
	}
}
