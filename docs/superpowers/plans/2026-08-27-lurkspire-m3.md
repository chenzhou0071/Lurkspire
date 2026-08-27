# Lurkspire M3（联机打磨）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 联机完整化——登录/匹配/战绩入库（MySQL）、结算面板、同步手感打磨（插值/延迟补偿）、4-8 人局实测。验收：4 人局完整对局（登录→匹配→对战→结算→战绩）。

**Architecture:** 网关补全大厅业务（账号/匹配/战绩——从 SignalDrift lobby 移植简化）；MySQL 接入；房间结果回传 → ELO/战绩入库；客户端状态渲染插值 + 远端玩家平滑。

**Tech Stack:** Go、MySQL（go-sql-driver）、SignalDrift lobby 代码移植、Unity 6

## Global Constraints

- 大厅业务移植 SignalDrift（bcrypt/HMAC Token/匹配池）——代码直接复制改造，不做新设计
- MySQL 表结构沿用 SignalDrift（users/records）语义
- 所有新逻辑 TDD（`-race` 全绿）
- 结算队列：单 worker 串行写库（沿用 SignalDrift EventQueue 模式）
- 同步手感目标：远端玩家位置插值平滑无跳变、本地零延迟

---

### Task 1: 大厅业务移植（账号/Token）

**Files:**
- Create: `internal/lobby/service.go`、`internal/lobby/token.go`（从 SignalDrift 移植）
- Create: `internal/store/store.go`（Mem/MySQL 双实现）
- Test: `internal/lobby/service_test.go`

**验收：** 注册/登录/Token 校验测试全绿；网关接入登录流程（客户端 Login → LoginResp）。

### Task 2: MySQL 接入

**Files:**
- Create: `internal/store/mysql.go`（users/records 表）
- Create: `db/schema.sql`
- Test: 集成测试（本地 MySQL 起库）

**验收：** 注册落库、登录查库、战绩插入。

### Task 3: 匹配池与房间分配

**Files:**
- Create: `internal/lobby/match.go`（4-8 人匹配：等满 4 人开房或 30 秒宽松开房）
- Modify: `internal/gateway/hub.go`（匹配成功 → 建房 → MatchFound）
- Test: `internal/lobby/match_test.go`

**验收：** 4 人齐开房；不足 4 人 30 秒后开房（人机填充留给 M4）。

### Task 4: 结算回传与战绩

**Files:**
- Create: `internal/room/result.go`（结算 JSON：击杀榜/命中率/时长）
- Modify: 网关（房间结果 → 结算队列 → ELO/战绩入库）
- Test: 结算链路测试

**验收：** 对局打完 → 击杀榜入库 → 双方/全员收到 Settle（击杀榜）。

### Task 5: 客户端插值与平滑

**Files:**
- Modify（Unity）: `Assets/Scripts/Net/NetworkClient.cs`、新增 `Assets/Scripts/Player/RemotePlayerView.cs`
- Test: EditMode（插值数学：两帧间 Lerp 正确性）

**验收：** 远端玩家移动平滑无跳变（30Hz 包 → 60fps 渲染）。

### Task 6: 4 人局实测与调优

- [ ] 用户 4 开（或 2 人 × 2 窗口）实测

```text
验收清单：
□ 登录 → 匹配 → 4 人同房
□ 对战完整：移动/枪/刀/格挡/锁头全部生效
□ 击杀榜实时更新
□ 打完结算（击杀榜/命中率）→ 战绩入库 → 档案可查
□ 断线重连：中途关窗口 → 重连回房
□ 手感：远端平滑、本地零延迟
□ 数值平衡抽查（击杀时间符合设计：枪 4 发/刀 2 刀）
```

- [ ] 修复 + 提交

---

## Self-Review 备注

- 规格覆盖：登录/匹配/结算/战绩（T1-T4）、联机打磨（T5/T6）
- 人机（Bot）不在 M3——M4 若需要再加
- ELO 在 Lurkspire 是否需要：FFA 多人计分（击杀榜）为主，ELO 可省（或按击杀榜折算）——M3 实现时与用户确认
