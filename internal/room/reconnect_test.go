// reconnect_test.go — 断线宽限：离线保留/超时淘汰/重连恢复/空房通知
package room

import (
	"sync/atomic"
	"testing"
	"time"
)

// testReconnectConfig 宽限 6 tick（TickHz=60 → ≈100ms——留足调度余量）
func testReconnectConfig() Config {
	return Config{MaxPlayers: 8, TickHz: 60, ReconnectGraceTicks: 6}
}

func sleepTicks(n int) { time.Sleep(time.Duration(n) * 25 * time.Millisecond) }

func TestRoom_Offline_KeepsState(t *testing.T) {
	r := NewRoom("rc1", testReconnectConfig())
	defer r.Stop()
	if err := r.AddPlayer(1); err != nil {
		t.Fatal(err)
	}
	waitSnapshot(r, 1)
	r.SetOffline(1)
	sleepTicks(2) // 未超宽限（3）——玩家与状态保留
	if got := len(r.StateSnapshot()); got != 1 {
		t.Fatalf("offline player should stay in grace: %d", got)
	}
}

func TestRoom_Offline_EliminatedAfterGrace(t *testing.T) {
	r := NewRoom("rc2", testReconnectConfig())
	defer r.Stop()
	r.AddPlayer(1)
	waitSnapshot(r, 1)
	r.SetOffline(1)
	sleepTicks(8) // 超过宽限（6）——淘汰
	if got := len(r.StateSnapshot()); got != 0 {
		t.Fatalf("offline player should be eliminated: %d", got)
	}
}

func TestRoom_Reconnect_Restores(t *testing.T) {
	r := NewRoom("rc3", testReconnectConfig())
	defer r.Stop()
	r.AddPlayer(1)
	waitSnapshot(r, 1)
	r.SetOffline(1)
	sleepTicks(2)                          // 宽限内
	if err := r.AddPlayer(1); err != nil { // 重连恢复
		t.Fatal(err)
	}
	sleepTicks(8) // 若离线计时未清——此处会被淘汰
	if got := len(r.StateSnapshot()); got != 1 {
		t.Fatalf("reconnected player should stay online: %d", got)
	}
}

func TestRoom_Empty_OnEmptyOnce(t *testing.T) {
	r := NewRoom("rc4", testReconnectConfig())
	defer r.Stop()
	var count int32
	r.SetOnEmpty(func(name string) {
		if name != "rc4" {
			t.Errorf("empty callback name wrong: %s", name)
		}
		atomic.AddInt32(&count, 1)
	})
	r.AddPlayer(1)
	waitSnapshot(r, 1)
	r.RemovePlayer(1) // 主动退出 → 房间空
	sleepTicks(3)
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("onEmpty should fire once: %d", got)
	}
	sleepTicks(3)
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("onEmpty should not repeat: %d", got)
	}
}

func TestRoom_FreshRoom_NoEmptyCallback(t *testing.T) {
	r := NewRoom("rc5", testReconnectConfig())
	defer r.Stop()
	var count int32
	r.SetOnEmpty(func(string) { atomic.AddInt32(&count, 1) })
	sleepTicks(5) // 从未有玩家——不应触发空房通知
	if got := atomic.LoadInt32(&count); got != 0 {
		t.Fatalf("fresh room should not fire onEmpty: %d", got)
	}
}
