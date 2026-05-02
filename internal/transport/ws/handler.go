package ws

import (
	"log/slog"

	"github.com/DANG-PH/game-service-go/internal/game/player"
	"github.com/DANG-PH/game-service-go/internal/game/state"
	"github.com/DANG-PH/game-service-go/internal/shared/messages"
	"github.com/DANG-PH/game-service-go/internal/shared/protocol"
)

// Handler là nơi route message theo msgType byte đầu tiên.
// Mỗi msgType có 1 case xử lý riêng.
//
// Pattern này gọi là "dispatcher" — đơn giản, dễ đọc, không cần reflection.
// Khi thêm message mới: thêm const ở protocol/msgtype.go, thêm case ở đây, tạo file message struct.
// - KHÔNG broadcast trực tiếp — chỉ update state.
// - Tick loop sẽ broadcast 20Hz.
// - Vẫn giữ Redis update (cần cho NestJS đọc + cron flush DB).
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
// data[0] = msgType, data[1:] = payload.
func (h *Handler) Handle(c *Conn, data []byte) {
	if len(data) == 0 {
		return
	}

	msgType := data[0]
	payload := data[1:]

	switch msgType {
	case protocol.MsgPlayerMove:
		h.handlePlayerMove(c, payload)

	default:
		h.log.Warn("unknown message type", "type", msgType, "userID", c.userID)
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

func (h *Handler) handlePlayerMove(c *Conn, payload []byte) {
	var m messages.PlayerMove
	if err := m.Decode(payload); err != nil {
		h.log.Warn("decode PlayerMove failed", "err", err, "userID", c.userID)
		return
	}

	var ms *state.MapState
	if c.mapID != m.MapID {
		ms = h.switchMap(c, m.MapID) // atomic từ góc nhìn handler
	} else {
		ms = h.manager.GetOrCreateMap(m.MapID)
	}

	ms.UpdateFromMove(c.userID, &m)
}
