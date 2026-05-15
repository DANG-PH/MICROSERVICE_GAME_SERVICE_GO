package protocol

// // Handshake — first message client gửi sau khi WebSocket connect.
// //
// // Format:
// //
// //	[0x00]                          msgType = MsgHandshake (1 byte)
// //	[uint16] protocolVersion        2 bytes
// //	[int32]  userId                 4 bytes
// //	[string] token (JWT)            2 + N bytes
// //	[string] gameSessionId          2 + N bytes
// //
// // Tại sao client phải gửi userId rõ ràng dù JWT đã chứa?
// // → Tiết kiệm parse JWT chỉ để biết userId. Server vẫn verify JWT để confirm khớp.
// type Handshake struct {
// 	ProtocolVersion uint16
// 	UserID          int32
// 	Token           string
// 	GameSessionID   string
// }

// func (m *Handshake) Decode(data []byte) error {
// 	d := NewDecoder(data)
// 	var err error
// 	if m.ProtocolVersion, err = d.ReadUint16(); err != nil {
// 		return err
// 	}
// 	if m.UserID, err = d.ReadInt32(); err != nil {
// 		return err
// 	}
// 	if m.Token, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	if m.GameSessionID, err = d.ReadString(); err != nil {
// 		return err
// 	}
// 	return nil
// }

// // HandshakeAck — server reply khi handshake OK.
// //
// //	[0x80] msgType
// //	(không có payload, đủ rồi)
// func EncodeHandshakeAck() []byte {
// 	enc := NewEncoder(MsgHandshakeAck)
// 	return enc.Bytes()
// }

// // HandshakeNack — server reject handshake.
// //
// //	[0x81] msgType
// //	[uint8] reason (NackReasonVersion, NackReasonAuth, ...)
// func EncodeHandshakeNack(reason uint8) []byte {
// 	enc := NewEncoder(MsgHandshakeNack)
// 	enc.WriteUint8(reason)
// 	return enc.Bytes()
// }
