package ws

import (
	"context"
	"log/slog"
	"time"

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

func (h *Handler) handlePlayerMove(c *Conn, payload []byte) {
	var m messages.PlayerMove
	if err := m.Decode(payload); err != nil {
		h.log.Warn("decode PlayerMove failed", "err", err, "userID", c.userID)
		return
	}

	// Đổi map — cleanup state cũ + chuyển room.
	if c.mapID != m.MapID {
		if c.mapID != "" {
			h.manager.RemovePlayerFromMap(c.mapID, c.userID) // tự cleanup empty map
		}
		h.hub.MoveToRoom(c, m.MapID)
	}

	// Update state — UpdateFromMove tự tạo entry nếu chưa có.
	// Tick loop sẽ broadcast (KHÔNG broadcast ở đây).
	ms := h.manager.GetOrCreateMap(m.MapID)
	ms.UpdateFromMove(c.userID, &m)

	// Redis update — fire-and-forget per packet.
	//
	// Tại sao cần go ở đây?
	//   ─ readLoop gọi handler tuần tự, nếu block ở Redis (50-200ms)
	//     thì packet kế tiếp phải chờ → gameplay lag.
	//   ─ Client gửi 10-20 move packet/giây, không thể chờ Redis.
	//
	// Goroutine cho phép:
	//   ─ Hot path (update state in-memory + tick broadcast) chạy < 1ms
	//   ─ Redis update chạy ngầm, không ảnh hưởng latency
	//
	// TODO Phase 2:
	//   ─ Bounded queue + worker dedicated thay vì per-packet goroutine
	//     (chống goroutine leak khi Redis chậm + spam packet)
	//   ─ Batch Redis update trong tick loop, dùng pipeline
	//     gom nhiều player vào 1 round-trip
	//
	// Trade-off hiện tại: goroutine có thể ghi Redis state CŨ lên state MỚI
	// nếu Redis chậm. Chấp nhận được vì Redis chỉ là snapshot cho NestJS,
	// không ảnh hưởng gameplay (gameplay đọc từ in-memory MapState).
	go func(userID int32, move messages.PlayerMove) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := h.playerService.HandleMove(ctx, userID, &move); err != nil {
			h.log.Warn("redis update failed", "err", err, "userID", userID)
		}
	}(c.userID, m)
}
