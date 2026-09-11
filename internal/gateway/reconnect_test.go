// reconnect_test.go — 断线恢复：Token 重连/宽限内房间恢复/心跳超时踢出
package gateway

import (
	"testing"
	"time"

	"lurkspire/server/internal/lobby"
	"lurkspire/server/internal/protocol"
	"lurkspire/server/internal/room"
	"lurkspire/server/internal/store"
)

// loginAndToken 注册登录并解析出 token
func loginAndToken(t *testing.T, c *client, account, password, nickname string) (uint32, string) {
	t.Helper()
	f := registerAndLogin(t, c, account, password, nickname)
	errCode, token, uid, _, ok := protocol.DecodeLoginResp(f.Body)
	if !ok || errCode != protocol.LobbyOK || token == "" {
		t.Fatalf("login resp parse: err=%d token=%q ok=%v", errCode, token, ok)
	}
	return uid, token
}

func TestGateway_Reconnect_TokenRestores(t *testing.T) {
	srv, _ := startTestGateway(t)
	c1 := dial(t, srv)
	uid, token := loginAndToken(t, c1, "rc1", "pw", "重连甲")

	// 模拟掉线（不发良好再见——直接关连接）
	c1.conn.Close()
	time.Sleep(100 * time.Millisecond)

	// 新连接用 token 免密重连
	c2 := dial(t, srv)
	if err := c2.send(protocol.MsgReconnect, protocol.EncodeReconnect(token)); err != nil {
		t.Fatal(err)
	}
	f := recvUntil(t, c2, protocol.MsgLoginResp, 3*time.Second)
	errCode, _, uid2, nick, ok := protocol.DecodeLoginResp(f.Body)
	if !ok || errCode != protocol.LobbyOK || uid2 != uid || nick != "重连甲" {
		t.Fatalf("reconnect resp wrong: err=%d uid=%d/%d nick=%s ok=%v", errCode, uid2, uid, nick, ok)
	}
	// 登录后推送（背包等）照常下发
	recvUntil(t, c2, protocol.MsgBagList, 2*time.Second)
}

func TestGateway_Reconnect_BadToken_Rejected(t *testing.T) {
	srv, _ := startTestGateway(t)
	c := dial(t, srv)
	if err := c.send(protocol.MsgReconnect, protocol.EncodeReconnect("garbage-token")); err != nil {
		t.Fatal(err)
	}
	f := recvUntil(t, c, protocol.MsgLoginResp, 3*time.Second)
	// 错误应答 = errCode(1)+uid(4)——首字节即错误码（0=OK 表示被错误接受）
	if len(f.Body) == 0 || f.Body[0] == protocol.LobbyOK {
		t.Fatalf("bad token should be rejected: body=%v", f.Body)
	}
}

func TestGateway_Reconnect_RoomGraceRestores(t *testing.T) {
	srv, _ := startTestGateway(t)
	// c1 建房并进对局
	c1 := dial(t, srv)
	_, tok1 := loginAndToken(t, c1, "rg1", "pw", "宽限甲")
	c1.join(t, "arena")
	// c2 加入同房
	c2 := dial(t, srv)
	loginAndToken(t, c2, "rg2", "pw", "宽限乙")
	c2.join(t, "arena")

	// c1 掉线 → 宽限内玩家保留（c2 的 State 仍应含 2 人）
	c1.conn.Close()
	time.Sleep(300 * time.Millisecond)
	f := recvUntil(t, c2, protocol.MsgBattleState, 3*time.Second)
	states := protocol.DecodeState(f.Body)
	if len(states) != 2 {
		t.Fatalf("offline player should stay in grace: %d players", len(states))
	}

	// c1 新连接 Token 重连 + 重回原房
	c1b := dial(t, srv)
	if err := c1b.send(protocol.MsgReconnect, protocol.EncodeReconnect(tok1)); err != nil {
		t.Fatal(err)
	}
	recvUntil(t, c1b, protocol.MsgLoginResp, 3*time.Second)
	c1b.join(t, "arena") // 恢复（AddPlayer 幂等——位置/血量/分数保留）
	// 重连后仍收得到房间广播
	recvUntil(t, c1b, protocol.MsgBattleState, 3*time.Second)
}

func TestGateway_IdleTimeout_Kick(t *testing.T) {
	m := room.NewManager(room.Config{MaxPlayers: 8, TickHz: 30})
	svc := lobby.NewService(store.NewMemStore(), "test-secret")
	hub := NewHub(m, svc)
	srv := NewServer("127.0.0.1:0", hub)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	srv.SetIdleTimeout(120*time.Millisecond, 40*time.Millisecond) // 测试短超时
	go srv.Serve()
	t.Cleanup(srv.Close)

	c := dial(t, srv)
	time.Sleep(400 * time.Millisecond) // 不发任何帧——等待超时踢出
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := protocol.NewFrameReader(c.rd).Next(); err == nil {
		t.Fatal("connection should be closed by idle timeout")
	}
}
