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
	ErrAccountExists = errors.New("store: account already exists")
	ErrNotFound      = errors.New("store: not found")
)

// User 账号记录
type User struct {
	ID           uint64
	Account      string
	PasswordHash string
	Nickname     string
	EquipID      int
}

// Store 存储接口（MemStore 供测试；MySQLStore 生产）
type Store interface {
	CreateUser(u *User) error              // ErrAccountExists 重名
	GetUserByAccount(account string) (*User, error)
	GetUserByID(id uint64) (*User, error)
	SetEquip(id uint64, equip int) error
	// 好友（T3 用）
	AddFriend(a, b uint64) error
	FriendIDs(uid uint64) ([]uint64, error)
}

// ---- MemStore（测试/开发——重启丢）----

type MemStore struct {
	mu      sync.Mutex
	nextID  uint64
	users   map[string]*User // account → user
	byID    map[uint64]*User
	friends map[uint64]map[uint64]bool // uid → 好友集合
}

func NewMemStore() *MemStore {
	return &MemStore{
		users:   make(map[string]*User),
		byID:    make(map[uint64]*User),
		friends: make(map[uint64]map[uint64]bool),
	}
}

func (m *MemStore) CreateUser(u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.Account]; ok {
		return ErrAccountExists
	}
	m.nextID++
	u.ID = m.nextID
	cp := *u
	m.users[u.Account] = &cp
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

func (m *MemStore) GetUserByID(id uint64) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *MemStore) SetEquip(id uint64, equip int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return ErrNotFound
	}
	u.EquipID = equip
	return nil
}

func (m *MemStore) AddFriend(a, b uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.friends[a] == nil {
		m.friends[a] = make(map[uint64]bool)
	}
	if m.friends[b] == nil {
		m.friends[b] = make(map[uint64]bool)
	}
	m.friends[a][b] = true
	m.friends[b][a] = true
	return nil
}

func (m *MemStore) FriendIDs(uid uint64) ([]uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []uint64
	for f := range m.friends[uid] {
		out = append(out, f)
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
	res, err := m.db.Exec(
		"INSERT INTO users (account, password_hash, nickname) VALUES (?,?,?)",
		u.Account, u.PasswordHash, u.Nickname)
	if err != nil {
		if isDup(err) {
			return ErrAccountExists
		}
		return err
	}
	id, _ := res.LastInsertId()
	u.ID = uint64(id)
	return nil
}

func (m *MySQLStore) GetUserByAccount(account string) (*User, error) {
	return m.scanRow(m.db.QueryRow(
		"SELECT id, account, password_hash, nickname, equip_id FROM users WHERE account=?", account))
}

func (m *MySQLStore) GetUserByID(id uint64) (*User, error) {
	return m.scanRow(m.db.QueryRow(
		"SELECT id, account, password_hash, nickname, equip_id FROM users WHERE id=?", id))
}

func (m *MySQLStore) SetEquip(id uint64, equip int) error {
	_, err := m.db.Exec("UPDATE users SET equip_id=? WHERE id=?", equip, id)
	return err
}

func (m *MySQLStore) AddFriend(a, b uint64) error {
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	_, err := m.db.Exec(
		"INSERT IGNORE INTO friends (user_a, user_b) VALUES (?,?)", lo, hi)
	return err
}

func (m *MySQLStore) FriendIDs(uid uint64) ([]uint64, error) {
	rows, err := m.db.Query(
		"SELECT IF(user_a=?, user_b, user_a) FROM friends WHERE user_a=? OR user_b=?", uid, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint64
	for rows.Next() {
		var f uint64
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

func isDup(err error) bool {
	// MySQL 1062 = duplicate entry
	return err != nil && len(err.Error()) > 0 && err.Error()[:1] == "E" &&
		contains(err.Error(), "1062")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
