// service_test.go — 账号服务：注册/重名/登录错密码/Token 校验/装备
package lobby

import (
	"strings"
	"testing"

	"lurkspire/server/internal/store"
)

func newTestService() *Service {
	return NewService(store.NewMemStore(), "test-secret")
}

func TestRegister_Login_OK(t *testing.T) {
	s := newTestService()
	uid, err := s.Register("alice", "pass123", "爱丽丝")
	if err != nil || uid == 0 {
		t.Fatalf("register: uid=%d err=%v", uid, err)
	}
	token, err := s.Login("alice", "pass123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	got, err := s.Verify(token)
	if err != nil || got != uid {
		t.Fatalf("verify: uid=%d err=%v", got, err)
	}
	nick, _ := s.Nickname(uid)
	if nick != "爱丽丝" {
		t.Fatalf("nickname: got %q", nick)
	}
}

func TestRegister_Duplicate_Rejected(t *testing.T) {
	s := newTestService()
	s.Register("bob", "pw1", "鲍勃")
	if _, err := s.Register("bob", "pw2", "另一个"); err != ErrAccountExists {
		t.Fatalf("duplicate register: want ErrAccountExists, got %v", err)
	}
	// 昵称唯一（不同账号同昵称也拒绝）
	if _, err := s.Register("bob2", "pw", "鲍勃"); err != ErrNicknameExists {
		t.Fatalf("duplicate nickname: want ErrNicknameExists, got %v", err)
	}
}

func TestSearchByNickname_FindUser(t *testing.T) {
	s := newTestService()
	s.Register("frank", "pw", "弗兰克")
	u, err := s.SearchByNickname("弗兰克")
	if err != nil || u.Nickname != "弗兰克" {
		t.Fatalf("search by nickname: %+v err=%v", u, err)
	}
	if _, err := s.SearchByNickname("不存在的人"); err != ErrBadAccount {
		t.Fatalf("search missing: want ErrBadAccount, got %v", err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	s := newTestService()
	s.Register("carol", "right", "卡罗尔")
	if _, err := s.Login("carol", "wrong"); err != ErrBadPassword {
		t.Fatalf("wrong password: want ErrBadPassword, got %v", err)
	}
	if _, err := s.Login("nobody", "x"); err != ErrBadAccount {
		t.Fatalf("unknown account: want ErrBadAccount, got %v", err)
	}
}

func TestVerify_BadToken(t *testing.T) {
	s := newTestService()
	if _, err := s.Verify("garbage!!"); err != ErrBadToken {
		t.Fatalf("garbage token: want ErrBadToken, got %v", err)
	}
	// 篡改 token（换 uid 但签名不匹配——我们的格式签名固定位——截断长度不对即拒）
	s.Register("dave", "pw", "戴夫")
	tok, _ := s.Login("dave", "pw")
	if _, err := s.Verify(tok[:len(tok)-4] + "AAAA"); err != ErrBadToken {
		t.Fatalf("tampered token: want ErrBadToken, got %v", err)
	}
}

func TestRegister_EmptyInput_Rejected(t *testing.T) {
	s := newTestService()
	if _, err := s.Register("", "pw", "昵称"); err != ErrBadInput {
		t.Fatalf("empty account: want ErrBadInput, got %v", err)
	}
	if _, err := s.Register("acc", "pw", ""); err != ErrBadInput {
		t.Fatalf("empty nickname: want ErrBadInput, got %v", err)
	}
	if _, err := s.Register("acc", "pw", strings.Repeat("长", 40)); err != ErrBadInput {
		t.Fatalf("long nickname: want ErrBadInput, got %v", err)
	}
}

func TestEquip_SetAndGet(t *testing.T) {
	s := newTestService()
	uid, _ := s.Register("eve", "pw", "伊芙")
	if e, _ := s.EquipID(uid); e != 0 {
		t.Fatalf("default equip: want 0, got %d", e)
	}
	if err := s.SetEquip(uid, 4); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.EquipID(uid); e != 4 {
		t.Fatalf("equip after set: want 4, got %d", e)
	}
}

func TestFriends_MemStore(t *testing.T) {
	st := store.NewMemStore()
	a := &store.User{Account: "a", PasswordHash: "x", Nickname: "A"}
	b := &store.User{Account: "b", PasswordHash: "x", Nickname: "B"}
	st.CreateUser(a)
	st.CreateUser(b)
	// a 申请 b → b 的申请栏有 a（好友还没有）
	if err := st.InviteFriend(a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	pending, _ := st.PendingFriendIDs(b.ID)
	if len(pending) != 1 || pending[0] != a.ID {
		t.Fatalf("pending list wrong: %v", pending)
	}
	if fa, _ := st.FriendIDs(a.ID); len(fa) != 0 {
		t.Fatalf("not friend yet: %v", fa)
	}
	// b 同意 → 双向好友
	if err := st.AcceptFriend(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	fa, _ := st.FriendIDs(a.ID)
	fb, _ := st.FriendIDs(b.ID)
	if len(fa) != 1 || fa[0] != b.ID || len(fb) != 1 || fb[0] != a.ID {
		t.Fatalf("friend bidirectional broken: a=%v b=%v", fa, fb)
	}
	// b 拒绝后 a 的申请消失（新申请 c→b 拒绝）
	c := &store.User{Account: "c", PasswordHash: "x", Nickname: "C"}
	st.CreateUser(c)
	st.InviteFriend(c.ID, b.ID)
	if err := st.RejectFriend(b.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if p2, _ := st.PendingFriendIDs(b.ID); len(p2) != 0 {
		t.Fatalf("rejected pending should vanish: %v", p2)
	}
}
