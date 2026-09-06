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
	ErrAccountExists = errors.New("lobby: account exists")
	ErrBadAccount    = errors.New("lobby: account not found")
	ErrBadPassword   = errors.New("lobby: wrong password")
	ErrBadToken      = errors.New("lobby: invalid token")
	ErrBadInput      = errors.New("lobby: bad input")
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

// Register 注册：账号唯一 + 密码 bcrypt + 昵称必填
func (s *Service) Register(account, password, nickname string) (uint64, error) {
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
		return 0, err
	}
	return u.ID, nil
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
func (s *Service) Verify(token string) (uint64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 16+sha256.Size {
		return 0, ErrBadToken
	}
	// HMAC 签名校验（篡改 uid/过期时间都会失配）
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(raw[:16])
	if !hmac.Equal(mac.Sum(nil), raw[16:]) {
		return 0, ErrBadToken
	}
	uid := binary.BigEndian.Uint64(raw[0:8])
	expire := int64(binary.BigEndian.Uint64(raw[8:16]))
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
func (s *Service) Nickname(uid uint64) (string, error) {
	u, err := s.store.GetUserByID(uid)
	if err != nil {
		return "", ErrBadAccount
	}
	return u.Nickname, nil
}

// EquipID 当前装备（进对局用）
func (s *Service) EquipID(uid uint64) (int, error) {
	u, err := s.store.GetUserByID(uid)
	if err != nil {
		return 0, ErrBadAccount
	}
	return u.EquipID, nil
}

// SetEquip 切换装备（T5 用——先留接口）
func (s *Service) SetEquip(uid uint64, equip int) error {
	return s.store.SetEquip(uid, equip)
}

// sign HMAC 签名 Token：base64(uid(8) | expire(8) | hmac(16))
func (s *Service) sign(uid uint64) (string, error) {
	expire := time.Now().Add(tokenTTL).Unix()
	payload := make([]byte, 16)
	binary.BigEndian.PutUint64(payload[0:8], uid)
	binary.BigEndian.PutUint64(payload[8:16], uint64(expire))
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(payload)
	out := make([]byte, 16+mac.Size())
	copy(out, payload)
	copy(out[16:], mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(out), nil
}
