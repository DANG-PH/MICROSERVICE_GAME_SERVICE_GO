package ws

import (
	"log/slog"
	"sync"
)

// Hub quản lý tất cả connection và phân loại theo map.
//
// THIẾT KẾ LOCK:
// 1 sync.RWMutex global cho cả hub thì đơn giản nhưng contention cao —
// mỗi broadcast phải acquire lock để iterate connection trong map đó.
//
// Cải thiện: sharded map theo mapID. Mỗi map có lock riêng.
// Nhưng giai đoạn này 1 lock đơn giản đã đủ — game của bạn chưa có 100k CCU.
// Khi nào benchmark thấy lock contention thì refactor sang sharded.
//
// Pattern này gọi là "central registry" — nhiều framework dùng (Phoenix Channels, ws_bridge, ...).
type Hub struct {
	log *slog.Logger

	mu sync.RWMutex
	// userID → conn. Để gửi point-to-point (kick socket, error reply).
	connsByUser map[int32]*Conn
	// mapID → set of conn. Để broadcast tới room.
	// Dùng map[*Conn]struct{} thay vì slice vì delete O(1).
	roomsByMap map[string]map[*Conn]struct{}
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		log:         log,
		connsByUser: make(map[int32]*Conn),
		roomsByMap:  make(map[string]map[*Conn]struct{}),
	}
}

// register thêm conn sau khi handshake thành công.
// Nếu user này đã có connection cũ → kick cũ trước (chỉ cho phép 1 connection per user).
// Khớp với pattern NestJS: nếu mở 2 tab thì 1 tab bị kick.
func (h *Hub) register(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Kick connection cũ nếu có.
	if oldConn, exists := h.connsByUser[c.userID]; exists {
		h.log.Info("user already connected, kicking old conn", "userID", c.userID)
		// Close ngoài lock để tránh deadlock — nhưng ở đây oldConn.Close()
		// chỉ close channel, không gọi back vào hub → an toàn.
		oldConn.Close()
		// Cleanup oldConn khỏi room.
		h.removeFromRoomLocked(oldConn)
	}

	h.connsByUser[c.userID] = c
	h.addToRoomLocked(c)

	h.log.Info("conn registered", "userID", c.userID, "mapID", c.mapID,
		"totalConns", len(h.connsByUser))
}

// unregister gọi khi conn close (read loop return).
func (h *Hub) unregister(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Chỉ xóa nếu conn này vẫn là conn đang active của user
	// (tránh case kick xảy ra: oldConn.unregister() chạy SAU newConn.register()
	// → sẽ vô tình xóa newConn ra khỏi map).
	if existing, ok := h.connsByUser[c.userID]; ok && existing == c {
		delete(h.connsByUser, c.userID)
	}
	h.removeFromRoomLocked(c)

	h.log.Info("conn unregistered", "userID", c.userID,
		"totalConns", len(h.connsByUser))
}

// MoveToRoom đổi map của conn. Gọi khi NestJS bắn event setMap qua Redis pub/sub
// (sau này implement) — Phase 1 chưa cần.
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

// BroadcastToMap gửi message tới tất cả conn trong map, TRỪ conn excludeConn.
// excludeConn = nil để gửi tới tất cả.
//
// Pattern Socket.IO: this.server.to(MAP:x).emit(...) gửi tới tất cả.
// Pattern client.to(MAP:x).emit(...) gửi tới tất cả TRỪ chính client đó.
// Hàm này hỗ trợ cả 2 case bằng excludeConn.
func (h *Hub) BroadcastToMap(mapID string, data []byte, excludeConn *Conn) {
	h.mu.RLock()
	room, ok := h.roomsByMap[mapID]
	if !ok {
		h.mu.RUnlock()
		return
	}

	// Copy danh sách conn ra slice để release lock sớm.
	// Nếu giữ lock trong khi Send() → các Send() bị block khi slow client → block luôn các request khác.
	conns := make([]*Conn, 0, len(room))
	for c := range room {
		if c != excludeConn {
			conns = append(conns, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range conns {
		c.Send(data)
	}
}

// SendToUser gửi message tới 1 user cụ thể.
func (h *Hub) SendToUser(userID int32, data []byte) bool {
	h.mu.RLock()
	c, ok := h.connsByUser[userID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	c.Send(data)
	return true
}

// Stats trả về metric cho monitoring.
func (h *Hub) Stats() (totalConns int, totalRooms int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connsByUser), len(h.roomsByMap)
}
