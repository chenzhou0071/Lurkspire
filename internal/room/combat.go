// combat.go — 服务端权威命中：枪射线/刀近战/冲刺斩/锁头必中/格挡/死亡重生
// 只允许在 Room.loop goroutine 内调用（单写者）
package room

import (
	"math"

	"lurkspire/server/internal/protocol"
)

// 战斗常量（对齐客户端 GameConfig：枪 25/40m、刀 50/3m、冲刺 6m、
// 锁头 50、格挡减半-10、重生 2s、射速 0.12s）
const (
	GunDamage           = 25
	GunRange      int32 = 40
	SwordDamage         = 50
	SwordRange    int32 = 3
	SwordArcDeg         = 60.0 // 挥砍扇形半角（度）
	DashDamage          = 50
	DashRange     int32 = 6
	DashArcDeg          = 30.0 // 冲刺路径半角（度）
	LockDamage          = 50
	LockRange     int32 = 60
	LockMaxCharges      = 3
	LockChargeSeconds   = 10.0
	CombatBlockMin      = 10.0 // 格挡条 <10 不触发格挡
	BlockCost           = 10.0
	BlockDamageMult     = 0.5
	RespawnTicks        = 60 // 重生 2 秒（30Hz）
	FireCooldownTicks   = 4  // 射速 0.12s（30Hz≈4 tick）
	PlayerRadius        = 1.0 // 玩家碰撞球半径（中心 y+1——覆盖整个人 0-2m）
)

// Combat 战斗结算器（持房间玩家表 + 地图墙盒——单写者内使用）
type Combat struct {
	players map[uint32]*Player
	walls   []WallBox
	eyeY    float32 // 射手视线高度（射线起点 y 偏移）
}

func NewCombat(players map[uint32]*Player, walls []WallBox) *Combat {
	return &Combat{players: players, walls: walls, eyeY: 1.5}
}

// ---- 射线/球体几何 ----

// DirFromAngles 欧拉角（度）→ 单位方向（yaw 绕 Y，pitch 绕 X——与客户端准星一致）
func DirFromAngles(yaw, pitch float32) (float32, float32, float32) {
	yr := yaw * math.Pi / 180
	pr := pitch * math.Pi / 180
	cp := float32(math.Cos(float64(pr)))
	return cp * float32(math.Sin(float64(yr))),
		float32(math.Sin(float64(pr))),
		cp * float32(math.Cos(float64(yr)))
}

// rayAABB 射线-墙盒求交：返回命中距离（t），未命中返回 -1（Slab 法）
func rayAABB(ox, oy, oz, dx, dy, dz float32, w WallBox) float32 {
	tmin := float32(math.Inf(-1))
	tmax := float32(math.Inf(1))
	half := [3]float32{w.W / 2, w.H / 2, w.D / 2}
	center := [3]float32{w.X, w.Y, w.Z}
	o := [3]float32{ox, oy, oz}
	d := [3]float32{dx, dy, dz}
	for i := 0; i < 3; i++ {
		if math.Abs(float64(d[i])) < 1e-9 {
			if o[i] < center[i]-half[i] || o[i] > center[i]+half[i] {
				return -1 // 平行且在外
			}
			continue
		}
		t1 := (center[i] - half[i] - o[i]) / d[i]
		t2 := (center[i] + half[i] - o[i]) / d[i]
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tmin {
			tmin = t1
		}
		if t2 < tmax {
			tmax = t2
		}
		if tmin > tmax {
			return -1
		}
	}
	if tmin < 0 {
		return -1 // 射线起点在盒内或反向
	}
	return tmin
}

// raySphere 射线-球体求交：返回命中距离（t），未命中返回 -1
func raySphere(ox, oy, oz, dx, dy, dz, sx, sy, sz, r float32) float32 {
	px, py, pz := sx-ox, sy-oy, sz-oz
	proj := px*dx + py*dy + pz*dz
	if proj < 0 {
		return -1 // 球在射线反方向
	}
	// 垂直距离²
	perp2 := px*px + py*py + pz*pz - proj*proj
	if perp2 > r*r {
		return -1
	}
	// 进入距离（球表面到投影点）
	enter := float32(math.Sqrt(float64(r*r - perp2)))
	return proj - enter
}

// RaycastWalls 沿射线找最近墙（未撞返回 -1）
func (c *Combat) RaycastWalls(ox, oy, oz, dx, dy, dz float32) float32 {
	best := float32(-1)
	for _, w := range c.walls {
		t := rayAABB(ox, oy, oz, dx, dy, dz, w)
		if t > 0 && (best < 0 || t < best) {
			best = t
		}
	}
	return best
}

// ---- 命中结算 ----

// hit 结算一次命中：格挡介入 → 扣血 → 死亡处理 → 返回命中事件
// (shooter 可为 nil——场景伤害/无射手；此处必有射手)
func (c *Combat) hit(shooter, target *Player, dmg uint8) protocol.HitEvent {
	ev := protocol.HitEvent{Shooter: shooter.UID, Target: target.UID, Damage: dmg}
	// 格挡介入：目标格挡姿态且条足够 → 减伤 50% + 条 -10
	if target.Blocking() {
		ev.Damage = uint8(float32(dmg) * BlockDamageMult)
		target.Block -= BlockCost
		if target.Block < 0 {
			target.Block = 0
		}
	}
	// 扣血（防下溢）
	if ev.Damage >= target.HP {
		ev.Damage = target.HP
		target.HP = 0
	} else {
		target.HP -= ev.Damage
	}
	// 死亡：击杀计分 + 重生计时（JustDied 标记供 Death 广播）
	if target.HP == 0 {
		shooter.Score++
		target.Deaths++
		target.DeadTicks = RespawnTicks
		target.JustDied = true
	}
	return ev
}

// ---- 武器动作（由按钮驱动） ----

// ApplyShot 枪射线（40m）：墙挡判定 → 最近玩家球体命中
func (c *Combat) ApplyShot(shooter *Player) []protocol.HitEvent {
	if shooter.Dead() {
		return nil
	}
	if shooter.FireCd > 0 {
		return nil // 射速冷却中
	}
	shooter.FireCd = FireCooldownTicks
	ox, oy, oz := shooter.X, shooter.Y+c.eyeY, shooter.Z
	dx, dy, dz := DirFromAngles(shooter.AimX, shooter.AimY)
	wallT := c.RaycastWalls(ox, oy, oz, dx, dy, dz)
	// 找射线上最近存活玩家（碰撞球：中心 y+1、半径 1——含 0-2m 身高）
	var best *Player
	bestT := float32(-1)
	for _, p := range c.players {
		if p == shooter || p.Dead() {
			continue
		}
		t := raySphere(ox, oy, oz, dx, dy, dz, p.X, p.Y+1, p.Z, PlayerRadius)
		if t > 0 && t <= float32(GunRange) && (bestT < 0 || t < bestT) {
			bestT = t
			best = p
		}
	}
	if best == nil {
		return nil
	}
	if wallT > 0 && wallT < bestT {
		return nil // 墙挡
	}
	return []protocol.HitEvent{c.hit(shooter, best, GunDamage)}
}

// ApplySword 刀近战（前方 3m 扇形 60°）：范围内全员一刀 50
// 冷却 0.25s（防左键按住多帧重复触发——客户端冷却服务端镜像）
func (c *Combat) ApplySword(shooter *Player) []protocol.HitEvent {
	if shooter.Dead() || shooter.SwordCd > 0 {
		return nil
	}
	shooter.SwordCd = 8 // 0.25s（30Hz）
	_, _, dz := shooter.AimX, shooter.AimY, shooter.Yaw // 面向：用 Yaw 简化扇形朝向
	_ = dz
	events := []protocol.HitEvent{}
	// 扇形中心方向：视线水平分量（yaw）
	yawR := shooter.Yaw * math.Pi / 180
	fx := float32(math.Sin(float64(yawR)))
	fz := float32(math.Cos(float64(yawR)))
	cosArc := float32(math.Cos(SwordArcDeg * math.Pi / 180))
	for _, p := range c.players {
		if p == shooter || p.Dead() {
			continue
		}
		ex, ez := p.X-shooter.X, p.Z-shooter.Z
		dist := float32(math.Sqrt(float64(ex*ex + ez*ez)))
		if dist > float32(SwordRange) {
			continue
		}
		// 高度差（2m 内可砍到——地面差异容忍）
		if float32(math.Abs(float64(p.Y-shooter.Y))) > 2 {
			continue
		}
		dot := (ex*fx + ez*fz) / dist // 方向夹角余弦
		if dot < cosArc {
			continue // 扇形外
		}
		events = append(events, c.hit(shooter, p, SwordDamage))
	}
	return events
}

// ApplyDash 冲刺斩（前方 6m 路径 ±30°）：路径内全员一刀 50
// 冷却 1.5s（防 Shift 按住多帧重复触发）
func (c *Combat) ApplyDash(shooter *Player) []protocol.HitEvent {
	if shooter.Dead() || shooter.DashCd > 0 {
		return nil
	}
	shooter.DashCd = 45 // 1.5s（30Hz）
	yawR := shooter.Yaw * math.Pi / 180
	fx := float32(math.Sin(float64(yawR)))
	fz := float32(math.Cos(float64(yawR)))
	cosArc := float32(math.Cos(DashArcDeg * math.Pi / 180))
	events := []protocol.HitEvent{}
	for _, p := range c.players {
		if p == shooter || p.Dead() {
			continue
		}
		ex, ez := p.X-shooter.X, p.Z-shooter.Z
		dist := float32(math.Sqrt(float64(ex*ex + ez*ez)))
		if dist > float32(DashRange) {
			continue
		}
		if float32(math.Abs(float64(p.Y-shooter.Y))) > 2 {
			continue
		}
		dot := (ex*fx + ez*fz) / dist
		if dot < cosArc {
			continue
		}
		events = append(events, c.hit(shooter, p, DashDamage))
	}
	return events
}

// ApplyLock 锁头（必中最近目标——无视偏移；范围 60m 内直线判定，
// 无墙检测与客户端一致：锁头是"点名"必中）
// 充能消耗：由调用方先检查（LockCharges > 0）；发射后 1s 冷却防重复触发
// （客户端 BtnLock 可能持续多帧——冷却保证一次蓄力只发一发）
func (c *Combat) ApplyLock(shooter *Player) []protocol.HitEvent {
	if shooter.Dead() || shooter.LockCharges <= 0 || shooter.LockCd > 0 {
		return nil
	}
	shooter.LockCharges--
	shooter.LockCd = 30 // 1 秒冷却（30Hz）
	// 射手准星方向（AimX 水平/AimY 俯仰——度）——锁头锥形 20°（防锁到背后）
	dx, dy, dz := DirFromAngles(shooter.AimX, shooter.AimY)
	cosArc := float32(math.Cos(20 * math.Pi / 180))
	// 最近存活目标（无视墙——锁头必中机制；准星 20° 内）
	var best *Player
	bestDist := float32(math.Inf(1))
	for _, p := range c.players {
		if p == shooter || p.Dead() {
			continue
		}
		ex, ey, ez := p.X-shooter.X, (p.Y + 1) - shooter.Y, p.Z-shooter.Z
		d := float32(math.Sqrt(float64(ex*ex + ey*ey + ez*ez)))
		if d <= float32(LockRange) && d < bestDist {
			// 方向夹角过滤（准星锥形）
			dot := (ex*dx + ey*dy + ez*dz) / d
			if dot >= cosArc {
				bestDist = d
				best = p
			}
		}
	}
	if best == nil {
		return nil
	}
	return []protocol.HitEvent{c.hit(shooter, best, LockDamage)}
}

// ---- 房间驱动（Tick 内） ----

// TickCombat 每帧推进：射速冷却/锁头充能/死亡重生
func (c *Combat) TickCombat() {
	for _, p := range c.players {
		if p.FireCd > 0 {
			p.FireCd--
		}
		if p.LockCd > 0 {
			p.LockCd-- // 锁头冷却倒数
		}
		if p.SwordCd > 0 {
			p.SwordCd-- // 挥砍冷却倒数
		}
		if p.DashCd > 0 {
			p.DashCd-- // 冲刺冷却倒数
		}
		// 锁头充能：10s 一发，存 3 封顶
		if p.LockCharges < LockMaxCharges {
			p.LockTimer += 1.0 / 30
			if p.LockTimer >= LockChargeSeconds {
				p.LockTimer = 0
				p.LockCharges++
			}
		}
		// 死亡重生：倒计时结束复活（满血回随机出生点——防守尸）
		if p.DeadTicks > 0 {
			p.DeadTicks--
			if p.DeadTicks == 0 {
				sp := PickSpawn()
				p.HP = 100
				p.Block = 100
				p.X, p.Y, p.Z = sp.X, sp.Y, sp.Z
				p.FirstReport = true // 复活后首帧跳过位移校验（传送）
			}
		}
	}
}
