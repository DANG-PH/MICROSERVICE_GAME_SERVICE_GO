package protocol

import (
	"math"
	"testing"
)

// Round-trip test: encode rồi decode lại phải bằng giá trị ban đầu.
// Đây là test pattern quan trọng nhất cho codec.
func TestCodecRoundTrip(t *testing.T) {
	enc := NewEncoder(0x42)
	enc.WriteUint8(255)
	enc.WriteInt8(-1)
	enc.WriteUint16(65535)
	enc.WriteInt32(-2147483648)
	enc.WriteFloat32(3.14)
	enc.WriteBool(true)
	enc.WriteBool(false)
	if err := enc.WriteString("hello, 世界"); err != nil {
		t.Fatal(err)
	}

	data := enc.Bytes()
	if data[0] != 0x42 {
		t.Errorf("expected msgType 0x42, got 0x%02x", data[0])
	}

	dec := NewDecoder(data[1:]) // bỏ msgType byte

	if v, _ := dec.ReadUint8(); v != 255 {
		t.Errorf("uint8: got %d", v)
	}
	if v, _ := dec.ReadInt8(); v != -1 {
		t.Errorf("int8: got %d", v)
	}
	if v, _ := dec.ReadUint16(); v != 65535 {
		t.Errorf("uint16: got %d", v)
	}
	if v, _ := dec.ReadInt32(); v != -2147483648 {
		t.Errorf("int32: got %d", v)
	}
	if v, _ := dec.ReadFloat32(); math.Abs(float64(v-3.14)) > 0.0001 {
		t.Errorf("float32: got %f", v)
	}
	if v, _ := dec.ReadBool(); v != true {
		t.Errorf("bool true: got %v", v)
	}
	if v, _ := dec.ReadBool(); v != false {
		t.Errorf("bool false: got %v", v)
	}
	if v, _ := dec.ReadString(); v != "hello, 世界" {
		t.Errorf("string: got %q", v)
	}

	if dec.Remaining() != 0 {
		t.Errorf("expected 0 remaining, got %d", dec.Remaining())
	}
}

// Test buffer too short — phải return error, không crash.
func TestDecoderShortBuffer(t *testing.T) {
	dec := NewDecoder([]byte{0x01})
	if _, err := dec.ReadUint16(); err == nil {
		t.Error("expected error reading uint16 from 1-byte buffer")
	}
}
