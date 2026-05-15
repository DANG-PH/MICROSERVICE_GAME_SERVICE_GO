package protocol

// // PlayerMove — client gửi vị trí mới.
// //
// // Giữ TẤT CẢ field giống NestJS để parity 100%, không cần sửa client logic.
// // Sau này tối ưu sẽ tách cosmetic ra message riêng.
// //
// // Format chi tiết:
// //
// //	[0x01]                msgType (1 byte)
// //	[string] mapID         2 + N bytes (vd: "traidat", "namek")
// //	[float32] x            4 bytes
// //	[float32] y            4 bytes
// //	[uint8]  trangthai     1 byte (encode bằng enum, xem enums/trangthai.go)
// //	[int8]   dir           1 byte (-1 hoặc 1)
// //	[string] dau           2 + N bytes
// //	[string] than          2 + N bytes
// //	[string] chan          2 + N bytes
// //	[float32] timeChoHienBay  4 bytes
// //	[float32] lechDauX     4 bytes
// //	[float32] lechDauY     4 bytes
// //	[float32] lechThanX    4 bytes
// //	[float32] lechThanY    4 bytes
// //	[float32] lechChanX    4 bytes
// //	[float32] lechChanY    4 bytes
// //	[bool]    dangMangVanBay 1 byte
// //	[string]  tenVanBay    2 + N bytes
// //	[float32] rong         4 bytes
// //	[float32] cao          4 bytes
// //	[string]  avatar       2 + N bytes
// //
// // Total: ~80-150 bytes tùy length string. So với JSON ~400-600 bytes → giảm 3-5x.
// type PlayerMove struct {
// 	MapID          string
// 	X              float32
// 	Y              float32
// 	Trangthai      uint8
// 	Dir            int8
// 	Dau            string
// 	Than           string
// 	Chan           string
// 	TimeChoHienBay float32
// 	LechDauX       float32
// 	LechDauY       float32
// 	LechThanX      float32
// 	LechThanY      float32
// 	LechChanX      float32
// 	LechChanY      float32
// 	DangMangVanBay bool
// 	TenVanBay      string
// 	Rong           float32
// 	Cao            float32
// 	Avatar         string
// }

// func (m *PlayerMove) Decode(data []byte) error {
// 	d := NewDecoder(data)
// 	var err error

// 	if m.MapID, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	if m.X, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.Y, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.Trangthai, err = d.ReadUint8(); err != nil {
// 		return err
// 	}
// 	if m.Dir, err = d.ReadInt8(); err != nil {
// 		return err
// 	}
// 	if m.Dau, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	if m.Than, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	if m.Chan, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	if m.TimeChoHienBay, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.LechDauX, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.LechDauY, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.LechThanX, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.LechThanY, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.LechChanX, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.LechChanY, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.DangMangVanBay, err = d.ReadBool(); err != nil {
// 		return err
// 	}
// 	if m.TenVanBay, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	if m.Rong, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.Cao, err = d.ReadFloat32(); err != nil {
// 		return err
// 	}
// 	if m.Avatar, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	return nil
// }

// // PlayerSync — server broadcast move của player tới các player khác trong map.
// // Giống PlayerMove nhưng thêm userId ở đầu.
// type PlayerSync struct {
// 	UserID         int32
// 	X              float32
// 	Y              float32
// 	Trangthai      uint8
// 	Dir            int8
// 	Dau            string
// 	Than           string
// 	Chan           string
// 	TimeChoHienBay float32
// 	LechDauX       float32
// 	LechDauY       float32
// 	LechThanX      float32
// 	LechThanY      float32
// 	LechChanX      float32
// 	LechChanY      float32
// 	DangMangVanBay bool
// 	TenVanBay      string
// 	Rong           float32
// 	Cao            float32
// 	Avatar         string
// 	ServerTime     int64
// }

// func (m *PlayerSync) Encode() []byte {
// 	enc := NewEncoder(MsgPlayerSync)
// 	enc.WriteInt32(m.UserID)
// 	enc.WriteFloat32(m.X)
// 	enc.WriteFloat32(m.Y)
// 	enc.WriteUint8(m.Trangthai)
// 	enc.WriteInt8(m.Dir)
// 	_ = enc.WriteString(m.Dau)
// 	_ = enc.WriteString(m.Than)
// 	_ = enc.WriteString(m.Chan)
// 	enc.WriteFloat32(m.TimeChoHienBay)
// 	enc.WriteFloat32(m.LechDauX)
// 	enc.WriteFloat32(m.LechDauY)
// 	enc.WriteFloat32(m.LechThanX)
// 	enc.WriteFloat32(m.LechThanY)
// 	enc.WriteFloat32(m.LechChanX)
// 	enc.WriteFloat32(m.LechChanY)
// 	enc.WriteBool(m.DangMangVanBay)
// 	_ = enc.WriteString(m.TenVanBay)
// 	enc.WriteFloat32(m.Rong)
// 	enc.WriteFloat32(m.Cao)
// 	_ = enc.WriteString(m.Avatar)
// 	return enc.Bytes()
// }

// // PlayerSyncBatch — server gửi toàn bộ dirty players trong 1 packet.
// //
// // Format:
// //
// //	[0x83]         msgType (1 byte)
// //	[uint16]       count   (2 bytes) — số lượng player
// //	[PlayerSync]×N          — từng player, layout giống PlayerSync
// //
// // Tại sao uint16 cho count? Lý thuyết 1 map có tối đa 65535 player —
// // uint8 chỉ được 255, quá thấp. uint16 = 2 bytes, chấp nhận được.
// type PlayerSyncBatch struct {
// 	Players []PlayerSync
// }

// func (b *PlayerSyncBatch) Encode() []byte {
// 	enc := NewEncoder(MsgPlayerSyncBatch)
// 	enc.WriteUint16(uint16(len(b.Players)))
// 	for i := range b.Players {
// 		p := &b.Players[i]
// 		enc.WriteInt32(p.UserID)
// 		enc.WriteFloat32(p.X)
// 		enc.WriteFloat32(p.Y)
// 		enc.WriteUint8(p.Trangthai)
// 		enc.WriteInt8(p.Dir)
// 		_ = enc.WriteString(p.Dau)
// 		_ = enc.WriteString(p.Than)
// 		_ = enc.WriteString(p.Chan)
// 		enc.WriteFloat32(p.TimeChoHienBay)
// 		enc.WriteFloat32(p.LechDauX)
// 		enc.WriteFloat32(p.LechDauY)
// 		enc.WriteFloat32(p.LechThanX)
// 		enc.WriteFloat32(p.LechThanY)
// 		enc.WriteFloat32(p.LechChanX)
// 		enc.WriteFloat32(p.LechChanY)
// 		enc.WriteBool(p.DangMangVanBay)
// 		_ = enc.WriteString(p.TenVanBay)
// 		enc.WriteFloat32(p.Rong)
// 		enc.WriteFloat32(p.Cao)
// 		_ = enc.WriteString(p.Avatar)
// 		enc.WriteInt64(p.ServerTime)
// 	}
// 	return enc.Bytes()
// }
