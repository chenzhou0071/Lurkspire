// msgid.go — 消息号分区：1-99 网关 / 200-299 大厅（M3 用）/ 300-349 战斗 / 400-449 内部
package protocol

// ---- 网关（1-99）----
const (
	MsgLogin      = 1 // 登录
	MsgLoginResp  = 2 // 登录应答
	MsgHeartbeat  = 9 // 心跳
	MsgKick       = 10
)

// ---- 战斗（300-349）----
const (
	MsgBattleJoin   = 300 // 入房（含房间名/出生点）
	MsgBattleJoinOK = 301 // 入房成功（玩家列表）
	MsgBattleInput  = 310 // 客户端输入/移动上报
	MsgBattleState  = 320 // 服务端状态广播（位置/朝向/血量/武器）
	MsgBattleHit    = 330 // 命中通知（客户端表现用）
	MsgBattleDeath  = 331 // 死亡/击杀广播
	MsgBattleSettle = 340 // 结算
	MsgBattleErr    = 349 // 战斗错误
)

// ---- 网关↔房间 内部（400-449）----
const (
	MsgRegister = 400 // roomd 注册（容量上报）
	MsgLoad     = 401 // 负载上报
	MsgRoomCreate = 402 // 建房指令
	MsgRoomResult = 403 // 房间结果回传
	MsgForward  = 410 // 客户端消息转发到房间
	MsgPush     = 411 // 房间推送转发到客户端
	MsgReject   = 412 // 建房被拒（满载）
)
