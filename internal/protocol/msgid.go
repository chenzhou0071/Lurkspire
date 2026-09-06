// msgid.go — 消息号分区：200-299 大厅 / 300-349 战斗
// （M2 网关旧消息已随单进程架构废弃——登录移入大厅段）
package protocol

// ---- 通用 ----
const (
	MsgHeartbeat = 9 // 心跳回显
)

// ---- 大厅（200-299——见 lobby.go 详细定义）----
// MsgLogin 200 ... MsgBagEquip 231

// ---- 战斗（300-349）----
const (
	MsgBattleJoin   = 300 // 入房（登录后——房间名）
	MsgBattleJoinOK = 301 // 入房成功（玩家列表）
	MsgBattleInput  = 310 // 客户端输入/移动上报
	MsgBattleState  = 320 // 服务端状态广播（位置/朝向/血量/武器）
	MsgBattleHit    = 330 // 命中通知（客户端表现用）
	MsgBattleDeath  = 331 // 死亡/击杀广播
	MsgBattleSettle = 340 // 结算（保留协议——无限对局不使用）
	MsgBattleErr    = 349 // 战斗错误
)
