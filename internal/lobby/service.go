// service.go — 账号服务：注册/登录/Token 签发校验（bcrypt + HMAC）
package lobby

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"lurkspire/server/internal/store"
)

var (
	ErrAccountExists  = errors.New("lobby: account exists")
	ErrNicknameExists = errors.New("lobby: nickname exists")
	ErrBadAccount     = errors.New("lobby: account not found")
	ErrBadPassword    = errors.New("lobby: wrong password")
	ErrBadToken       = errors.New("lobby: invalid token")
	ErrBadInput       = errors.New("lobby: bad input")
)

// Token 有效期（M3 演示 24h 够用）
const tokenTTL = 24 * time.Hour

// Service 账号服务
type Service struct {
	store  store.Store
	secret []byte
}

func NewService(s store.Store, secret string) *Service {
	return &Service{store: s, secret: []byte(secret)}
}

// Register 注册：账号唯一 + 昵称唯一（好友搜索凭据）+ 密码 bcrypt
func (s *Service) Register(account, password, nickname string) (uint32, error) {
	if account == "" || len(account) > 64 || password == "" || nickname == "" || len(nickname) > 32 {
		return 0, ErrBadInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	u := &store.User{Account: account, PasswordHash: string(hash), Nickname: nickname}
	if err := s.store.CreateUser(u); err != nil {
		if err == store.ErrAccountExists {
			return 0, ErrAccountExists
		}
		if err == store.ErrNicknameExists {
			return 0, ErrNicknameExists
		}
		return 0, err
	}
	return u.ID, nil
}

// SearchByNickname 按昵称搜索用户（加好友凭据——昵称唯一）
func (s *Service) SearchByNickname(nickname string) (*store.User, error) {
	u, err := s.store.GetUserByNickname(nickname)
	if err != nil {
		return nil, ErrBadAccount
	}
	return u, nil
}

// InviteFriend a 申请加 b（申请栏模式——持久化待处理）
func (s *Service) InviteFriend(a, b uint32) error {
	return s.store.InviteFriend(a, b)
}

// PendingFriendIDs 我的待处理申请
func (s *Service) PendingFriendIDs(uid uint32) ([]uint32, error) {
	return s.store.PendingFriendIDs(uid)
}

// AcceptFriend 同意申请（双向好友）
func (s *Service) AcceptFriend(uid, other uint32) error {
	return s.store.AcceptFriend(uid, other)
}

// RejectFriend 拒绝申请
func (s *Service) RejectFriend(uid, other uint32) error {
	return s.store.RejectFriend(uid, other)
}

// FriendIDs 好友 uid 列表（双向生效）
func (s *Service) FriendIDs(uid uint32) ([]uint32, error) {
	return s.store.FriendIDs(uid)
}

// Login 登录：校验密码 → 发 Token（HMAC(uid|expire)）
func (s *Service) Login(account, password string) (string, error) {
	u, err := s.store.GetUserByAccount(account)
	if err != nil {
		if err == store.ErrNotFound {
			return "", ErrBadAccount
		}
		return "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", ErrBadPassword
	}
	return s.sign(u.ID)
}

// Verify Token → uid（校验 HMAC 签名——防篡改）
func (s *Service) Verify(token string) (uint32, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 12+sha256.Size {
		return 0, ErrBadToken
	}
	// HMAC 签名校验（篡改 uid/过期时间都会失配）
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(raw[:12])
	if !hmac.Equal(mac.Sum(nil), raw[12:]) {
		return 0, ErrBadToken
	}
	uid := binary.BigEndian.Uint32(raw[0:4])
	expire := int64(binary.BigEndian.Uint64(raw[4:12]))
	if expire < time.Now().Unix() {
		return 0, ErrBadToken
	}
	// 校验存在
	if _, err := s.store.GetUserByID(uid); err != nil {
		return 0, ErrBadToken
	}
	return uid, nil
}

// Nickname 昵称（会话展示）
func (s *Service) Nickname(uid uint32) (string, error) {
	u, err := s.store.GetUserByID(uid)
	if err != nil {
		return "", ErrBadAccount
	}
	return u.Nickname, nil
}

// EquipID 当前装备（进对局用）
func (s *Service) EquipID(uid uint32) (int, error) {
	u, err := s.store.GetUserByID(uid)
	if err != nil {
		return 0, ErrBadAccount
	}
	return u.EquipID, nil
}

// SetEquip 切换装备（T5 用——先留接口）
func (s *Service) SetEquip(uid uint32, equip int) error {
	return s.store.SetEquip(uid, equip)
}

// sign HMAC 签名 Token：base64(uid(4) | expire(8) | hmac(32))
func (s *Service) sign(uid uint32) (string, error) {
	expire := time.Now().Add(tokenTTL).Unix()
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uid)
	binary.BigEndian.PutUint64(payload[4:12], uint64(expire))
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(payload)
	out := make([]byte, 12+mac.Size())
	copy(out, payload)
	copy(out[12:], mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(out), nil
}
