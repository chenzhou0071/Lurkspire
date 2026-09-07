// store.go — 数据存储抽象：账号/好友/装备（Mem 测试用 / MySQL 生产）
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

var (
	ErrAccountExists  = errors.New("store: account already exists")
	ErrNicknameExists = errors.New("store: nickname already exists")
	ErrNotFound       = errors.New("store: not found")
)

// User 账号记录
type User struct {
	ID           uint32
	Account      string
	PasswordHash string
	Nickname     string
	EquipID      int
}

// Store 存储接口（MemStore 供测试；MySQLStore 生产）
type Store interface {
	CreateUser(u *User) error // ErrAccountExists/ErrNicknameExists
	GetUserByAccount(account string) (*User, error)
	GetUserByNickname(nickname string) (*User, error) // 好友按昵称搜索
	GetUserByID(id uint32) (*User, error)
	SetEquip(id uint32, equip int) error
	// 好友（单向申请 + 双向生效——申请栏模式）
	InviteFriend(a, b uint32) error                // a 申请加 b（重复申请幂等）
	PendingFriendIDs(uid uint32) ([]uint32, error) // 我的待处理申请（别人申请我）
	AcceptFriend(uid, other uint32) error          // 我同意 other 的申请
	RejectFriend(uid, other uint32) error          // 我拒绝 other 的申请
	FriendIDs(uid uint32) ([]uint32, error)        // 好友（status=1 双向可见）
}

// ---- MemStore（测试/开发——重启丢）----

type MemStore struct {
	mu     sync.Mutex
	nextID uint32
	users  map[string]*User // account → user
	byNick map[string]*User // nickname → user（唯一）
	byID   map[uint32]*User
	// 好友关系：key = "a,b" → status（0 申请中/1 好友）
	friends map[string]int
}

func NewMemStore() *MemStore {
	return &MemStore{
		users:   make(map[string]*User),
		byNick:  make(map[string]*User),
		byID:    make(map[uint32]*User),
		friends: make(map[string]int),
	}
}

func (m *MemStore) CreateUser(u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.Account]; ok {
		return ErrAccountExists
	}
	if _, ok := m.byNick[u.Nickname]; ok {
		return ErrNicknameExists
	}
	m.nextID++
	u.ID = m.nextID
	cp := *u
	m.users[u.Account] = &cp
	m.byNick[u.Nickname] = &cp
	m.byID[u.ID] = &cp
	return nil
}

func (m *MemStore) GetUserByAccount(account string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[account]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *MemStore) GetUserByNickname(nickname string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byNick[nickname]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *MemStore) GetUserByID(id uint32) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *MemStore) SetEquip(id uint32, equip int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return ErrNotFound
	}
	u.EquipID = equip
	return nil
}

func fkey(a, b uint32) string { return fmt.Sprintf("%d,%d", a, b) }

// InviteFriend a 申请 b（幂等：已有关系/申请不覆盖）
func (m *MemStore) InviteFriend(a, b uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fkey(a, b)
	if _, ok := m.friends[key]; ok {
		return nil // 已有（申请中或好友）——幂等
	}
	m.friends[key] = 0
	return nil
}

// PendingFriendIDs 待处理申请（别人申请我 status=0）
func (m *MemStore) PendingFriendIDs(uid uint32) ([]uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []uint32
	for key, status := range m.friends {
		if status != 0 {
			continue
		}
		var a, b uint32
		fmt.Sscanf(key, "%d,%d", &a, &b)
		if b == uid {
			out = append(out, a)
		}
	}
	return out, nil
}

// AcceptFriend 我同意 other 的申请
func (m *MemStore) AcceptFriend(uid, other uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fkey(other, uid)
	if _, ok := m.friends[key]; ok {
		m.friends[key] = 1
	}
	return nil
}

// RejectFriend 我拒绝 other 的申请
func (m *MemStore) RejectFriend(uid, other uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.friends, fkey(other, uid))
	return nil
}

func (m *MemStore) FriendIDs(uid uint32) ([]uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []uint32
	for key, status := range m.friends {
		if status != 1 {
			continue
		}
		var a, b uint32
		fmt.Sscanf(key, "%d,%d", &a, &b)
		if a == uid {
			out = append(out, b)
		} else if b == uid {
			out = append(out, a)
		}
	}
	return out, nil
}

// ---- MySQLStore（生产）----

type MySQLStore struct {
	db *sql.DB
}

// OpenMySQL 连接（dsn 如 user:pass@tcp(127.0.0.1:3306)/lurkspire?parseTime=true）
func OpenMySQL(dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("mysql ping: %w", err)
	}
	return &MySQLStore{db: db}, nil
}

func (m *MySQLStore) Close() error { return m.db.Close() }

func (m *MySQLStore) CreateUser(u *User) error {
	// 插前预检（精确区分 account/nickname 冲突；唯一约束兜底并发）
	if _, err := m.GetUserByAccount(u.Account); err == nil {
		return ErrAccountExists
	}
	if _, err := m.GetUserByNickname(u.Nickname); err == nil {
		return ErrNicknameExists
	}
	res, err := m.db.Exec(
		"INSERT INTO users (account, password_hash, nickname) VALUES (?,?,?)",
		u.Account, u.PasswordHash, u.Nickname)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	u.ID = uint32(id)
	return nil
}

func (m *MySQLStore) GetUserByAccount(account string) (*User, error) {
	return m.scanRow(m.db.QueryRow(
		"SELECT id, account, password_hash, nickname, equip_id FROM users WHERE account=?", account))
}

func (m *MySQLStore) GetUserByNickname(nickname string) (*User, error) {
	return m.scanRow(m.db.QueryRow(
		"SELECT id, account, password_hash, nickname, equip_id FROM users WHERE nickname=?", nickname))
}

func (m *MySQLStore) GetUserByID(id uint32) (*User, error) {
	return m.scanRow(m.db.QueryRow(
		"SELECT id, account, password_hash, nickname, equip_id FROM users WHERE id=?", id))
}

func (m *MySQLStore) SetEquip(id uint32, equip int) error {
	_, err := m.db.Exec("UPDATE users SET equip_id=? WHERE id=?", equip, id)
	return err
}

func (m *MySQLStore) InviteFriend(a, b uint32) error {
	_, err := m.db.Exec(
		"INSERT IGNORE INTO friends (user_a, user_b, status) VALUES (?,?,0)", a, b)
	return err
}

func (m *MySQLStore) PendingFriendIDs(uid uint32) ([]uint32, error) {
	rows, err := m.db.Query(
		"SELECT user_a FROM friends WHERE user_b=? AND status=0", uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint32
	for rows.Next() {
		var f uint32
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (m *MySQLStore) AcceptFriend(uid, other uint32) error {
	_, err := m.db.Exec(
		"UPDATE friends SET status=1 WHERE user_a=? AND user_b=? AND status=0", other, uid)
	return err
}

func (m *MySQLStore) RejectFriend(uid, other uint32) error {
	_, err := m.db.Exec(
		"DELETE FROM friends WHERE user_a=? AND user_b=? AND status=0", other, uid)
	return err
}

func (m *MySQLStore) FriendIDs(uid uint32) ([]uint32, error) {
	rows, err := m.db.Query(
		"SELECT IF(user_a=?, user_b, user_a) FROM friends WHERE status=1 AND (user_a=? OR user_b=?)",
		uid, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint32
	for rows.Next() {
		var f uint32
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (m *MySQLStore) scanRow(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Account, &u.PasswordHash, &u.Nickname, &u.EquipID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}
