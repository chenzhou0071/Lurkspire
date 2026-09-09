// battle_test.go — 对局协议：roundtrip / golden bytes / 短数据防御
package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeState_RoundTrip(t *testing.T) {
	states := []PlayerState{
		{UID: 1, X: 100.5, Y: 2.25, Z: 30, Yaw: 1.5, HP: 100, Weapon: 0, Alt: 1, Block: 80, Anim: 3},
		{UID: 2, X: 50, Y: 0, Z: -10.5, Yaw: -2, HP: 55, Weapon: 1, Alt: 0, Block: 100, Anim: 1},
	}
	got := DecodeState(EncodeState(states))
	if len(got) != 2 {
		t.Fatalf("want 2 states, got %d", len(got))
	}
	if got[0] != states[0] || got[1] != states[1] {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, states)
	}
}

func TestEncodeInput_RoundTrip(t *testing.T) {
	in := &InputReport{MoveX: 1, MoveY: -1, Yaw: 0.5, Buttons: BtnFire | BtnSlide, AimX: 45, AimY: -30}
	got := DecodeInput(EncodeInput(in))
	if got != *in {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, *in)
	}
}

func TestEncodeHit_RoundTrip(t *testing.T) {
	h := &HitEvent{Shooter: 2, Target: 1, Damage: 50, Headshot: true}
	got := DecodeHit(EncodeHit(h))
	if got != *h {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, *h)
	}
}

func TestEncodeState_GoldenBytes(t *testing.T) {
	states := []PlayerState{
		{UID: 1, X: 1, Y: 2, Z: 3, Yaw: 4, HP: 100, Weapon: 0, Alt: 1, Block: 90, Anim: 0, Score: 7, Deaths: 3},
	}
	b := EncodeState(states)
	want := []byte{
		0x01,                   // count=1
		0x00, 0x00, 0x00, 0x01, // uid=1
		0x3F, 0x80, 0x00, 0x00, // x=1.0
		0x40, 0x00, 0x00, 0x00, // y=2.0
		0x40, 0x40, 0x00, 0x00, // z=3.0
		0x40, 0x80, 0x00, 0x00, // yaw=4.0
		0x64,                   // hp=100
		0x00,                   // weapon=0
		0x01,                   // alt=1
		0x42, 0xB4, 0x00, 0x00, // block=90.0
		0x00,       // anim=0
		0x00, 0x07, // score=7
		0x00, 0x03, // deaths=3
		0x00, 0x00, 0x00, 0x00, // 预留 4B
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("golden mismatch:\n got %v\nwant %v", b, want)
	}
}

func TestDecodeInput_ShortData_ReturnsZero(t *testing.T) {
	got := DecodeInput([]byte{0x01}) // 只有 1 字节
	if got != (InputReport{}) {
		t.Fatalf("want zero InputReport, got %+v", got)
	}
}

func TestEncodeSettle_RoundTrip(t *testing.T) {
	entries := []SettleEntry{{UID: 1, Score: 20}, {UID: 2, Score: 12}, {UID: 3, Score: 3}}
	got := DecodeSettle(EncodeSettle(entries))
	if len(got) != 3 || got[0] != entries[0] || got[2] != entries[2] {
		t.Fatalf("settle roundtrip mismatch:\n got %+v\nwant %+v", got, entries)
	}
}

func TestPlayerInfo_RoundTrip(t *testing.T) {
	uid, nick, ok := DecodePlayerInfo(EncodePlayerInfo(42, "玩家四十二"))
	if !ok || uid != 42 || nick != "玩家四十二" {
		t.Fatalf("playerinfo roundtrip: uid=%d nick=%s ok=%v", uid, nick, ok)
	}
	if _, _, ok := DecodePlayerInfo([]byte{0x01}); ok {
		t.Fatal("short data should fail")
	}
}
