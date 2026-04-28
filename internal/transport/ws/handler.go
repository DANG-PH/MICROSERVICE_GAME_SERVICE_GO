package ws

import (
	"context"
	"log/slog"
	"time"

	"github.com/DANG-PH/game-service-go/internal/game/player"
	"github.com/DANG-PH/game-service-go/internal/shared/messages"
	"github.com/DANG-PH/game-service-go/internal/shared/protocol"
)

// Handler là nơi route message theo msgType byte đầu tiên.
// Mỗi msgType có 1 case xử lý riêng.
//
// Pattern này gọi là "dispatcher" — đơn giản, dễ đọc, không cần reflection.
// Khi thêm message mới: thêm const ở protocol/msgtype.go, thêm case ở đây, tạo file message struct.
type Handler struct {
	log           *slog.Logger
	hub           *Hub
	playerService *player.Service
}

func NewHandler(log *slog.Logger, hub *Hub, ps *player.Service) *Handler {
	return &Handler{
		log:           log,
		hub:           hub,
		playerService: ps,
	}
}

// Handle implement MessageHandler interface.
// Được gọi từ Conn.readLoop mỗi khi có message từ client.
//
// data[0] = msgType, data[1:] = payload.
func (h *Handler) Handle(c *Conn, data []byte) {
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

	if c.mapID != m.MapID {
		h.log.Info("conn changing map via move", "userID", c.userID, "from", c.mapID, "to", m.MapID)
		h.hub.MoveToRoom(c, m.MapID)
	}

	// === BROADCAST NGAY — không chờ Redis ===
	// Pattern: hot path không block trên I/O không critical.
	// Redis chỉ phục vụ NestJS đọc state (mapSnapshot, getValidSession),
	// trễ vài ms không ảnh hưởng gameplay.
	syncPacket := player.BuildSyncPacket(c.userID, &m)
	h.hub.BroadcastToMap(m.MapID, syncPacket, c)

	// === FIRE-AND-FORGET Redis update ===
	// Spawn goroutine cho mỗi update.
	// Goroutine ở Go RẺ (~2KB stack), không phải lo overhead như tạo thread.
	// Context timeout 1s — Redis chậm hơn → drop request đó, không backlog.
	go func(userID int32, move messages.PlayerMove) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := h.playerService.HandleMove(ctx, userID, &move); err != nil {
			h.log.Warn("redis update failed", "err", err, "userID", userID)
		}
	}(c.userID, m)

	// Tại sao cần go ở đây?
	// client không đợi response. Nhưng readLoop mới là người đợi:
	// readLoop gọi handler.Handle(c, data)
	// 	→ handlePlayerMove chạy
	// 	→ nếu không có go: block ở Redis update (có thể 50-200ms)
	// 	→ trong thời gian đó readLoop KHÔNG đọc packet mới
	// 	→ client gửi tiếp 10 move packet → tất cả nằm chờ trong TCP buffer
	// 	→ gameplay bị lag
	// Có go:
	// 	→ broadcast ngay (< 1ms)
	// 	→ Redis update chạy ngầm
	// 	→ readLoop tiếp tục đọc packet mới ngay lập tức
	// Game realtime mỗi giây có thể có hàng chục move packet — block readLoop dù 50ms cũng ảnh hưởng gameplay.
	// TODO: goroutine per-packet có thể ghi đè state cũ lên state mới nếu Redis chậm.
	// Chấp nhận được vì Redis chỉ là snapshot cho NestJS đọc, không ảnh hưởng gameplay.
	// Fix nếu NestJS cần Redis chính xác tuyệt đối.
}
