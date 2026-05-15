package ws

import (
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/DANG-PH/game-service-go/internal/game/player"
	"github.com/DANG-PH/game-service-go/internal/game/state"
	pb "github.com/DANG-PH/game-service-go/internal/protocol/pb"
)

// Handler là nơi route message theo Envelope.oneof.
// Mỗi message type có 1 case xử lý riêng.
//
// Pattern này gọi là "dispatcher" — đơn giản, dễ đọc, không cần reflection.
// Khi thêm message mới: thêm message vào game.proto, generate lại, thêm case ở đây.
// - KHÔNG broadcast trực tiếp — chỉ update state.
// - Tick loop sẽ broadcast 20Hz.
// - Vẫn giữ Redis update (cần cho NestJS đọc + cron flush DB).
//
// THAY ĐỔI SO VỚI CUSTOM BINARY:
//   - Không còn switch data[0] msgType — thay bằng proto.Unmarshal + switch env.Payload.(type)
//   - Không cần tách data[1:] payload riêng nữa
//   - Khi thêm message mới: thêm vào game.proto thay vì thêm const ở protocol/msgtype.go
type Handler struct {
	log           *slog.Logger
	hub           *Hub
	manager       *state.Manager
	playerService *player.Service
}

func NewHandler(log *slog.Logger,
	hub *Hub,
	manager *state.Manager,
	ps *player.Service,
) *Handler {
	return &Handler{
		log:           log,
		hub:           hub,
		manager:       manager,
		playerService: ps,
	}
}

// Handle implement MessageHandler interface.
// Được gọi từ Conn.readLoop mỗi khi có message từ client.
//
// THAY ĐỔI SO VỚI CUSTOM BINARY:
//   - Cũ: msgType := data[0], payload := data[1:], switch msgType
//   - Mới: proto.Unmarshal toàn bộ data → switch env.Payload.(type)
func (h *Handler) Handle(c *Conn, data []byte) {
	if len(data) == 0 {
		return
	}

	var env pb.Envelope
	if err := proto.Unmarshal(data, &env); err != nil {
		h.log.Warn("unmarshal envelope failed", "err", err, "userID", c.userID)
		return
	}

	switch p := env.Payload.(type) {
	case *pb.Envelope_PlayerMove:
		h.handlePlayerMove(c, p.PlayerMove)

	default:
		h.log.Warn("unknown message type", "userID", c.userID)
	}
}

// switchMap chuyển player từ map cũ sang map mới,
// đồng thời giữ hai layer luôn đồng bộ:
//
//	Hub.roomsByMap — xác định conn nào nhận broadcast packet
//	Manager.maps   — lưu game state thực sự của từng map
//
// Hai layer phải được cập nhật cùng nhau. Nếu gọi MoveToRoom hoặc
// GetOrCreateMap riêng lẻ, state có thể lệch — ví dụ Hub đã route
// packet sang map mới nhưng MapState vẫn nằm ở map cũ.
//
// Thứ tự thực hiện quan trọng:
//  1. Xóa khỏi MapState cũ trước — tránh broadcast vị trí cũ
//     vào room cũ trong khoảng thời gian chuyển tiếp.
//  2. MoveToRoom — Hub route packet sang room mới.
//  3. GetOrCreateMap — khởi tạo MapState nếu chưa có player nào vào map này.
//
// Hub không cần biết player có mặc đồ k, đứng ở tọa độ nào
// Manager không cần biết WebSocket conn nào đang mở
// Logic chỉ được gọi từ handlePlayerMove khi c.mapID != mapID mới.
func (h *Handler) switchMap(c *Conn, newMapID string) *state.MapState {
	if c.mapID != "" {
		h.manager.RemovePlayerFromMap(c.mapID, c.userID)
	}
	h.hub.MoveToRoom(c, newMapID)             // update Hub routing
	return h.manager.GetOrCreateMap(newMapID) // đảm bảo MapState tồn tại
}

// handlePlayerMove nhận *pb.PlayerMove trực tiếp — không cần decode thủ công như custom binary.
//
// THAY ĐỔI SO VỚI CUSTOM BINARY:
//   - Cũ: handlePlayerMove(c *Conn, payload []byte) → m.Decode(payload)
//   - Mới: handlePlayerMove(c *Conn, m *pb.PlayerMove) — proto đã unmarshal từ Handle()
func (h *Handler) handlePlayerMove(c *Conn, m *pb.PlayerMove) {
	var ms *state.MapState
	if c.mapID != m.MapId {
		ms = h.switchMap(c, m.MapId) // atomic từ góc nhìn handler
	} else {
		ms = h.manager.GetOrCreateMap(m.MapId)
	}

	ms.UpdateFromMove(c.userID, m)
}
