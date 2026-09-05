// room_test.go — 房间核心：容量/输入移动/移除/管理器
package room

import (
	"testing"
	"time"

	"lurkspire/server/internal/protocol"
)

func testConfig() Config {
	return Config{MaxPlayers: 8, TickHz: 30, Speed: 16, SpawnRange: 10}
}

func waitSnapshot(r *Room, n int) []protocol.PlayerState {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := r.StateSnapshot()
		if len(s) == n {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	return r.StateSnapshot()
}

func TestRoom_AddPlayer_Capacity(t *testing.T) {
	r := NewRoom("r1", testConfig())
	defer r.Stop()
	for uid := uint32(1); uid <= 8; uid++ {
		if err := r.AddPlayer(uid); err != nil {
			t.Fatalf("add %d: %v", uid, err)
		}
	}
	if err := r.AddPlayer(9); err != ErrRoomFull {
		t.Fatalf("9th player: want ErrRoomFull, got %v", err)
	}
	if len(waitSnapshot(r, 8)) != 8 {
		t.Fatal("want 8 players in snapshot")
	}
}

func TestRoom_RemovePlayer_SnapshotExcludes(t *testing.T) {
	r := NewRoom("r3", testConfig())
	defer r.Stop()
	r.AddPlayer(1)
	r.AddPlayer(2)
	waitSnapshot(r, 2)

	r.RemovePlayer(1)
	s := waitSnapshot(r, 1)
	if len(s) != 1 || s[0].UID != 2 {
		t.Fatalf("after remove: want only uid2, got %+v", s)
	}
}

func TestRoom_Manager_CreateGetDestroy(t *testing.T) {
	m := NewManager(testConfig())
	r, err := m.CreateRoom("arena-1")
	if err != nil {
		t.Fatal(err)
	}
	// 同名幂等
	r2, _ := m.CreateRoom("arena-1")
	if r2 != r {
		t.Fatal("same id should return same room")
	}
	if got, ok := m.GetRoom("arena-1"); !ok || got != r {
		t.Fatal("GetRoom miss")
	}
	m.DestroyRoom("arena-1")
	if _, ok := m.GetRoom("arena-1"); ok {
		t.Fatal("room should be destroyed")
	}
}
