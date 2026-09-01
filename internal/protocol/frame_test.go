// frame_test.go — 帧编解码：golden bytes / 粘包拆包 / 错误帧
package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func binaryWriteUint16(b []byte, off int, v uint16) {
	binary.BigEndian.PutUint16(b[off:], v)
}

func binaryWriteUint32(b []byte, off int, v uint32) {
	binary.BigEndian.PutUint32(b[off:], v)
}

func TestEncode_GoldenBytes(t *testing.T) {
	f := &Frame{MsgID: 310, Seq: 7, Body: []byte{0x01, 0x02}}
	got := Encode(f)
	want := []byte{
		0x53, 0x44, // magic
		0x01, 0x36, // msgID=310
		0x00, 0x00, 0x00, 0x07, // seq=7
		0x00, 0x00, 0x00, 0x02, // bodyLen=2
		0x01, 0x02, // body
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestFrameReader_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Encode(&Frame{MsgID: 300, Seq: 1, Body: []byte("hello")}))
	buf.Write(Encode(&Frame{MsgID: 301, Seq: 2, Body: nil})) // 粘包：两个帧连写

	r := NewFrameReader(&buf)
	f1, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if f1.MsgID != 300 || f1.Seq != 1 || string(f1.Body) != "hello" {
		t.Fatalf("f1 wrong: %+v", f1)
	}
	f2, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if f2.MsgID != 301 || f2.Seq != 2 || len(f2.Body) != 0 {
		t.Fatalf("f2 wrong: %+v", f2)
	}
}

func TestFrameReader_BadMagic(t *testing.T) {
	r := NewFrameReader(bytes.NewReader([]byte{0x00, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
	if _, err := r.Next(); err != ErrBadMagic {
		t.Fatalf("want ErrBadMagic, got %v", err)
	}
}

func TestFrameReader_BodyTooLarge(t *testing.T) {
	b := make([]byte, HeaderSize)
	binaryWriteUint16(b, 0, Magic)
	binaryWriteUint32(b, 8, MaxBodySize+1)
	r := NewFrameReader(bytes.NewReader(b))
	if _, err := r.Next(); err != ErrBodyTooLarge {
		t.Fatalf("want ErrBodyTooLarge, got %v", err)
	}
}
