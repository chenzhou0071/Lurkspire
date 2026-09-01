// frame.go — 二进制帧编解码：12 字节头（magic/msgID/seq/bodyLen）+ body，64KB 上限，大端序
// 与 SignalDrift 同一帧协议家族（Magic 0x5344）
package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	Magic       uint16 = 0x5344 // "SD"
	HeaderSize         = 12
	MaxBodySize        = 64 * 1024
)

var (
	ErrBadMagic     = errors.New("protocol: bad magic")
	ErrBodyTooLarge = errors.New("protocol: body too large")
)

type Frame struct {
	MsgID uint16
	Seq   uint32
	Body  []byte
}

func Encode(f *Frame) []byte {
	b := make([]byte, HeaderSize+len(f.Body))
	binary.BigEndian.PutUint16(b[0:2], Magic)
	binary.BigEndian.PutUint16(b[2:4], f.MsgID)
	binary.BigEndian.PutUint32(b[4:8], f.Seq)
	binary.BigEndian.PutUint32(b[8:12], uint32(len(f.Body)))
	copy(b[HeaderSize:], f.Body)
	return b
}

type FrameReader struct {
	r      io.Reader
	header [HeaderSize]byte
}

func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: r}
}

// Next 阻塞读取一个完整帧；io.ReadFull 天然处理 TCP 拆包，
// 循环调用 Next 天然处理粘包。
func (fr *FrameReader) Next() (*Frame, error) {
	if _, err := io.ReadFull(fr.r, fr.header[:]); err != nil {
		return nil, err
	}
	if binary.BigEndian.Uint16(fr.header[0:2]) != Magic {
		return nil, ErrBadMagic
	}
	bodyLen := binary.BigEndian.Uint32(fr.header[8:12])
	if bodyLen > MaxBodySize {
		return nil, ErrBodyTooLarge
	}
	f := &Frame{
		MsgID: binary.BigEndian.Uint16(fr.header[2:4]),
		Seq:   binary.BigEndian.Uint32(fr.header[4:8]),
	}
	if bodyLen > 0 {
		f.Body = make([]byte, bodyLen)
		if _, err := io.ReadFull(fr.r, f.Body); err != nil {
			return nil, err
		}
	}
	return f, nil
}
