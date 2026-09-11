// lobby.go — 大厅协议编解码（200-299）：登录注册/好友/房间/背包
// 字符串编解码：u8/u16 长度前缀 + UTF8；与帧协议同大端
package protocol

import (
	"encoding/binary"
)

// ---- 大厅消息号（200-299）----
const (
	MsgLogin             = 200 // 登录（account+password）
	MsgLoginResp         = 201 // 登录应答（errCode/token/uid/nickname）
	MsgReg               = 202 // 注册（account+password+nickname）
	MsgRegResp           = 203 // 注册应答（errCode/uid）
	MsgReconnect         = 204 // Token 重连（断线免密恢复——body=token 字符串，应答同 MsgLoginResp）
	MsgFriendSearch      = 210 // 按昵称搜索
	MsgFriendSearchR     = 211 // 搜索结果（目标 uid/昵称/是否已好友）
	MsgFriendInvite      = 212 // 发邀请（目标 uid）
	MsgFriendInviteN     = 213 // 收到邀请推送（邀请方 uid/昵称）
	MsgFriendAccept      = 214 // 同意邀请（目标 uid）
	MsgFriendReject      = 215 // 拒绝邀请（目标 uid）
	MsgFriendList        = 216 // 好友列表（uid/昵称/在线）——登录/变更后推送
	MsgFriendPendingList = 218 // 申请栏（我的待处理申请——登录下发）
	MsgFriendOnline      = 217 // 在线状态推送（uid/在线）
	MsgRoomList          = 220 // 房间列表快照
	MsgRoomCreate        = 221 // 创建房间（name+note）
	MsgRoomJoin          = 222 // 加入房间（name）
	MsgRoomLeave         = 223 // 离开房间
	MsgBagList           = 230 // 背包列表（当前装备）
	MsgBagEquip          = 231 // 切换装备（equipID）
)

// ---- 大厅错误码 ----
const (
	LobbyOK             = 0
	LobbyErrBadInput    = 1
	LobbyErrAccount     = 2 // 账号已存在
	LobbyErrNickname    = 3 // 昵称已存在
	LobbyErrNoAccount   = 4 // 账号不存在
	LobbyErrPassword    = 5 // 密码错
	LobbyErrToken       = 6 // Token 无效
	LobbyErrNotFriend   = 7 // 目标不是好友/搜索不到
	LobbyErrSelf        = 8 // 不能加自己
	LobbyErrRoomFull    = 9
	LobbyErrRoomExists  = 10
	LobbyErrRoomMissing = 11
	LobbyErrEquip       = 12 // 装备 ID 非法
)

// ---- 字符串编解码 ----

func putStr(b []byte, off int, s string) int {
	binary.BigEndian.PutUint16(b[off:], uint16(len(s)))
	copy(b[off+2:], s)
	return off + 2 + len(s)
}

func getStr(b []byte, off int) (string, int, bool) {
	if len(b) < off+2 {
		return "", off, false
	}
	n := int(binary.BigEndian.Uint16(b[off:]))
	if len(b) < off+2+n {
		return "", off, false
	}
	return string(b[off+2 : off+2+n]), off + 2 + n, true
}

// ---- 登录/注册 ----

// EncodeReconnect token → body（u16 len + utf8）——断线重连免密认证
func EncodeReconnect(token string) []byte {
	b := make([]byte, 2+len(token))
	putStr(b, 0, token)
	return b
}

// DecodeReconnect body → token（短数据返回 false）
func DecodeReconnect(b []byte) (string, bool) {
	token, _, ok := getStr(b, 0)
	return token, ok
}

// EncodeLogin account+password → body
func EncodeLogin(account, password string) []byte {
	b := make([]byte, 2+len(account)+2+len(password))
	off := putStr(b, 0, account)
	putStr(b, off, password)
	return b
}

// DecodeLogin body → account/password
func DecodeLogin(b []byte) (string, string, bool) {
	account, off, ok := getStr(b, 0)
	if !ok {
		return "", "", false
	}
	password, _, ok := getStr(b, off)
	return account, password, ok
}

// EncodeReg account+password+nickname → body
func EncodeReg(account, password, nickname string) []byte {
	b := make([]byte, 2+len(account)+2+len(password)+2+len(nickname))
	off := putStr(b, 0, account)
	off = putStr(b, off, password)
	putStr(b, off, nickname)
	return b
}

// DecodeReg body → account/password/nickname
func DecodeReg(b []byte) (string, string, string, bool) {
	account, off, ok := getStr(b, 0)
	if !ok {
		return "", "", "", false
	}
	password, off, ok := getStr(b, off)
	if !ok {
		return "", "", "", false
	}
	nickname, _, ok := getStr(b, off)
	return account, password, nickname, ok
}

// EncodeLoginResp errCode(1) + token(u16)+uid(4)+nickname(u16) → 登录/重连应答
func EncodeLoginResp(errCode uint8, token string, uid uint32, nickname string) []byte {
	b := make([]byte, 1+2+len(token)+4+2+len(nickname))
	b[0] = errCode
	off := putStr(b, 1, token)
	binary.BigEndian.PutUint32(b[off:], uid)
	off += 4
	putStr(b, off, nickname)
	return b
}

// DecodeLoginResp body → errCode/token/uid/nickname（重连测试/对拍用）
func DecodeLoginResp(b []byte) (uint8, string, uint32, string, bool) {
	if len(b) < 1 {
		return 0, "", 0, "", false
	}
	errCode := b[0]
	token, off, ok := getStr(b, 1)
	if !ok || len(b) < off+4 {
		return 0, "", 0, "", false
	}
	uid := binary.BigEndian.Uint32(b[off:])
	nick, _, ok2 := getStr(b, off+4)
	return errCode, token, uid, nick, ok2
}

// EncodeRegResp errCode(1) + uid(4)
func EncodeRegResp(errCode uint8, uid uint32) []byte {
	b := make([]byte, 5)
	b[0] = errCode
	binary.BigEndian.PutUint32(b[1:], uid)
	return b
}

// ---- 好友 ----

// EncodeFriendTarget uid(4) → body（搜索/邀请/同意/拒绝统一目标格式）
func EncodeUID(uid uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uid)
	return b
}

func DecodeUID(b []byte) (uint32, bool) {
	if len(b) < 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(b), true
}

// EncodeFriendInfo uid(4)+nickname(u16)+online(1) → 好友条目（列表/搜索/邀请推送用）
func EncodeFriendInfo(uid uint32, nickname string, online bool) []byte {
	b := make([]byte, 4+2+len(nickname)+1)
	binary.BigEndian.PutUint32(b, uid)
	off := putStr(b, 4, nickname)
	if online {
		b[off] = 1
	}
	return b
}

type FriendInfo struct {
	UID      uint32
	Nickname string
	Online   bool
}

func DecodeFriendInfo(b []byte) (FriendInfo, bool) {
	if len(b) < 4 {
		return FriendInfo{}, false
	}
	fi := FriendInfo{UID: binary.BigEndian.Uint32(b)}
	nick, off, ok := getStr(b, 4)
	if !ok {
		return FriendInfo{}, false
	}
	fi.Nickname = nick
	fi.Online = off < len(b) && b[off] == 1
	return fi, true
}

// EncodeFriendList count(1) + N×FriendInfo
func EncodeFriendList(list []FriendInfo) []byte {
	b := make([]byte, 1)
	b[0] = uint8(len(list))
	for _, fi := range list {
		b = append(b, EncodeFriendInfo(fi.UID, fi.Nickname, fi.Online)...)
	}
	return b
}

func DecodeFriendList(b []byte) ([]FriendInfo, bool) {
	if len(b) < 1 {
		return nil, false
	}
	n := int(b[0])
	off := 1
	var out []FriendInfo
	for i := 0; i < n; i++ {
		fi, ok := DecodeFriendInfo(b[off:])
		if !ok {
			return nil, false
		}
		// 计算该条目长度（uid4 + nickname 2+n + online 1）
		off += 4 + 2 + len(fi.Nickname) + 1
		out = append(out, fi)
	}
	return out, true
}

// EncodeOnlinePush uid(4)+online(1)
func EncodeOnlinePush(uid uint32, online bool) []byte {
	b := make([]byte, 5)
	binary.BigEndian.PutUint32(b, uid)
	if online {
		b[4] = 1
	}
	return b
}

// DecodeOnlinePush body → uid/online
func DecodeOnlinePush(b []byte) (uint32, bool) {
	if len(b) < 5 {
		return 0, false
	}
	return binary.BigEndian.Uint32(b), b[4] == 1
}

// ---- 房间 ----

type RoomInfo struct {
	Name    string
	Creator string // 创建者昵称
	Note    string
	Players int
	Max     int
}

// EncodeRoomInfo 单房间信息
func EncodeRoomInfo(r RoomInfo) []byte {
	b := make([]byte, 2+len(r.Name)+2+len(r.Creator)+2+len(r.Note)+2)
	off := putStr(b, 0, r.Name)
	off = putStr(b, off, r.Creator)
	off = putStr(b, off, r.Note)
	b[off] = uint8(r.Players)
	b[off+1] = uint8(r.Max)
	return b
}

func DecodeRoomInfo(b []byte) (RoomInfo, bool) {
	name, off, ok := getStr(b, 0)
	if !ok {
		return RoomInfo{}, false
	}
	creator, off, ok := getStr(b, off)
	if !ok {
		return RoomInfo{}, false
	}
	note, off, ok := getStr(b, off)
	if !ok || len(b) < off+2 {
		return RoomInfo{}, false
	}
	return RoomInfo{
		Name: name, Creator: creator, Note: note,
		Players: int(b[off]), Max: int(b[off+1]),
	}, true
}

// EncodeRoomList count(1) + N×RoomInfo
func EncodeRoomList(rooms []RoomInfo) []byte {
	b := []byte{uint8(len(rooms))}
	for _, r := range rooms {
		b = append(b, EncodeRoomInfo(r)...)
	}
	return b
}

func DecodeRoomList(b []byte) ([]RoomInfo, bool) {
	if len(b) < 1 {
		return nil, false
	}
	n := int(b[0])
	off := 1
	var out []RoomInfo
	for i := 0; i < n; i++ {
		r, ok := DecodeRoomInfo(b[off:])
		if !ok {
			return nil, false
		}
		off += 2 + len(r.Name) + 2 + len(r.Creator) + 2 + len(r.Note) + 2
		out = append(out, r)
	}
	return out, true
}

// EncodeRoomOp name+note（创建/加入——note 可空）
func EncodeRoomOp(name, note string) []byte {
	b := make([]byte, 2+len(name)+2+len(note))
	off := putStr(b, 0, name)
	putStr(b, off, note)
	return b
}

func DecodeRoomOp(b []byte) (string, string, bool) {
	name, off, ok := getStr(b, 0)
	if !ok {
		return "", "", false
	}
	note, _, ok := getStr(b, off)
	return name, note, ok
}

// EncodeRoomListResp errCode(1)+房间列表（加入/创建应答）
func EncodeRoomListResp(errCode uint8, rooms []RoomInfo) []byte {
	b := []byte{errCode}
	return append(b, EncodeRoomList(rooms)...)
}

// ---- 背包 ----

// BagItem 装备定义（4 件全解锁——服务端常量表）
type BagItem struct {
	ID   int
	Name string
	Desc string
}

// 装备表（T5 生效逻辑）
var BagItems = []BagItem{
	{0, "无", "不装备"},
	{1, "轻盈之靴", "二段跳高度 +0.3m"},
	{2, "疾风护腕", "跑步速度 +1"},
	{3, "扩容弹匣", "弹夹子弹 +4"},
	{4, "守护徽章", "格挡条上限 +10"},
}

// EncodeBagList curEquip(1) + count(1) + N×(id(1)+name(u16)+desc(u16))
func EncodeBagList(curEquip int, items []BagItem) []byte {
	b := []byte{uint8(curEquip), uint8(len(items))}
	for _, it := range items {
		b = append(b, uint8(it.ID))
		var tmp [4]byte
		binary.BigEndian.PutUint16(tmp[0:2], uint16(len(it.Name)))
		b = append(b, tmp[0:2]...)
		b = append(b, it.Name...)
		binary.BigEndian.PutUint16(tmp[0:2], uint16(len(it.Desc)))
		b = append(b, tmp[0:2]...)
		b = append(b, it.Desc...)
	}
	return b
}

// EncodeBagEquip equipID(1)
func EncodeBagEquip(equipID uint8) []byte {
	return []byte{equipID}
}

func DecodeBagEquip(b []byte) (int, bool) {
	if len(b) < 1 {
		return 0, false
	}
	return int(b[0]), true
}
