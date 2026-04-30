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
// BIGENDIAN vs LITTLEENDIAN là gì?
// Khi ghi số nhiều byte (ví dụ số 256 = 0x0100) vào bộ nhớ, có 2 cách sắp xếp:
//
//   BigEndian (byte cao trước):    [0x01][0x00]  ← mạng, Java, Wireshark dùng cái này
//   LittleEndian (byte thấp trước): [0x00][0x01]  ← CPU x86, Windows dùng cái này
//
// Ví dụ số 1000 (0x000003E8):
//   BigEndian:    [0x00][0x00][0x03][0xE8]  ← đọc trái sang phải như số thường
//   LittleEndian: [0xE8][0x03][0x00][0x00]  ← đọc ngược
//
// Tại sao phải quan tâm?
// Client (LibGDX/Java) và server (Go) phải dùng CÙNG thứ tự byte,
// nếu không số 1000 gửi đi sẽ được đọc thành 3.992.977.408 ở đầu kia.
// Chọn BigEndian vì đó là "network byte order" — convention của mọi protocol mạng.

var (
	ErrBufferTooShort = errors.New("buffer too short")
	ErrStringTooLong  = errors.New("string too long (max 65535 bytes)")
)

// ---------- Encoder ----------

// Encoder là cái "bút" dùng để viết các field vào 1 mảng bytes theo thứ tự.
// Mỗi lần gọi Write* thì bytes được nối thêm vào đuôi buffer.
// Cuối cùng gọi Bytes() để lấy toàn bộ packet ra gửi qua WebSocket.
type Encoder struct {
	buf []byte
}

// NewEncoder tạo 1 packet mới, byte đầu tiên luôn là msgType.
// Ví dụ: NewEncoder(0x01) → packet bắt đầu bằng [0x01, ...]
// msgType ở đầu để server/client đọc byte đầu là biết ngay đây là loại message gì,
// không cần đọc hết packet mới biết.
//
// Pre-allocate là gì?
// make([]byte, 0, 64) tạo slice rỗng nhưng đã xin sẵn 64 bytes bộ nhớ.
// Khi append, Go không phải xin thêm bộ nhớ cho đến khi vượt 64 bytes.
// Nếu không pre-allocate: append 1 byte → Go xin 1 byte, append thêm → xin thêm...
// mỗi lần xin bộ nhớ là copy toàn bộ buffer sang chỗ mới → chậm.
// Hầu hết packet < 64 bytes nên pre-allocate 64 là vừa đủ cho 1 lần xin duy nhất.
func NewEncoder(msgType uint8) *Encoder {
	buf := make([]byte, 0, 64)
	buf = append(buf, msgType)
	return &Encoder{buf: buf}
}

// WriteUint8 ghi 1 byte vào packet.
// Dùng cho các giá trị nhỏ 0-255: msgType, reason code, flag...
// 1 byte = không cần quan tâm BigEndian/LittleEndian vì chỉ có 1 byte,
// không có vấn đề "byte nào trước byte nào sau".
func (e *Encoder) WriteUint8(v uint8) {
	e.buf = append(e.buf, v)
}

// WriteInt8 ghi 1 byte có dấu vào packet (-128 đến 127).
// Dùng khi giá trị có thể âm nhưng nhỏ, ví dụ delta di chuyển nhỏ.
// Về bytes thì giống WriteUint8, chỉ khác cách Go interpret giá trị.
//
// int8 vs uint8:
//
//	uint8: 0000 0000 → 1111 1111 = 0 đến 255
//	int8:  1000 0000 → 0111 1111 = -128 đến 127
//	Cùng 8 bit, chỉ khác cách đọc — bit đầu là dấu hay giá trị.
func (e *Encoder) WriteInt8(v int8) {
	e.buf = append(e.buf, byte(v))
}

// WriteUint16 ghi 2 bytes vào packet (0 đến 65535).
// Dùng cho protocol version, string length...
//
// Ví dụ số 300 (0x012C) ghi BigEndian:
//
//	byte 1: 0x01 (phần cao)
//	byte 2: 0x2C (phần thấp)
//
// Bên đọc ghép lại: 0x01 * 256 + 0x2C = 300 ✓
func (e *Encoder) WriteUint16(v uint16) {
	e.buf = binary.BigEndian.AppendUint16(e.buf, v)
}

// WriteUint32 ghi 4 bytes vào packet (0 đến ~4 tỷ).
// Dùng cho userID, timestamp...
// 4 bytes vì userID trong database thường là INT (32-bit).
func (e *Encoder) WriteUint32(v uint32) {
	e.buf = binary.BigEndian.AppendUint32(e.buf, v)
}

// WriteInt32 ghi 4 bytes có dấu vào packet (-2 tỷ đến 2 tỷ).
// Dùng cho userID kiểu int32 để khớp với NestJS/Java dùng int 32-bit.
//
// Tại sao ép uint32(v) trước khi ghi?
// binary.BigEndian chỉ có AppendUint32, không có AppendInt32.
// int32 và uint32 có cùng số bit, chỉ khác cách interpret —
// ép kiểu không thay đổi bits, chỉ thay đổi cách Go nhìn vào nó.
// Bên đọc dùng ReadInt32 sẽ ép ngược lại → ra đúng giá trị ban đầu.
func (e *Encoder) WriteInt32(v int32) {
	e.buf = binary.BigEndian.AppendUint32(e.buf, uint32(v))
}

func (e *Encoder) WriteInt64(v int64) {
	e.buf = binary.BigEndian.AppendUint64(e.buf, uint64(v))
}

// WriteFloat32 ghi tọa độ x, y của player vào packet (4 bytes).
//
// Float32 là gì?
// Số thực dấu phẩy động 32-bit theo chuẩn IEEE 754.
// Ví dụ: 10.5 được lưu thành 4 bytes: [0x41][0x28][0x00][0x00]
// Không phải số nguyên nên không thể dùng AppendUint32 trực tiếp.
//
// math.Float32bits làm gì?
// Lấy 4 bytes bộ nhớ của float32 và đọc nó như uint32 — không tính toán gì,
// chỉ "nhìn cùng 4 bytes theo cách khác". Sau đó AppendUint32 ghi 4 bytes đó ra.
// Bên đọc dùng math.Float32frombits để đảo ngược lại.
func (e *Encoder) WriteFloat32(v float32) {
	e.buf = binary.BigEndian.AppendUint32(e.buf, math.Float32bits(v))
}

// WriteBool ghi 1 byte: 1 nếu true, 0 nếu false.
// Dùng cho các flag: isRunning, isAttacking...
//
// Tại sao tốn cả 1 byte cho 1 bit thông tin?
// Đơn giản nhất để implement và đọc. Nếu có 8 flag trở lên thì
// gom vào 1 byte dùng bitfield (mỗi bit là 1 flag) tiết kiệm hơn,
// nhưng code phức tạp hơn. Giai đoạn này chưa cần.
func (e *Encoder) WriteBool(v bool) {
	if v {
		e.buf = append(e.buf, 1)
	} else {
		e.buf = append(e.buf, 0)
	}
}

// WriteString ghi chuỗi UTF-8 vào packet theo format: [2 bytes độ dài][N bytes nội dung].
//
// Tại sao cần ghi độ dài trước — gọi là "length-prefixed string"?
// Bên đọc nhận được stream bytes liên tục, không có dấu phân cách.
// Nếu không có độ dài, không biết string kết thúc ở đâu.
// Cách khác là dùng null-terminated (kết thúc bằng byte 0x00) như C,
// nhưng cách đó không cho phép string chứa ký tự null và khó validate hơn.
// Length-prefix rõ ràng và an toàn hơn.
//
// UTF-8 là gì?
// Cách mã hóa ký tự thành bytes. Ký tự ASCII (a-z, 0-9) = 1 byte,
// ký tự tiếng Việt = 2-3 bytes. Vì vậy len(s) là số bytes, không phải số ký tự.
// "abc" = 3 bytes, "xin chào" = 11 bytes dù nhìn có 8 ký tự.
func (e *Encoder) WriteString(s string) error {
	if len(s) > 65535 {
		return ErrStringTooLong
	}
	e.WriteUint16(uint16(len(s)))
	e.buf = append(e.buf, s...)
	return nil
}

// Bytes trả về toàn bộ packet dưới dạng []byte để gửi qua WebSocket.
//
// Tại sao không copy?
// Copy tốn thêm bộ nhớ và CPU. Vì packet được gửi ngay sau khi Bytes() được gọi
// và Encoder không được dùng lại, không copy là an toàn và nhanh hơn.
// Nếu sau này Encoder được reuse thì phải đổi thành copy.
func (e *Encoder) Bytes() []byte {
	return e.buf
}

// ---------- Decoder ----------

// Decoder là cái "bút chì đọc" — đọc từng field ra khỏi mảng bytes theo thứ tự.
// pos là con trỏ vị trí hiện tại trong buffer, mỗi lần Read* thì pos tiến lên.
//
// Tại sao phải đọc đúng thứ tự?
// Buffer chỉ là chuỗi bytes liền nhau, không có nhãn "đây là x", "đây là y".
// Nếu Encoder ghi theo thứ tự [x][y][mapID] thì Decoder phải đọc đúng thứ tự đó.
// Đọc sai thứ tự → bytes của y bị interpret thành x → số sai, không có lỗi nào báo.
type Decoder struct {
	buf []byte
	pos int
}

// NewDecoder tạo decoder từ phần payload (không bao gồm byte msgType đầu tiên).
// Tại sao bỏ byte đầu? Vì caller đã đọc data[0] để biết msgType rồi,
// truyền data[1:] vào đây để Decoder chỉ thấy phần payload.
// data[1:] trong Go là slice từ index 1 đến hết — không copy, chỉ thay đổi điểm bắt đầu.
func NewDecoder(buf []byte) *Decoder {
	return &Decoder{buf: buf, pos: 0}
}

// Remaining trả về số bytes chưa đọc.
// Dùng để validate trước mỗi lần đọc — nếu còn ít hơn số bytes cần đọc
// thì packet bị cắt ngắn (client bug hoặc ai đó gửi packet giả).
// Không validate → đọc ngoài slice → panic → server crash.
func (d *Decoder) Remaining() int {
	return len(d.buf) - d.pos
}

// ReadUint8 đọc 1 byte tại vị trí pos, tiến pos lên 1.
// Trả lỗi nếu hết bytes — caller xử lý lỗi thay vì panic.
func (d *Decoder) ReadUint8() (uint8, error) {
	if d.Remaining() < 1 {
		return 0, ErrBufferTooShort
	}
	v := d.buf[d.pos]
	d.pos++
	return v, nil
}

// ReadInt8 đọc 1 byte rồi interpret là số có dấu (-128 đến 127).
// int8(v) không thay đổi bits — chỉ thay đổi cách Go đọc giá trị.
// Ví dụ byte 0xFF: uint8 = 255, int8 = -1. Cùng 1 byte.
func (d *Decoder) ReadInt8() (int8, error) {
	v, err := d.ReadUint8()
	return int8(v), err
}

// ReadUint16 đọc 2 bytes BigEndian tại pos, tiến pos lên 2.
// binary.BigEndian.Uint16 ghép 2 bytes lại: byte[0]*256 + byte[1].
func (d *Decoder) ReadUint16() (uint16, error) {
	if d.Remaining() < 2 {
		return 0, ErrBufferTooShort
	}
	v := binary.BigEndian.Uint16(d.buf[d.pos:])
	d.pos += 2
	return v, nil
}

// ReadUint32 đọc 4 bytes BigEndian tại pos, tiến pos lên 4.
func (d *Decoder) ReadUint32() (uint32, error) {
	if d.Remaining() < 4 {
		return 0, ErrBufferTooShort
	}
	v := binary.BigEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return v, nil
}

// ReadInt32 đọc 4 bytes rồi interpret là số có dấu.
// int32(v) không thay đổi bits — chỉ đổi cách Go đọc.
// Phải dùng cùng cặp với WriteInt32, không được mix với WriteUint32.
func (d *Decoder) ReadInt32() (int32, error) {
	v, err := d.ReadUint32()
	return int32(v), err
}

func (d *Decoder) ReadInt64() (int64, error) {
	if d.Remaining() < 8 {
		return 0, ErrBufferTooShort
	}
	v := binary.BigEndian.Uint64(d.buf[d.pos:])
	d.pos += 8
	return int64(v), nil
}

// ReadFloat32 đọc 4 bytes rồi interpret là số thực float32.
// Ngược lại với WriteFloat32:
//
//	Ghi: float32 → Float32bits → uint32 → 4 bytes
//	Đọc: 4 bytes → uint32 → Float32frombits → float32
//
// math.Float32frombits chỉ reinterpret 4 bytes thành float, không tính toán gì.
func (d *Decoder) ReadFloat32() (float32, error) {
	v, err := d.ReadUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

// ReadBool đọc 1 byte, trả true nếu khác 0.
// Dùng != 0 thay vì == 1 để chấp nhận mọi giá trị non-zero là true —
// defensive programming phòng trường hợp client gửi 2, 255... thay vì đúng 1.
func (d *Decoder) ReadBool() (bool, error) {
	v, err := d.ReadUint8()
	return v != 0, err
}

// ReadString đọc string length-prefixed: [2 bytes độ dài][N bytes nội dung].
// Bước 1: đọc 2 bytes để biết string dài bao nhiêu bytes.
// Bước 2: kiểm tra còn đủ bytes không (tránh panic).
// Bước 3: cắt đúng N bytes ra, đổi thành string Go (UTF-8).
// string(slice) trong Go copy bytes vào string mới — cần thiết vì
// string Go là immutable, không thể trỏ thẳng vào buffer đang dùng.
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
