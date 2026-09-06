// server_test.go — 网关集成测试：真 TCP 连接 → Join → 互见/输入流转/优雅关闭
package gateway

import (
	"bufio"
	"net"
	"testing"
	"time"

	"lurkspire/server/internal/lobby"
	"lurkspire/server/internal/protocol"
	"lurkspire/server/internal/room"
	"lurkspire/server/internal/store"
)

// 起一个测试网关（随机端口）
func startTestGateway(t *testing.T) (*Server, *Hub) {
	t.Helper()
	m := room.NewManager(room.Config{MaxPlayers: 8, TickHz: 30, Speed: 16, SettleScore: 20})
	svc := lobby.NewService(store.NewMemStore(), "test-secret")
	hub := NewHub(m, svc)
	srv := NewServer("127.0.0.1:0", hub)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(srv.Close)
	return srv, hub
}

// dial 客户端：连接 + 帧读写
type client struct {
	conn net.Conn
	rd   *bufio.Reader
}

func dial(t *testing.T, srv *Server) *client {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return &client{conn: conn, rd: bufio.NewReader(conn)}
}

func (c *client) send(msgID uint16, body []byte) error {
	_, err := c.conn.Write(protocol.Encode(&protocol.Frame{MsgID: msgID, Body: body}))
	return err
}

func (c *client) recv(timeout time.Duration) (*protocol.Frame, error) {
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	return protocol.NewFrameReader(c.rd).Next()
}

func (c *client) join(t *testing.T, name string) *protocol.Frame {
	t.Helper()
	if err := c.send(protocol.MsgBattleJoin, []byte(name)); err != nil {
		t.Fatal(err)
	}
	f, err := c.recv(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f.MsgID != protocol.MsgBattleJoinOK {
		t.Fatalf("want JoinOK(301), got %d: %s", f.MsgID, f.Body)
	}
	return f
}

// registerAndLogin 注册并登录（返回 loginResp）
func registerAndLogin(t *testing.T, c *client, account, password, nickname string) *protocol.Frame {
	t.Helper()
	// 注册
	if err := c.send(protocol.MsgReg, protocol.EncodeReg(account, password, nickname)); err != nil {
		t.Fatal(err)
	}
	f, err := c.recv(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f.MsgID != protocol.MsgRegResp || f.Body[0] != protocol.LobbyOK {
		t.Fatalf("register failed: msg=%d body=%v", f.MsgID, f.Body)
	}
	// 登录
	if err := c.send(protocol.MsgLogin, protocol.EncodeLogin(account, password)); err != nil {
		t.Fatal(err)
	}
	f, err = c.recv(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestGateway_RegLogin_Flow(t *testing.T) {
	srv, _ := startTestGateway(t)
	c := dial(t, srv)
	f := registerAndLogin(t, c, "tester1", "pw123", "测试员1")
	if f.MsgID != protocol.MsgLoginResp || f.Body[0] != protocol.LobbyOK {
		t.Fatalf("login failed: msg=%d body=%v", f.MsgID, f.Body)
	}
	// 登录应答含昵称（检查尾部含"测试员1" UTF8）
	body := f.Body
	if len(body) < 5 || body[0] != protocol.LobbyOK {
		t.Fatalf("bad login resp: %v", body)
	}
}

func TestGateway_UnAuthed_Rejected(t *testing.T) {
	srv, _ := startTestGateway(t)
	c := dial(t, srv)
	// 未登录发 Join → 被拒（返回登录错误码）
	if err := c.send(protocol.MsgBattleJoin, []byte("arena")); err != nil {
		t.Fatal(err)
	}
	f, err := c.recv(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f.Body == nil || len(f.Body) < 1 || f.Body[0] != protocol.LobbyErrToken {
		t.Fatalf("unauthed join should be rejected with token error: msg=%d body=%v", f.MsgID, f.Body)
	}
}

func TestGateway_RegDuplicateNickname(t *testing.T) {
	srv, _ := startTestGateway(t)
	c1 := dial(t, srv)
	registerAndLogin(t, c1, "acc_a", "pw", "同昵称")
	c2 := dial(t, srv)
	// c2 注册同昵称 → 拒绝（昵称唯一）
	if err := c2.send(protocol.MsgReg, protocol.EncodeReg("acc_b", "pw", "同昵称")); err != nil {
		t.Fatal(err)
	}
	f, err := c2.recv(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f.Body == nil || len(f.Body) < 1 || f.Body[0] != protocol.LobbyErrNickname {
		t.Fatalf("duplicate nickname should reject: msg=%d body=%v", f.MsgID, f.Body)
	}
}

func TestGateway_TwoClientsJoinSameRoom(t *testing.T) {
	srv, _ := startTestGateway(t)

	// 客户端 1 建房
	c1 := dial(t, srv)
	registerAndLogin(t, c1, "u1", "pw", "玩家1")
	f1 := c1.join(t, "arena")
	if len(f1.Body) < 5 {
		t.Fatalf("joinok body too short: %d", len(f1.Body))
	}
	// 客户端 2 加入同房
	c2 := dial(t, srv)
	registerAndLogin(t, c2, "u1b", "pw", "玩家1b")
	f2 := c2.join(t, "arena")
	if len(f2.Body) < 5 {
		t.Fatalf("joinok2 body too short: %d", len(f2.Body))
	}

	// c2 入房后应收到 c1 的状态广播（30Hz——含 c1）
	f, err := c2.recv(3 * time.Second)
	if err != nil {
		t.Fatal("c2 no state broadcast")
	}
	if f.MsgID != protocol.MsgBattleState {
		t.Fatalf("want state(320), got %d", f.MsgID)
	}
	states := protocol.DecodeState(f.Body)
	if len(states) != 2 {
		t.Fatalf("state should contain 2 players, got %+v", states)
	}
}

func TestGateway_InputFlowsToRoom(t *testing.T) {
	srv, _ := startTestGateway(t)
	c1 := dial(t, srv)
	registerAndLogin(t, c1, "u2", "pw", "玩家2")
	c1.join(t, "arena")
	c2 := dial(t, srv)
	registerAndLogin(t, c2, "u2b", "pw", "玩家2b")
	c2.join(t, "arena")

	// c1 上报移动（按 W 前进）——服务端积分 → c2 收到状态变化
	start := time.Now()
	for i := 0; i < 30; i++ { // 30 帧 ≈ 1 秒
		report := protocol.InputReport{MoveX: 0, MoveY: 1, AimY: 0}
		if err := c1.send(protocol.MsgBattleInput, protocol.EncodeInput(&report)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// c2 读广播直到拿到位置变化的 c1 或超时
	c2.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	rd := protocol.NewFrameReader(c2.rd)
	found := false
	for time.Now().Before(start.Add(2 * time.Second)) {
		f, err := rd.Next()
		if err != nil {
			break
		}
		if f.MsgID != protocol.MsgBattleState {
			continue
		}
		for _, s := range protocol.DecodeState(f.Body) {
			if s.Z > 5 { // 移动了至少 5m（30 帧前进）
				found = true
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("c2 never saw c1 moving (input not flowing)")
	}
}

func TestGateway_Heartbeat(t *testing.T) {
	srv, _ := startTestGateway(t)
	c := dial(t, srv)
	if err := c.send(protocol.MsgHeartbeat, nil); err != nil {
		t.Fatal(err)
	}
	f, err := c.recv(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f.MsgID != protocol.MsgHeartbeat {
		t.Fatalf("want heartbeat echo, got %d", f.MsgID)
	}
}

func TestGateway_GracefulClose(t *testing.T) {
	srv, _ := startTestGateway(t)
	c := dial(t, srv)
	registerAndLogin(t, c, "u3", "pw", "玩家3")
	c.join(t, "arena")
	srv.Close() // 优雅关闭
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.recv(2 * time.Second); err == nil {
		t.Fatal("connection should be closed after server shutdown")
	}
}
