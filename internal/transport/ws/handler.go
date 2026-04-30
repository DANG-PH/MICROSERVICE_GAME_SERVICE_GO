package ws

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/DANG-PH/game-service-go/internal/game/player"
	"github.com/DANG-PH/game-service-go/internal/game/state"
	"github.com/DANG-PH/game-service-go/internal/shared/enums"
	"github.com/DANG-PH/game-service-go/internal/shared/messages"
	"github.com/DANG-PH/game-service-go/internal/shared/protocol"
)

type Handler struct {
	log           *slog.Logger
	hub           *Hub
	manager       *state.Manager
	playerService *player.Service

	// === DEBUG ===
	debugLastReceivedTime map[int32]int64
	debugMu               sync.Mutex
}

func NewHandler(log *slog.Logger,
	hub *Hub,
	manager *state.Manager,
	ps *player.Service,
) *Handler {
	return &Handler{
		log:                   log,
		hub:                   hub,
		manager:               manager,
		playerService:         ps,
		debugLastReceivedTime: make(map[int32]int64),
	}
}

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

	// === DEBUG: log gap giữa các PlayerMove từ cùng userID ===
	now := time.Now().UnixMilli()
	h.debugMu.Lock()
	lastTime := h.debugLastReceivedTime[c.userID]
	h.debugLastReceivedTime[c.userID] = now
	h.debugMu.Unlock()

	gap := now - lastTime
	if lastTime == 0 {
		gap = -1
	}

	// Chỉ log khi gap > 200ms (bình thường gap = 100ms)
	// hoặc -1 (lần đầu)
	if gap > 200 || gap == -1 {
		h.log.Info("PlayerMove received (gap large)",
			"userID", c.userID,
			"x", m.X,
			"y", m.Y,
			"trangthai", enums.TrangthaiToString(m.Trangthai),
			"gap_ms", gap,
		)
	}

	// Đổi map
	if c.mapID != m.MapID {
		h.log.Info("conn changing map", "userID", c.userID, "from", c.mapID, "to", m.MapID)

		if c.mapID != "" {
			h.manager.RemovePlayerFromMap(c.mapID, c.userID)
		}

		h.hub.MoveToRoom(c, m.MapID)
		c.mapID = m.MapID
	}

	ms := h.manager.GetOrCreateMap(m.MapID)
	ms.UpdateFromMove(c.userID, &m)

	// Redis update — fire-and-forget per packet
	go func(userID int32, move messages.PlayerMove) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if err := h.playerService.HandleMove(ctx, userID, &move); err != nil {
			h.log.Warn("redis update failed", "err", err, "userID", userID)
		}
	}(c.userID, m)
}
