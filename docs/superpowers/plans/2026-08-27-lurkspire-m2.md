# Lurkspire M2（服务器）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lurkspire 服务器——复用 SignalDrift 双进程架构（网关 + 房间 + MySQL），房间改 4-8 人广播；同步方案：客户端移动 + 服务端验证（速度上限/瞬移检测）、服务端权威命中（射线判定）。验收：双开同局正常同步、对局可打完。

**Architecture:** Go 服务端（新仓库 `E:\pro\Lurkspire`，代码结构参考 SignalDrift `server/`）。帧协议直接移植（12B 头 + 大端 + 消息号分区）；网关进程做连接/登录/匹配（精简版），房间进程做 30Hz 对局。命中即时射线（非投射物）。移动上报 + 服务端校验。

**Tech Stack:** Go、TCP 帧协议、MySQL（M3 才接战绩，M2 先用内存）、Unity 6（客户端接入）

## Global Constraints

- 帧协议与消息号分区：从 SignalDrift 移植（magic 0x5344、12B 头、1-99 网关/300-349 战斗/400-449 内部）
- 同步方案（设计文档已确认）：客户端移动 + 服务端验证；服务端权威命中
- 数值沿用 `GameConfig` 客户端侧定义（服务端不做玩法模拟，只做移动验证 + 命中判定 + 状态转发）
- 每个任务 TDD（Go 测试 `-race` 全绿为硬门槛）
- M2 无数据库依赖（内存存储），M3 再接 MySQL
- 提交粒度：每任务一提交

---

### Task 1: Go 项目骨架与协议移植

**Files:**
- Create: `go.mod`（module lurkspire/server）
- Create: `cmd/gateway/main.go`、`cmd/roomd/main.go`（骨架）
- Create: `internal/protocol/frame.go`、`internal/protocol/msgid.go`（从 SignalDrift 移植）
- Test: `internal/protocol/frame_test.go`

**Interfaces:**
- Produces: `protocol.FrameReader`（粘包拆包）、`protocol.Encode(msgID, seq, body)`、`protocol.MsgBattleXxx` 常量

- [ ] **Step 1: 移植帧协议**

从 `E:\pro\SignalDrift\server\internal\protocol\` 复制 `frame.go`/`msgid.go` 并适配新消息号：
```go
// 战斗消息号（Lurkspire 专用 300-349）
const (
    MsgBattleJoin    = 300 // 入房
    MsgBattleJoinOK  = 301 // 入房成功（含玩家列表）
    MsgBattleInput   = 310 // 客户端输入/移动上报
    MsgBattleState   = 320 // 服务端状态广播（位置/朝向/血量/武器状态）
    MsgBattleHit     = 330 // 命中通知（客户端表现用）
    MsgBattleDeath   = 331 // 死亡/击杀广播
    MsgBattleSettle  = 340 // 结算
    MsgBattleErr     = 349
)
```

- [ ] **Step 2: TDD 帧编解码测试**（从 SignalDrift 移植测试模式：golden bytes 对拍）

- [ ] **Step 3: 提交**

```bash
git add go.mod cmd internal/protocol
git commit -m "feat: Go 骨架与帧协议移植"
```

---

### Task 2: 对局协议定义（状态包/输入包）

**Files:**
- Create: `internal/protocol/battle.go`（编解码）
- Test: `internal/protocol/battle_test.go`

**Interfaces:**
- Produces:
  - `protocol.PlayerState`：`{uid uint32, x/y/z float32, yaw float32, hp uint8, weapon uint8, alt uint8(交替枪号), block float32, anim uint8}`
  - `protocol.InputReport`：`{moveX/moveY int8, yaw float32, buttons uint8(射击/刀/格挡/滑铲/跳), aimX/aimY float32}`
  - `protocol.HitEvent`：`{shooter uint32, target uint32, damage uint8, headshot bool}`
  - `EncodeState([]PlayerState) []byte`、`DecodeInput([]byte) InputReport`

- [ ] **Step 1: 写失败测试**（编解码 roundtrip + golden bytes）

- [ ] **Step 2: 实现编解码**（二进制、大端——参考 SignalDrift codec 风格）

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(protocol): 对局状态/输入/命中编解码"
```

---

### Task 3: 房间核心（4-8 人 30Hz）

**Files:**
- Create: `internal/room/room.go`、`internal/room/manager.go`
- Test: `internal/room/room_test.go`

**Interfaces:**
- Produces: `room.Room`（`NewRoom(id, config)` / `AddPlayer(uid) error`（满 8 人拒绝）/ `RemovePlayer(uid)` / `HandleInput(uid, protocol.InputReport)` / `Tick()` 30Hz 循环 / `StateSnapshot() []protocol.PlayerState`）、`room.Manager`（建房/找房/容量 8）

- [ ] **Step 1: 写失败测试**

```go
// 房间容量 8、第 9 人拒绝
// 输入更新玩家状态（移动上报 → 玩家位置变化）
// 移除玩家后广播列表不含
```

- [ ] **Step 2: 实现**

```go
// Room：单写者 goroutine（Tick 循环），输入经 channel 进入
type Room struct {
    players map[uint32]*Player
    inputCh chan inputMsg
}
// Player：{uid, pos Vector3, yaw, hp, weapon, alt, block, lastReportTick, suspicious}
```

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(room): 4-8人房间核心（30Hz/容量/输入槽）"
```

---

### Task 4: 移动验证（速度上限/瞬移检测）

**Files:**
- Create: `internal/room/validation.go`
- Test: `internal/room/validation_test.go`

**Interfaces:**
- Consumes: `room.Player`、`GameConfig` 数值（从客户端 GameConfig 抄：RunSpeed 12、WallRun 0.85 倍、Slide 1.1 倍）
- Produces: `validation.CheckMove(player, report, dt) bool`（返回是否合法——超速/瞬移拒绝并标记 `suspicious`）

- [ ] **Step 1: 写失败测试**

```go
// 正常速度移动 → 通过
// 2 倍速上报（瞬移）→ 拒绝（位置不更新）
// 连续 3 次非法 → 标记 suspicious（踢出候选）
// 跑墙速度（0.85×12）→ 通过
// 滑铲速度（1.1×12）→ 通过
```

- [ ] **Step 2: 实现**

```go
// 校验逻辑：期望位移 = 速度上限 × dt；实际位移 > 1.5× 期望 → 拒绝
// 速度上限按状态取（普通/跑墙/滑铲——客户端上报状态位，服务端按状态给上限）
```

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(room): 移动验证（速度上限/瞬移检测/可疑标记）"
```

---

### Task 5: 服务端权威命中

**Files:**
- Create: `internal/room/combat.go`
- Test: `internal/room/combat_test.go`

**Interfaces:**
- Consumes: `protocol.InputReport.buttons`、玩家位置/朝向
- Produces: `room.Combat.ApplyShot(shooter, aimDir)` → `[]protocol.HitEvent`；`ApplySword(shooter, dir)` → `[]HitEvent`（近战扇形/冲刺斩范围）

- [ ] **Step 1: 写失败测试**

```go
// 射线 50m 内命中最近玩家 → 伤害 25（4 发击杀）
// 射线无人 → 空结果
// 刀近战 3m 扇形命中 → 伤害 50（两刀击杀）
// 格挡者：伤害减 50% + 格挡条 -10；格挡条 <10 不触发
// 锁头：命中最近目标必中（无视偏移，10s 充能/存 3 发服务端记账）
// 击杀 → Death 事件 + 计分 + 重生计时
```

- [ ] **Step 2: 实现**

```go
// 地图碰撞：M2 用简化地图（配置：墙盒列表——服务端拥有墙体 AABB，射线求交）
// 命中流程：射线 → 墙阻挡 → 玩家包围盒（球体 r=0.5）→ 伤害 → 格挡介入 → 死亡/重生
```

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(room): 服务端权威命中（枪/刀/锁头/格挡/死亡重生）"
```

---

### Task 6: 状态广播与结算

**Files:**
- Modify: `internal/room/room.go`
- Test: `internal/room/broadcast_test.go`

**Interfaces:**
- Produces: `Room.Tick` 广播 `MsgBattleState`（30Hz × N 玩家）+ `MsgBattleDeath`/`MsgBattleHit` 事件 + `MsgBattleSettle`（20 分或 5 分钟）

- [ ] **Step 1: 写失败测试**

```go
// Tick 后所有在线玩家收到 State（含自己）
// 击杀后收到 Death 事件（含分数更新）
// 20 分到达 → Settle 广播一次（锁存，不重复）
// 5 分钟时限 → Settle
```

- [ ] **Step 2: 提交**

```bash
git commit -m "feat(room): 状态广播/事件/结算"
```

---

### Task 7: 网关（连接/入房/匹配精简版）

**Files:**
- Create: `internal/gateway/server.go`（连接层，从 SignalDrift 移植精简）
- Create: `internal/gateway/hub.go`（房间分配：单 roomd 模式下直接建房；房间列表/加入）
- Test: `internal/gateway/server_test.go`

**Interfaces:**
- Produces: `gateway.Server`（监听/会话/帧路由/优雅关闭）、`MsgBattleJoin` 处理（建房或加入已有房——M2 简化：同一"房间名"加入）

- [ ] **Step 1: TDD 移植连接层测试**（心跳/seq/优雅关闭——从 SignalDrift 测试改造）

- [ ] **Step 2: 实现 Join 流程**：客户端发 Join（带房间名）→ 有房加入/无房创建 → JoinOK（玩家列表 + 出生点）

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(gateway): 连接层与入房（简化匹配）"
```

---

### Task 8: Unity 客户端接入（NetworkClient + 上报/接收）

**Files:**
- Create（Unity 项目 `E:\unity_xiangmu\Lurkspire`）: `Assets/Scripts/Net/Protocol.cs`（帧编解码 C# 镜像）、`Assets/Scripts/Net/NetworkClient.cs`（连接/重连）、`Assets/Scripts/Net/BattleMessages.cs`（对局 DTO）
- Modify: `Assets/Scripts/Player/PlayerInput.cs`（移动上报 30Hz）、`Assets/Scripts/Weapons/GunController.cs`（开火上报）、`Assets/Scripts/Weapons/SwordController.cs`（刀/格挡上报）
- Test: `Assets/Tests/EditMode/ProtocolTests.cs`（golden vector 对拍——Go 真实输出钉字面量）

**Interfaces:**
- Consumes: 全部 M2 服务端协议
- Produces: `NetworkClient.I`（SendInput/SendJoin/OnState 回调）、客户端本地权威移动（上报 + 服务端校正）

- [ ] **Step 1: 用户创建 Unity 项目**（`E:\unity_xiangmu\Lurkspire`，Unity 6 3D URP）——或与 M1 项目合并（一个 Unity 项目同时承载 M1 单机 + M2 联机）

- [ ] **Step 2: C# 编解码 golden vector 对拍测试**

- [ ] **Step 3: NetworkClient + 战斗消息接入**

- [ ] **Step 4: 移动上报集成**：本地 PlayerInput 照常驱动（零延迟手感），每 tick 上报位置/状态；收到服务端校正（仅拒绝时回滚/提示）

- [ ] **Step 5: 提交**

```bash
git commit -m "feat(client): 联机接入（协议对拍/上报/接收）"
```

---

### Task 9: 双开联调与 M2 验收

- [ ] **Step 1: 用户双开实测**（本机双开，同房间名加入）

```text
验收清单：
□ 双开同局：两个客户端看到对方（位置/朝向/移动平滑）
□ 开枪命中对方（服务端判定 → 掉血 → HUD 更新）
□ 刀/格挡生效（格挡减伤 + 格挡条）
□ 击杀 → 重生 → 计分
□ 瞬移检测：修改上报速度（作弊）→ 服务端拒绝（位置不动）
□ 20 分/5 分钟 → 结算
```

- [ ] **Step 2: 提交验收修复**

---

## Self-Review 备注

- 规格覆盖：客户端移动+服务端验证（T4）、权威命中（T5）、4-8 人广播（T3/T6）、结算（T6）、网关复用（T7）、客户端接入（T8）
- 数值一致：命中数值（枪 25/刀 50/格挡 50%/10）来自设计文档，服务端与客户端 GameConfig 双份维护（M2 阶段手写同步，M3 考虑配置下发）
- 地图碰撞：M2 简化（墙盒列表配置），M3 精修
