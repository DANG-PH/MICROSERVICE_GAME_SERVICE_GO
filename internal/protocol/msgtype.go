package protocol

// PROTOCOL_VERSION — tăng mỗi lần đổi schema breaking.
// Client phải gửi version này trong handshake, server reject nếu mismatch.
//
// Tại sao uint16? Lý thuyết version chỉ cần uint8 (0-255),
// nhưng uint16 cho phép 65535 lần breaking change — đủ cho cả vòng đời sản phẩm.
// Thực tế ít khi vượt quá 10, nhưng tốn thêm 1 byte là không đáng để tiếc.
const PROTOCOL_VERSION uint16 = 1

// Tại sao custom binary thay vì JSON hoặc Protobuf?
//
// JSON:
//   {"type":"player_move","x":10.5,"y":20.3} = ~40 bytes
//   [0x01][float32 x][float32 y]             = 9 bytes
//   Game gửi 60 packet/giây/player — JSON tốn gấp 4-5x bandwidth, thêm CPU parse.
//
// Protobuf:
//   Tốt, nhỏ gọn, có schema. Nhưng ẩn hết phần byte-level bên dưới.
//   Mục đích của codebase này là học cách bytes hoạt động — dùng Protobuf
//   thì tool làm hết, không học được gì.
//   Khi nào cần scale thật sự thì migrate sang Protobuf cũng không muộn.
//
// Custom binary:
//   Kiểm soát từng byte — biết chính xác packet trông như thế nào.
//   Không có overhead nào ngoài data thật sự cần gửi.
//   Phù hợp để học và cho game nhỏ đến trung bình.

// Message types.
// Client → Server: 0x01 - 0x7F
// Server → Client: 0x80 - 0xFF
//
// Tại sao dùng hệ thập lục phân (0x...) thay vì số thường?
//
//	1 byte = 8 bit = đúng 2 ký tự hex.
//	0xFF trông như 1 byte đầy, 0x80 trông như nửa byte — trực quan khi debug.
//	Nếu dùng số thường: 128, 129, 255 — không thấy pattern gì cả.
//	Khi mở Wireshark hoặc hexdump, mọi thứ hiện ra dạng hex — quen hex từ đầu
//	thì debug dễ hơn nhiều.
//
// Tại sao ranh giới tại 128 (0x80)?
//
//	uint8 có 256 giá trị (0-255), chia đôi tại 128 là tự nhiên nhất.
//	Quan trọng hơn: 128 = 1000 0000 trong nhị phân.
//	Tức là bit đầu tiên (bit cao nhất) = 1.
//	Mọi số từ 128-255 đều có bit đầu = 1.
//	Mọi số từ 0-127 đều có bit đầu = 0.
//	→ Chỉ cần check 1 bit là biết hướng message, không cần so sánh toàn bộ giá trị.
//	Trong code: if msgType >= 0x80 { // server→client }
//
// Tách 2 dải để hex dump nhìn vào là biết hướng nào.
const (
	// Client → Server (0x00 - 0x7F)
	// 0x00 dành riêng cho handshake vì đây là packet đặc biệt —
	// server chưa biết client là ai, chưa auth, chưa có session.
	// Tách ra 0x00 để dễ nhận dạng trong log: thấy 0x00 là biết ngay đây là kết nối mới.
	MsgHandshake  uint8 = 0x00 // first packet: version + token + sessionId
	MsgPlayerMove uint8 = 0x01 // client gửi move

	// Server → Client (0x80 - 0xFF)
	// Bắt đầu từ 0x80 vì đây là điểm bit đầu tiên chuyển từ 0 → 1.
	// 0x80, 0x81, 0x82 đặt cạnh nhau để các message liên quan (handshake flow)
	// nằm gần nhau — dễ tìm trong code và trong hexdump.
	MsgHandshakeAck    uint8 = 0x80 // accept handshake
	MsgHandshakeNack   uint8 = 0x81 // reject handshake (version mismatch, auth fail)
	MsgPlayerSync      uint8 = 0x82 // broadcast move của player khác
	MsgPlayerSyncBatch uint8 = 0x83 // broadcast tickrate toàn map

	// 0xFF = 1111 1111 — tất cả bit đều bật, dễ nhận ra trong hexdump.
	// Dùng cho error vì nhìn vào là thấy ngay "có gì đó sai".
	// Convention này phổ biến: 0xFF thường mang nghĩa "invalid" hoặc "error"
	// trong nhiều protocol (USB, CAN bus, ...).
	MsgError uint8 = 0xFF // generic error (kick, internal error)
)

// Handshake reject reasons.
// Đây là payload bên trong packet 0x81, không phải msgType.
// Dùng số thường (1, 2, 3) thay vì hex vì:
//   - Không cần trick bit gì ở đây, chỉ là mã lý do đơn giản.
//   - Số thường dễ đọc hơn khi log ra: "nackReason=2" rõ hơn "nackReason=0x02".
//   - Không có pattern nhị phân nào cần khai thác ở đây.
//
// Thứ tự 1, 2, 3 theo mức độ phổ biến khi xảy ra:
//
//	Version mismatch hay gặp nhất khi client cũ chưa update.
//	Auth fail hay gặp thứ hai khi token hết hạn.
//	Session fail ít gặp hơn.
//	Internal error hiếm nhất — chỉ khi server có bug.
const (
	NackReasonVersion  uint8 = 1
	NackReasonAuth     uint8 = 2
	NackReasonSession  uint8 = 3
	NackReasonInternal uint8 = 99 // TODO: nên đổi thành 0xFF cho nhất quán với convention "max value = catch-all"
)
