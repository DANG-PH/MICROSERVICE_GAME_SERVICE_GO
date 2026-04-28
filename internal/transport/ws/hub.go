package ws

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Hub struct {
	log *slog.Logger
	bus *Bus // nil = single-instance mode (dev/test)

	mu          sync.RWMutex
	connsByUser map[int32]*Conn
	roomsByMap  map[string]map[*Conn]struct{}
}

// NewHub — bus có thể nil (chạy single instance, ví dụ test).
func NewHub(log *slog.Logger, bus *Bus) *Hub {
	h := &Hub{
		log:         log,
		bus:         bus,
		connsByUser: make(map[int32]*Conn),
		roomsByMap:  make(map[string]map[*Conn]struct{}),
	}

	if bus != nil {
		bus.SetHandlers(
			h.localBroadcastToMap,
			func(userID int32, data []byte) {
				h.localSendToUser(userID, data) // discard bool
			},
			h.localKickUser,
		)
	}

	return h
}

// === API PUBLIC — call site không thay đổi ===

func (h *Hub) BroadcastToMap(mapID string, data []byte, excludeConn *Conn) {
	// 1. Broadcast local trước (latency thấp nhất).
	var excludeUserID int32
	if excludeConn != nil {
		excludeUserID = excludeConn.userID
	}
	h.localBroadcastToMap(mapID, data, excludeUserID)

	// 2. Fan-out cross-instance qua Redis.
	if h.bus != nil {
		// Fire-and-forget: publish chậm vài ms cũng không nên block call site.
		// Timeout ngắn — nếu Redis chậm thì drop, không backlog.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := h.bus.PublishBroadcast(ctx, mapID, data, excludeUserID); err != nil {
				h.log.Warn("publish broadcast failed", "err", err, "mapID", mapID)
			}
		}()
	}
}

func (h *Hub) SendToUser(userID int32, data []byte) bool {
	// Try local trước.
	if h.localSendToUser(userID, data) {
		return true
	}

	// Local không có → user có thể đang ở instance khác. Publish.
	if h.bus != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := h.bus.PublishSendToUser(ctx, userID, data); err != nil {
				h.log.Warn("publish send failed", "err", err, "userID", userID)
			}
		}()
		// Trả true vì có thể có instance khác sẽ deliver.
		// Caller không thể biết chắc — chấp nhận uncertainty.
		return true
	}
	return false
}

// KickUser kick user dù họ đang ở instance nào.
// Dùng khi: register conn mới mà thấy user đã có conn cũ ở instance khác.
func (h *Hub) KickUser(userID int32) {
	h.localKickUser(userID)
	if h.bus != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := h.bus.PublishKickUser(ctx, userID); err != nil {
				h.log.Warn("publish kick failed", "err", err, "userID", userID)
			}
		}()
	}
}

// === LOCAL METHODS — chỉ thao tác conn trên instance này ===

func (h *Hub) localBroadcastToMap(mapID string, data []byte, excludeUserID int32) {
	h.mu.RLock()
	room, ok := h.roomsByMap[mapID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	conns := make([]*Conn, 0, len(room))
	for c := range room {
		if c.userID != excludeUserID {
			conns = append(conns, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range conns {
		c.Send(data)
	}
}

func (h *Hub) localSendToUser(userID int32, data []byte) bool {
	h.mu.RLock()
	c, ok := h.connsByUser[userID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	c.Send(data)
	return true
}

func (h *Hub) localKickUser(userID int32) {
	h.mu.Lock()
	c, ok := h.connsByUser[userID]
	if ok {
		delete(h.connsByUser, userID)
		h.removeFromRoomLocked(c)
	}
	h.mu.Unlock()

	if ok {
		c.Close()
		h.log.Info("user kicked", "userID", userID)
	}
}

// === REGISTER/UNREGISTER — sửa nhẹ để dùng cross-instance kick ===

func (h *Hub) register(c *Conn) {
	h.mu.Lock()
	if oldConn, exists := h.connsByUser[c.userID]; exists {
		h.log.Info("user already connected on this instance, kicking old conn", "userID", c.userID)
		oldConn.Close()
		h.removeFromRoomLocked(oldConn)
	}
	h.connsByUser[c.userID] = c
	h.addToRoomLocked(c)
	h.mu.Unlock()

	// Kick conn cũ ở instance KHÁC nếu có.
	// Note: cũng publish kể cả khi đã kick local — vì user có thể có conn ở cả 2 instance.
	// Bus sẽ skip echo về chính mình.
	if h.bus != nil {
		go func(uid int32) {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := h.bus.PublishKickUser(ctx, uid); err != nil {
				h.log.Warn("publish kick on register failed", "err", err)
			}
		}(c.userID)
	}

	h.log.Info("conn registered", "userID", c.userID, "mapID", c.mapID)
}

func (h *Hub) unregister(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.connsByUser[c.userID]; ok && existing == c {
		delete(h.connsByUser, c.userID)
	}
	h.removeFromRoomLocked(c)
}

// MoveToRoom giữ nguyên — đổi map chỉ là local concern.
func (h *Hub) MoveToRoom(c *Conn, newMap string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeFromRoomLocked(c)
	c.mapID = newMap
	h.addToRoomLocked(c)
}

func (h *Hub) addToRoomLocked(c *Conn) {
	if c.mapID == "" {
		return
	}
	room, ok := h.roomsByMap[c.mapID]
	if !ok {
		room = make(map[*Conn]struct{})
		h.roomsByMap[c.mapID] = room
	}
	room[c] = struct{}{}
}

func (h *Hub) removeFromRoomLocked(c *Conn) {
	if c.mapID == "" {
		return
	}
	room, ok := h.roomsByMap[c.mapID]
	if !ok {
		return
	}
	delete(room, c)
	if len(room) == 0 {
		delete(h.roomsByMap, c.mapID)
	}
}

func (h *Hub) Stats() (totalConns int, totalRooms int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connsByUser), len(h.roomsByMap)
}
