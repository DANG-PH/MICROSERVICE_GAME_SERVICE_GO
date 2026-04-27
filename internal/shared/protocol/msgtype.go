package protocol

// PROTOCOL_VERSION — tăng mỗi lần đổi schema breaking.
// Client phải gửi version này trong handshake, server reject nếu mismatch.
const PROTOCOL_VERSION uint16 = 1

// Message types.
// Client → Server: 0x01 - 0x7F
// Server → Client: 0x80 - 0xFF
// Tách 2 dải để hex dump nhìn vào là biết hướng nào.
const (
	// Client → Server
	MsgHandshake  uint8 = 0x00 // first packet: version + token + sessionId
	MsgPlayerMove uint8 = 0x01 // client gửi move

	// Server → Client
	MsgHandshakeAck  uint8 = 0x80 // accept handshake
	MsgHandshakeNack uint8 = 0x81 // reject handshake (version mismatch, auth fail)
	MsgPlayerSync    uint8 = 0x82 // broadcast move của player khác
	MsgError         uint8 = 0xFF // generic error (kick, internal error)
)

// Handshake reject reasons.
const (
	NackReasonVersion    uint8 = 1
	NackReasonAuth       uint8 = 2
	NackReasonSession    uint8 = 3
	NackReasonInternal   uint8 = 99
)
