package protocol

import (
	"encoding/binary"
	"errors"
	"math"
)

// Tại sao tự viết encoder/decoder mà không dùng encoding/binary.Read/Write?
// 1. Read/Write dùng reflection → chậm hơn 5-10x.
// 2. Mỗi struct có schema riêng (field nào trước, field nào sau) — code-gen từ struct
//    không control được order chính xác như tự tay viết.
// 3. Học byte-level encoding là mục đích của bài này.
//
// Byte order: BigEndian (network byte order). Java DataInputStream mặc định BigEndian.
// Đừng dùng LittleEndian — gây nhầm lẫn khi debug bằng Wireshark.

var (
	ErrBufferTooShort = errors.New("buffer too short")
	ErrStringTooLong  = errors.New("string too long (max 65535 bytes)")
)

// ---------- Encoder ----------

// Encoder ghi data vào buffer tăng dần.
// Pattern: tạo encoder với msgType byte đầu, append các field, gọi Bytes() lấy kết quả.
type Encoder struct {
	buf []byte
}

// NewEncoder tạo encoder mới với 1 byte msgType ở đầu.
// Pre-allocate 64 bytes vì hầu hết packet < 64 bytes (player-move ~80 bytes là to nhất).
func NewEncoder(msgType uint8) *Encoder {
	buf := make([]byte, 0, 64)
	buf = append(buf, msgType)
	return &Encoder{buf: buf}
}

func (e *Encoder) WriteUint8(v uint8) {
	e.buf = append(e.buf, v)
}

func (e *Encoder) WriteInt8(v int8) {
	e.buf = append(e.buf, byte(v))
}

func (e *Encoder) WriteUint16(v uint16) {
	e.buf = binary.BigEndian.AppendUint16(e.buf, v)
}

func (e *Encoder) WriteUint32(v uint32) {
	e.buf = binary.BigEndian.AppendUint32(e.buf, v)
}

func (e *Encoder) WriteInt32(v int32) {
	e.buf = binary.BigEndian.AppendUint32(e.buf, uint32(v))
}

func (e *Encoder) WriteFloat32(v float32) {
	// Float32 → uint32 bits → 4 bytes BigEndian.
	// math.Float32bits chỉ đơn giản là reinterpret bits, không phải convert giá trị.
	e.buf = binary.BigEndian.AppendUint32(e.buf, math.Float32bits(v))
}

// WriteBool — 1 byte, 0 hoặc 1.
// Có thể tối ưu sau bằng bitfield (8 bool trong 1 byte) nhưng chưa cần.
func (e *Encoder) WriteBool(v bool) {
	if v {
		e.buf = append(e.buf, 1)
	} else {
		e.buf = append(e.buf, 0)
	}
}

// WriteString — uint16 length prefix + UTF-8 bytes.
// Max length 65535 bytes. Đủ cho mọi string trong game (chat max 200 chars).
func (e *Encoder) WriteString(s string) error {
	if len(s) > 65535 {
		return ErrStringTooLong
	}
	e.WriteUint16(uint16(len(s)))
	e.buf = append(e.buf, s...)
	return nil
}

// Bytes trả về raw bytes để gửi qua WebSocket.
// KHÔNG copy — caller không được modify slice trả về.
func (e *Encoder) Bytes() []byte {
	return e.buf
}

// ---------- Decoder ----------

// Decoder đọc data từ buffer theo thứ tự.
// pos là con trỏ hiện tại. Mỗi Read* advance pos.
// Khi sai thứ tự read so với encoder → data sai → bug.
type Decoder struct {
	buf []byte
	pos int
}

// NewDecoder tạo decoder. Caller chịu trách nhiệm bỏ msgType byte trước (data[1:]).
func NewDecoder(buf []byte) *Decoder {
	return &Decoder{buf: buf, pos: 0}
}

// Remaining trả về số byte chưa đọc — dùng để validate.
func (d *Decoder) Remaining() int {
	return len(d.buf) - d.pos
}

func (d *Decoder) ReadUint8() (uint8, error) {
	if d.Remaining() < 1 {
		return 0, ErrBufferTooShort
	}
	v := d.buf[d.pos]
	d.pos++
	return v, nil
}

func (d *Decoder) ReadInt8() (int8, error) {
	v, err := d.ReadUint8()
	return int8(v), err
}

func (d *Decoder) ReadUint16() (uint16, error) {
	if d.Remaining() < 2 {
		return 0, ErrBufferTooShort
	}
	v := binary.BigEndian.Uint16(d.buf[d.pos:])
	d.pos += 2
	return v, nil
}

func (d *Decoder) ReadUint32() (uint32, error) {
	if d.Remaining() < 4 {
		return 0, ErrBufferTooShort
	}
	v := binary.BigEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return v, nil
}

func (d *Decoder) ReadInt32() (int32, error) {
	v, err := d.ReadUint32()
	return int32(v), err
}

func (d *Decoder) ReadFloat32() (float32, error) {
	v, err := d.ReadUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

func (d *Decoder) ReadBool() (bool, error) {
	v, err := d.ReadUint8()
	return v != 0, err
}

// ReadString đọc uint16 length + N bytes UTF-8.
func (d *Decoder) ReadString() (string, error) {
	length, err := d.ReadUint16()
	if err != nil {
		return "", err
	}
	if d.Remaining() < int(length) {
		return "", ErrBufferTooShort
	}
	s := string(d.buf[d.pos : d.pos+int(length)])
	d.pos += int(length)
	return s, nil
}
