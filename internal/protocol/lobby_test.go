// lobby_test.go — 大厅协议：登录注册/好友/房间/背包编解码 roundtrip
package protocol

import (
	"testing"
)

func TestLogin_RoundTrip(t *testing.T) {
	body := EncodeLogin("alice_001", "pass123")
	acc, pw, ok := DecodeLogin(body)
	if !ok || acc != "alice_001" || pw != "pass123" {
		t.Fatalf("login roundtrip: %q %q ok=%v", acc, pw, ok)
	}
}

func TestReg_RoundTrip(t *testing.T) {
	body := EncodeReg("bob", "pw", "鲍勃")
	acc, pw, nick, ok := DecodeReg(body)
	if !ok || acc != "bob" || pw != "pw" || nick != "鲍勃" {
		t.Fatalf("reg roundtrip: %q %q %q ok=%v", acc, pw, nick, ok)
	}
}

func TestFriendInfo_RoundTrip(t *testing.T) {
	fi := FriendInfo{UID: 7, Nickname: "爱丽丝", Online: true}
	got, ok := DecodeFriendInfo(EncodeFriendInfo(fi.UID, fi.Nickname, fi.Online))
	if !ok || got.UID != 7 || got.Nickname != "爱丽丝" || !got.Online {
		t.Fatalf("friendinfo roundtrip: %+v ok=%v", got, ok)
	}
}

func TestFriendList_RoundTrip(t *testing.T) {
	list := []FriendInfo{
		{UID: 1, Nickname: "A", Online: true},
		{UID: 2, Nickname: "B", Online: false},
	}
	got, ok := DecodeFriendList(EncodeFriendList(list))
	if !ok || len(got) != 2 || got[0].UID != 1 || got[1].Nickname != "B" || got[1].Online {
		t.Fatalf("friendlist roundtrip: %+v ok=%v", got, ok)
	}
}

func TestRoomInfo_RoundTrip(t *testing.T) {
	r := RoomInfo{Name: "test_room", Creator: "房主", Note: "备注内容", Players: 3, Max: 8}
	got, ok := DecodeRoomInfo(EncodeRoomInfo(r))
	if !ok || got.Name != "test_room" || got.Creator != "房主" ||
		got.Note != "备注内容" || got.Players != 3 || got.Max != 8 {
		t.Fatalf("roominfo roundtrip: %+v ok=%v", got, ok)
	}
}

func TestRoomList_RoundTrip(t *testing.T) {
	rooms := []RoomInfo{
		{Name: "r1", Creator: "甲", Note: "", Players: 1, Max: 8},
		{Name: "r2", Creator: "乙", Note: "满房", Players: 8, Max: 8},
	}
	got, ok := DecodeRoomList(EncodeRoomList(rooms))
	if !ok || len(got) != 2 || got[1].Players != 8 {
		t.Fatalf("roomlist roundtrip: %+v ok=%v", got, ok)
	}
}

func TestShortData_Safe(t *testing.T) {
	if _, _, ok := DecodeLogin([]byte{0x01}); ok {
		t.Fatal("short data should fail")
	}
	if _, ok := DecodeRoomList([]byte{5, 0x01}); ok {
		t.Fatal("truncated roomlist should fail")
	}
}
