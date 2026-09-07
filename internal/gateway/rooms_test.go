// rooms_test.go — 房间列表大厅：创建/列表广播/加入/离开散房/同房名/满员
package gateway

import (
	"testing"
	"time"

	"lurkspire/server/internal/protocol"
)

// createRoom 建房（221）并等 JoinOK
func createRoom(t *testing.T, c *client, name, note string) {
	t.Helper()
	if err := c.send(protocol.MsgRoomCreate, protocol.EncodeRoomOp(name, note)); err != nil {
		t.Fatal(err)
	}
	recvUntil(t, c, protocol.MsgBattleJoinOK, 2*time.Second)
}

func TestRoom_Create_List_Join(t *testing.T) {
	srv, _ := startTestGateway(t)

	a := dial(t, srv)
	registerAndLogin(t, a, "ra", "pw", "房主甲")
	recvUntil(t, a, protocol.MsgRoomList, 2*time.Second) // 登录下发空列表
	b := dial(t, srv)
	registerAndLogin(t, b, "rb", "pw", "路人乙")
	recvUntil(t, b, protocol.MsgRoomList, 2*time.Second)

	// A 建房（备注）→ 应收到广播的列表（含新房）
	createRoom(t, a, "room1", "甲的房")
	// A 建房后广播残留（A 自己缓冲）——先消费再验证 B 视角
	recvUntil(t, a, protocol.MsgRoomList, 2*time.Second)
	rl := recvUntil(t, b, protocol.MsgRoomList, 2*time.Second)
	list, _ := protocol.DecodeRoomList(rl.Body)
	if len(list) != 1 || list[0].Name != "room1" ||
		list[0].Creator != "房主甲" || list[0].Note != "甲的房" || list[0].Players != 1 {
		t.Fatalf("room list after create wrong: %+v", list)
	}

	// B 加入 → 列表人数 2（跳过 A 缓冲里可能的旧 RoomList——读到人数 2 为止）
	if err := b.send(protocol.MsgRoomJoin, protocol.EncodeRoomOp("room1", "")); err != nil {
		t.Fatal(err)
	}
	recvUntil(t, b, protocol.MsgBattleJoinOK, 2*time.Second)
	var list2 []protocol.RoomInfo
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rl2 := recvUntil(t, a, protocol.MsgRoomList, 2*time.Second)
		list2, _ = protocol.DecodeRoomList(rl2.Body)
		if len(list2) == 1 && list2[0].Players == 2 {
			break // 等到人数 2 的广播
		}
	}
	if len(list2) != 1 || list2[0].Players != 2 {
		t.Fatalf("room players after join wrong: %+v", list2)
	}

	// B 出房 → 人数回 1
	if err := b.send(protocol.MsgRoomLeave, nil); err != nil {
		t.Fatal(err)
	}
	rl3 := recvUntil(t, a, protocol.MsgRoomList, 2*time.Second)
	list3, _ := protocol.DecodeRoomList(rl3.Body)
	if len(list3) != 1 || list3[0].Players != 1 {
		t.Fatalf("room players after leave wrong: %+v", list3)
	}

	// A 也出房 → 空房散 → 列表空（循环跳过旧帧——等空列表）
	if err := a.send(protocol.MsgRoomLeave, nil); err != nil {
		t.Fatal(err)
	}
	var list4 []protocol.RoomInfo
	deadline2 := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline2) {
		rl4 := recvUntil(t, b, protocol.MsgRoomList, 2*time.Second)
		list4, _ = protocol.DecodeRoomList(rl4.Body)
		if len(list4) == 0 {
			break // 等到空列表（房已散）
		}
	}
	if len(list4) != 0 {
		t.Fatalf("room should be destroyed after all leave: %+v", list4)
	}
}

func TestRoom_Create_DuplicateName_Rejected(t *testing.T) {
	srv, _ := startTestGateway(t)
	a := dial(t, srv)
	registerAndLogin(t, a, "rd1", "pw", "甲")
	recvUntil(t, a, protocol.MsgRoomList, 2*time.Second)
	createRoom(t, a, "dup", "first")
	b := dial(t, srv)
	registerAndLogin(t, b, "rd2", "pw", "乙")
	recvUntil(t, b, protocol.MsgRoomList, 2*time.Second)
	// B 建同名 → 拒绝（错误应答 errCode RoomExists）
	if err := b.send(protocol.MsgRoomCreate, protocol.EncodeRoomOp("dup", "")); err != nil {
		t.Fatal(err)
	}
	f := recvUntil(t, b, protocol.MsgLoginResp, 2*time.Second) // sendErr 用 LoginResp 号
	if f.Body == nil || len(f.Body) < 1 || f.Body[0] != protocol.LobbyErrRoomExists {
		t.Fatalf("duplicate room name should reject: body=%v", f.Body)
	}
}

func TestRoom_JoinMissing_Rejected(t *testing.T) {
	srv, _ := startTestGateway(t)
	a := dial(t, srv)
	registerAndLogin(t, a, "re1", "pw", "甲")
	recvUntil(t, a, protocol.MsgRoomList, 2*time.Second)
	if err := a.send(protocol.MsgRoomJoin, protocol.EncodeRoomOp("no_such_room", "")); err != nil {
		t.Fatal(err)
	}
	f := recvUntil(t, a, protocol.MsgLoginResp, 2*time.Second)
	if f.Body == nil || len(f.Body) < 1 || f.Body[0] != protocol.LobbyErrRoomMissing {
		t.Fatalf("join missing room should reject: body=%v", f.Body)
	}
}
