# Lurkspire M3（账号/好友/房间大厅/背包）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 联机完整化——登录注册（账号/密码/昵称独立）→ 大厅（双向好友实时在线 + 房间列表大厅创建/加入）→ 背包装备（二段跳+0.3m/跑步+1/弹夹+4/格挡+20，单装备）→ 游戏内打磨（ESC 退出确认/Tab 昵称黄底/换弹圆形条）。MySQL 持久化账号/好友/装备。无 ELO/战绩/匹配/结算。

**Architecture:** 单进程合一网关继续（内嵌全部业务）。SignalDrift lobby（bcrypt/Token）移植精简 + MySQL（go-sql-driver）。协议扩大厅段（200-299）。客户端加场景流：登录界面 → 大厅 → 房间（对局场景复用现有 TestArena）。

**Tech Stack:** Go、MySQL、bcrypt、Unity 6

## Global Constraints

- 协议消息号：大厅 200-299（新增）；战斗沿用 300+；帧协议不变
- 账号三独立：登录用 account+password；显示用 nickname（注册时填）
- 好友双向：A→B 邀请，B 同意后双方列表出现；在线状态服务端实时推送（登录/登出广播）
- 房间列表大厅：一排排房间（创建者昵称/房间名/备注/人数）；满 8 弹窗"房间已满"；加入失败提示连接超时；人走光散房（同 M2）
- 背包默认全解锁 4 件，只能装备 1 件；数值：AirJumpHeight +0.3 / RunSpeed +1 / MagazineSize +4 / BlockMax +20；装备选择存 MySQL（users.equip_id）
- 游戏内 Tab：显示昵称（非"玩家 N"），自己那行黄色背景
- 换弹条：弹药数下方圆形（弧形进度）条
- ESC：游戏内弹出"是否退出房间"确认（退出 → 回大厅）
- 每个任务 TDD（Go `-race` 全绿；Unity EditMode 对拍）
- 提交粒度：每任务一提交

---

### Task 1: MySQL 接入 + 账号服务（注册/登录/Token）

**Files:**
- Create: `db/schema.sql`（users 表：id/account/password_hash/nickname/equip_id）
- Create: `internal/store/store.go`（MySQL 连接/迁移——SignalDrift store 精简）
- Create: `internal/lobby/service.go`（Register/Login/Token 签发校验——bcrypt + HMAC）
- Test: `internal/lobby/service_test.go`（注册重名拒绝/登录错密码/Token 校验）

**Interfaces:**
- Consumes: `store.DB`、`lobby.Service`
- Produces: `lobby.Service`：`Register(account, password, nickname) (uid, error)`、`Login(account, password) (token, error)`、`Verify(token) (uid, error)`、`Nickname(uid) string`

**验收：** 注册落库、登录发 Token、重复注册拒绝、Token 可校验。客户端后续用它进大厅。

### Task 2: 大厅协议 + 登录接入网关

**Files:**
- Create: `internal/protocol/lobby.go`（编解码：Login/LoginResp/好友/房间消息体）
- Modify: `internal/gateway/hub.go`（会话挂 uid/昵称；登录态校验——未登录只能发登录类消息）
- Test: `internal/protocol/lobby_test.go`（roundtrip/golden）

**消息号（200-299）：**
```text
MsgLogin 200 / MsgLoginResp 201 / MsgReg 202 / MsgRegResp 203
MsgFriendSearch 210 / MsgFriendInvite 211 / MsgFriendAccept 212 / MsgFriendReject 213
MsgFriendList 214 / MsgFriendOnline 215（在线状态推送）
MsgRoomList 220 / MsgRoomCreate 221 / MsgRoomJoin 222 / MsgRoomLeave 223
MsgBagList 230 / MsgBagEquip 231
```

**验收：** 客户端注册/登录全链路（帧协议 200-203 通）；未登录打战斗消息被拒。

### Task 3: 好友系统（双向 + 在线实时）

**Files:**
- Create: `internal/lobby/friends.go`（好友关系 MySQL：一条关系两行记录；搜索；邀请状态机）
- Create: `internal/lobby/online.go`（在线表：uid→session；登录/登出广播给双方好友）
- Modify: `internal/gateway/hub.go`（好友消息路由：邀请 → 目标在线则推送/离线存邀请表？——M3 简化：仅在线可收到邀请——离线邀请待对方登录后推送）
- Test: `internal/lobby/friends_test.go`（双向建立/搜索/重复邀请拒绝）

**验收：** A 搜 B → 发邀请 → B 收到 → 同意 → 双方列表出现 → A 登出 → B 列表实时变离线（登出推送）→ A 登录 → B 列表实时上线。

### Task 4: 房间列表大厅（创建/加入/散房广播）

**Files:**
- Modify: `internal/gateway/hub.go`（房间注册表：房间名/创建者昵称/备注/人数——M2 Manager 扩展）
- Create: `internal/gateway/rooms.go`（创建/列表/加入/离开——列表变更推送 221/222 全大厅；满 8 拒绝；人走光销毁）
- Test: `internal/gateway/rooms_test.go`

**Interfaces:**
- Produces: `Rooms.List() []RoomInfo`（name/creator/note/players/max 8）、`Create(uid, name, note) error`、`Join(uid, roomName) error`（满员 ErrRoomFull）

**验收：** 建房显示列表（人数 1/8）→ 第二人加入（2/8）→ 满 8 拒绝弹"房间已满" → 人走光房从列表消失。

### Task 5: 背包装备（默认全解锁 + 装备持久化）

**Files:**
- Modify: `internal/lobby/service.go`（装备 4 件常量表 + users.equip_id 读写）
- Modify: 网关（对局加入时上报装备给房间——战斗应用格挡上限）
- Modify: `internal/room/`（Player 开局 Block 用装备上限——120）
- Test: `internal/lobby/bag_test.go`（装备切换/只能一件/持久化）

**装备表：** id0 无、id1 二段跳+0.3（客户端表现）、id2 跑步+1（客户端表现）、id3 弹夹+4（客户端表现）、id4 格挡+20（服务端 BlockMax 120）

**验收：** 大厅背包弹窗 4 件（默认全解锁）→ 选一件装备 → 重登保持 → 进对局：格挡上限 120（服务端权威）；移动/弹夹/二段跳客户端生效。

### Task 6: 客户端登录/大厅 UI（Unity）

**Files:**
- Create（Unity）: `Assets/Scripts/Net/NetUI/`（IMGUI 页面流：LoginPage/LobbyPage/RoomListPage/BagPage——复用 NetworkClient 连接）
- Modify: `NetworkClient`（登录态：连接→登录→大厅消息处理/发送）
- Test: `NetProtocolTests`（大厅编解码对拍）

**页面流：** 登录页（注册/登录 Tab——账号/密码/昵称）→ 大厅页（好友列表+房间列表+背包按钮+开始游戏按钮切换）→ 房间列表页（一排房间+创建弹窗[房间名/备注]+选中房间亮加入）→ 满员弹窗/超时提示。

**验收：** 全流程 UI 可操作（注册→登录→大厅→背包装备→建房→加入）。游戏内 Tab 显示昵称（服务端广播昵称——State 不带昵称——房间加入时下发玩家昵称表，Tab 用昵称表映射）+ 自己行黄底。ESC 退出确认弹窗（退出→回大厅→房间销毁逻辑走 Leave）。换弹圆形条（UGUI Image fillAmount 弧形——Image type Filled Radial360，弹药下叠加——换弹时 GunView 触发显示）。

### Task 7: 端到端验收

- [ ] 用户双开/三开实测

```text
验收清单：
□ 注册（填昵称）→ 登录 → 大厅
□ A 搜 B 加好友 → B 同意 → 双列表出现；A 登出 B 实时见离线
□ A 建房（房间名/备注）→ 大厅列表 B 可见（创建者昵称/人数 1/8）
□ B 加入 → 人数 2/8 → 对局开始（现有战斗全机制）
□ Tab：昵称显示 + 自己黄底；击杀/死亡实时
□ 换弹：弹药下圆形条转满
□ 背包：装备格挡+20 → 对局格挡上限 120；装备跑步 → 移动略快
□ ESC → 退出确认 → 回大厅 → 房列表人数减少
□ 房间满 8 加入 → "房间已满"弹窗
□ 断线重连（重登回大厅）
```

- [ ] 修复 + 提交

---

## Self-Review 备注

- 规格覆盖：账号（T1/T2）、好友（T3）、房间大厅（T4）、背包（T5）、客户端 UI（T6）、验收（T7）
- 无 ELO/战绩/匹配/结算（用户明确砍掉）
- 好友离线邀请：M3 简化——离线时邀请排队，对方登录后推送（T3 实现）
- 昵称显示：对局内服务端不下发昵称到 State（保持 36B）——加入房间时下发"玩家昵称表"，Tab 映射显示
