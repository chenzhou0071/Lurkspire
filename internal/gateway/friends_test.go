// friends_test.go — 好友系统集成：搜索/邀请/同意拒绝/双向列表/在线实时/离线邀请补发
package gateway

import (
	"testing"
	"time"

	"lurkspire/server/internal/protocol"
)

// loginOnly 已注册账号直接登录（跳过注册）
func loginOnly(t *testing.T, c *client, account, password string) *protocol.Frame {
	t.Helper()
	if err := c.send(protocol.MsgLogin, protocol.EncodeLogin(account, password)); err != nil {
		t.Fatal(err)
	}
	f, err := c.recv(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f.MsgID != protocol.MsgLoginResp || f.Body[0] != protocol.LobbyOK {
		t.Fatalf("login failed: msg=%d body=%v", f.MsgID, f.Body)
	}
	return f
}

// recvMsgID 读到指定消息（跳过其他帧——如登录后的好友列表推送）
func recvUntil(t *testing.T, c *client, wantMsg uint16, timeout time.Duration) *protocol.Frame {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.conn.SetReadDeadline(deadline)
		f, err := protocol.NewFrameReader(c.rd).Next()
		if err != nil {
			break
		}
		if f.MsgID == wantMsg {
			return f
		}
	}
	t.Fatalf("timeout waiting msg %d", wantMsg)
	return nil
}

// recvSkipping 跳过指定消息读下一帧
func recvSkipping(t *testing.T, c *client, skip uint16, timeout time.Duration) *protocol.Frame {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.conn.SetReadDeadline(deadline)
		f, err := protocol.NewFrameReader(c.rd).Next()
		if err != nil {
			break
		}
		if f.MsgID != skip {
			return f
		}
	}
	t.Fatalf("timeout skipping msg %d", skip)
	return nil
}

func TestFriend_FullFlow(t *testing.T) {
	srv, _ := startTestGateway(t)

	// A 和 B 注册登录
	a := dial(t, srv)
	registerAndLogin(t, a, "fa", "pw", "好友甲")
	b := dial(t, srv)
	registerAndLogin(t, b, "fb", "pw", "好友乙")

	// 登录后先收好友列表（初始空）
	fa := recvUntil(t, a, protocol.MsgFriendList, 2*time.Second)
	faList, _ := protocol.DecodeFriendList(fa.Body)
	if len(faList) != 0 {
		t.Fatal("A initial friend list should be empty")
	}
	recvUntil(t, b, protocol.MsgFriendList, 2*time.Second)

	// A 搜 B（按昵称）
	if err := a.send(protocol.MsgFriendSearch, protocol.EncodeRoomOp("好友乙", "")); err != nil {
		t.Fatal(err)
	}
	searchR := recvUntil(t, a, protocol.MsgFriendSearchR, 2*time.Second)
	fi, ok := protocol.DecodeFriendInfo(searchR.Body)
	if !ok || fi.Nickname != "好友乙" || !fi.Online {
		t.Fatalf("search result wrong: %+v ok=%v", fi, ok)
	}

	// A 邀请 B
	if err := a.send(protocol.MsgFriendInvite, protocol.EncodeUID(fi.UID)); err != nil {
		t.Fatal(err)
	}
	inv := recvUntil(t, b, protocol.MsgFriendInviteN, 2*time.Second)
	invFi, ok := protocol.DecodeFriendInfo(inv.Body)
	if !ok || invFi.Nickname != "好友甲" {
		t.Fatalf("invite push wrong: %+v ok=%v", invFi, ok)
	}

	// B 同意
	if err := b.send(protocol.MsgFriendAccept, protocol.EncodeUID(invFi.UID)); err != nil {
		t.Fatal(err)
	}
	// 双方收到更新后的好友列表
	lb := recvUntil(t, b, protocol.MsgFriendList, 2*time.Second)
	lbList, _ := protocol.DecodeFriendList(lb.Body)
	if len(lbList) != 1 || lbList[0].Nickname != "好友甲" || !lbList[0].Online {
		t.Fatalf("B friend list wrong: %+v", lbList)
	}
	la := recvUntil(t, a, protocol.MsgFriendList, 2*time.Second)
	laList, _ := protocol.DecodeFriendList(la.Body)
	if len(laList) != 1 || laList[0].Nickname != "好友乙" || !laList[0].Online {
		t.Fatalf("A friend list wrong: %+v", laList)
	}

	// A 断开 → B 收到 A 离线推送（B 的好友列表第一项 = 好友甲 = A）
	a.conn.Close()
	off := recvUntil(t, b, protocol.MsgFriendOnline, 2*time.Second)
	offUID, offOnline := protocol.DecodeOnlinePush(off.Body)
	if offUID != lbList[0].UID || offOnline {
		t.Fatalf("offline push wrong: uid=%d online=%v", offUID, offOnline)
	}
}

func TestFriend_OfflineInvite_DeliveredOnLogin(t *testing.T) {
	srv, _ := startTestGateway(t)

	// A 登录；C 未登录（先注册一个离线账号）
	c0 := dial(t, srv)
	registerAndLogin(t, c0, "fc", "pw", "好友丙")
	c0.conn.Close() // C 离线

	// A 搜到 C 并邀请（C 离线 → 排队）
	a := dial(t, srv)
	registerAndLogin(t, a, "fa2", "pw", "好友甲2")
	recvUntil(t, a, protocol.MsgFriendList, 2*time.Second)
	if err := a.send(protocol.MsgFriendSearch, protocol.EncodeRoomOp("好友丙", "")); err != nil {
		t.Fatal(err)
	}
	searchR := recvUntil(t, a, protocol.MsgFriendSearchR, 2*time.Second)
	fi, ok := protocol.DecodeFriendInfo(searchR.Body)
	if !ok || fi.Nickname != "好友丙" {
		t.Fatalf("search C wrong: %+v", fi)
	}
	if err := a.send(protocol.MsgFriendInvite, protocol.EncodeUID(fi.UID)); err != nil {
		t.Fatal(err)
	}

	// C 重新登录 → 申请栏应带出 A 的申请（持久化——离线不丢）
	c := dial(t, srv)
	loginOnly(t, c, "fc", "pw")
	pending := recvUntil(t, c, protocol.MsgFriendPendingList, 3*time.Second)
	pList, _ := protocol.DecodeFriendList(pending.Body)
	if len(pList) != 1 || pList[0].Nickname != "好友甲2" {
		t.Fatalf("pending list on login wrong: %+v", pList)
	}
	// B(我) 拒绝 → 申请消失（再收 PendingList 为空）
	if err := c.send(protocol.MsgFriendReject, protocol.EncodeUID(pList[0].UID)); err != nil {
		t.Fatal(err)
	}
	// 等 Reject 落库处理完再断开（服务端无应答——防止断连早于处理）
	time.Sleep(300 * time.Millisecond)
	// 发送方（A）在申请后无列表变化——接受方删条目由客户端本地做；
	// 服务端验证：重登后 PendingList 为空
	c.conn.Close()
	c2 := dial(t, srv)
	loginOnly(t, c2, "fc", "pw")
	pending2 := recvUntil(t, c2, protocol.MsgFriendPendingList, 3*time.Second)
	p2List, _ := protocol.DecodeFriendList(pending2.Body)
	if len(p2List) != 0 {
		t.Fatalf("after reject pending should be empty: %+v", p2List)
	}
}
