// bag_test.go — 背包：装备列表/切换持久/非法拒绝/房间格挡上限生效
package gateway

import (
	"testing"
	"time"

	"lurkspire/server/internal/lobby"
	"lurkspire/server/internal/protocol"
	"lurkspire/server/internal/room"
	"lurkspire/server/internal/store"
)

func TestBag_ListAndEquip(t *testing.T) {
	srv, _ := startTestGateway(t)
	c := dial(t, srv)
	registerAndLogin(t, c, "ba1", "pw", "背包甲")
	recvUntil(t, c, protocol.MsgFriendList, 2*time.Second) // 登录推送排空

	// 请求背包列表
	if err := c.send(protocol.MsgBagList, nil); err != nil {
		t.Fatal(err)
	}
	f := recvUntil(t, c, protocol.MsgBagList, 2*time.Second)
	cur := f.Body[0]
	if cur != 0 {
		t.Fatalf("default equip: want 0, got %d", cur)
	}
	if len(f.Body) < 2 || int(f.Body[1]) != 5 {
		t.Fatalf("bag should list 5 items (含无): %v", f.Body)
	}

	// 装备 4（守护徽章）
	if err := c.send(protocol.MsgBagEquip, protocol.EncodeBagEquip(4)); err != nil {
		t.Fatal(err)
	}
	f2 := recvUntil(t, c, protocol.MsgBagList, 2*time.Second)
	if f2.Body[0] != 4 {
		t.Fatalf("equip 4 failed: %v", f2.Body)
	}

	// 重登保持（持久化）
	c.conn.Close()
	c2 := dial(t, srv)
	loginOnly(t, c2, "ba1", "pw")
	if err := c2.send(protocol.MsgBagList, nil); err != nil {
		t.Fatal(err)
	}
	f3 := recvUntil(t, c2, protocol.MsgBagList, 2*time.Second)
	if f3.Body[0] != 4 {
		t.Fatalf("equip not persisted: %v", f3.Body)
	}

	// 非法装备 99 → 拒绝
	if err := c2.send(protocol.MsgBagEquip, protocol.EncodeBagEquip(99)); err != nil {
		t.Fatal(err)
	}
	fe := recvUntil(t, c2, protocol.MsgLoginResp, 2*time.Second)
	if fe.Body == nil || len(fe.Body) < 1 || fe.Body[0] != protocol.LobbyErrEquip {
		t.Fatalf("illegal equip should reject: %v", fe.Body)
	}
}

func TestBag_Equip4_RaisesBlockMaxInRoom(t *testing.T) {
	m := room.NewManager(room.Config{MaxPlayers: 8, TickHz: 30})
	svc := lobby.NewService(store.NewMemStore(), "test-secret")
	hub := NewHub(m, svc)
	srv := NewServer("127.0.0.1:0", hub)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	defer srv.Close()

	c := dial(t, srv)
	registerAndLogin(t, c, "bb1", "pw", "徽章乙")
	recvUntil(t, c, protocol.MsgFriendList, 2*time.Second)
	// 装备 4 → 建房（进房应用上限）
	if err := c.send(protocol.MsgBagEquip, protocol.EncodeBagEquip(4)); err != nil {
		t.Fatal(err)
	}
	recvUntil(t, c, protocol.MsgBagList, 2*time.Second)
	if err := c.send(protocol.MsgRoomCreate, protocol.EncodeRoomOp("bgroom", "")); err != nil {
		t.Fatal(err)
	}
	recvUntil(t, c, protocol.MsgBattleJoinOK, 2*time.Second)

	// 直接进房内查玩家格挡上限（110）
	roomName := "bgroom"
	r, _ := m.GetRoom(roomName)
	if r == nil {
		t.Fatal("room missing")
	}
	// 等待应用（channel 异步）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := r.StateSnapshot()
		if len(st) == 1 && st[0].Block > 100.5 {
			if st[0].Block != 110 {
				t.Fatalf("block max with equip4: want 110, got %v", st[0].Block)
			}
			return // ✓ 装备生效
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("equip4 block max not applied in room")
}
