package ws

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/DANG-PH/game-service-go/internal/game/state"
	"github.com/DANG-PH/game-service-go/internal/infra/bus"
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
//
// MULTI-INSTANCE:
// Hub implement BusHandler để Bus gọi vào khi nhận message cross-instance.
// 3 method On* là entrypoint cho message từ instance khác:
//   - OnBroadcast: instance khác broadcast tới map → fan-out tới conn local trong map đó
//   - OnSendToUser: instance khác gửi tới user → check user có ở instance này không
//   - OnKickUser: instance khác bảo kick user → kick nếu user ở instance này
type Hub struct {
	log     *slog.Logger
	bus     bus.BusInterface // nil = single-instance mode (dev/test)
	manager *state.Manager

	mu          sync.RWMutex
	connsByUser map[int32]*Conn
	roomsByMap  map[string]map[*Conn]struct{}

	// Worker pool cho NATS publish
	publishCh chan publishJob
}

type publishJob struct {
	kind          publishKind
	mapID         string
	data          []byte
	excludeUserID int32
	userID        int32 // dùng cho kick
}

type publishKind uint8

const (
	publishBroadcast publishKind = iota
	publishKick
)

const (
	publishWorkers   = 16   // tăng từ 4 lên 16
	publishChBufSize = 4096 // tăng từ 1024 lên 4096
)

// Compile-time check: Hub phải satisfy BusHandler interface.
// Nếu thiếu method OnBroadcast/OnSendToUser/OnKickUser → compile error tại đây,
// không phải đợi runtime crash khi bus.SetHandler được gọi.
//
// Đây là idiom phổ biến trong Go: đặt assertion ngay đầu file để rõ ràng
// "type này commit implement interface kia".
// (*Hub)(nil)Đây là type conversion. Cú pháp T(value) nghĩa là ép value thành type T.
// var x = nil       // ❌ compile error: use of untyped nil
// var x *Hub = nil  // ✅ pointer to Hub, nil
// var x = (*Hub)(nil) // ✅ tương đương dòng trên
// Check như này tránh đổi nhầm tên mà compile vẫn đúng, nếu ai đó thay đổi tên hàm thì k implements -> phải fail luôn
var _ bus.BusHandler = (*Hub)(nil)

// NewHub — bus có thể nil (chạy single instance, ví dụ test).
func NewHub(log *slog.Logger, bus bus.BusInterface, manager *state.Manager) *Hub {
	h := &Hub{
		log:         log,
		bus:         bus,
		manager:     manager,
		connsByUser: make(map[int32]*Conn),
		roomsByMap:  make(map[string]map[*Conn]struct{}),
		publishCh:   make(chan publishJob, publishChBufSize),
	}

	if bus != nil {
		// Wire 1 dòng: hub self-register làm handler của bus.
		// Khi bus nhận message cross-instance → gọi h.OnBroadcast/OnSendToUser/OnKickUser.
		bus.SetHandler(h)

		for i := 0; i < publishWorkers; i++ {
			go h.publishWorker()
		}

		bus.SubscribePlayerDisconnect(func(userID int32, mapID string) {
			h.manager.RemovePlayerFromMap(mapID, userID)
		})
	}

	return h
}

// === BUSHANDLER IMPLEMENTATION ===
// 3 method này được Bus gọi khi nhận message từ instance khác.
// Cũng được dùng làm "local-only" path cho API public bên dưới.

// OnBroadcast fan-out tới conn local trong map.
// Dùng map[*Conn]struct{} thay vì slice vì delete O(1).
// struct{} là empty struct — type duy nhất trong Go không chiếm memory.
// Dùng khi chỉ quan tâm key có tồn tại hay không, không quan tâm value.
func (h *Hub) OnBroadcast(mapID string, data []byte, excludeUserID int32) {
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
		if c.UserID() != excludeUserID {
			conns = append(conns, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range conns {
		c.Send(data)
	}
}

// OnKickUser kick user nếu họ ở instance này.
func (h *Hub) OnKickUser(userID int32) {
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

// === API PUBLIC — call site không thay đổi ===

// BroadcastToMap gửi message tới tất cả conn trong map, TRỪ conn excludeConn.
// excludeConn = nil để gửi tới tất cả.
//
// Pattern Socket.IO: this.server.to(MAP:x).emit(...) gửi tới tất cả.
// Pattern client.to(MAP:x).emit(...) gửi tới tất cả TRỪ chính client đó.
// Hàm này hỗ trợ cả 2 case bằng excludeConn.
func (h *Hub) BroadcastToMap(mapID string, data []byte, excludeConn *Conn) {
	var excludeUserID int32
	if excludeConn != nil {
		excludeUserID = excludeConn.userID
	}
	h.BroadcastToMapExcludeUser(mapID, data, excludeUserID)
}

// BroadcastToMapExcludeUser giống BroadcastToMap nhưng exclude theo userID
// thay vì theo *Conn — tiện cho tick loop (không cần lookup conn).
func (h *Hub) BroadcastToMapExcludeUser(mapID string, data []byte, excludeUserID int32) {
	// Local broadcast
	h.OnBroadcast(mapID, data, excludeUserID)

	// Cross-instance

	// Tạo go mỗi lần GC có Overhead, dùng pool goroutine + buffer channel để tối ưu phần này
	// if h.bus != nil {
	// 	go func() {
	// 		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	// 		defer cancel()
	// 		if err := h.bus.PublishBroadcast(ctx, mapID, data, excludeUserID); err != nil {
	// 			h.log.Warn("publish broadcast failed", "err", err, "mapID", mapID)
	// 		}
	// 	}()
	// }

	h.enqueue(publishJob{
		kind:          publishBroadcast,
		mapID:         mapID,
		data:          data,
		excludeUserID: excludeUserID,
	})
}

// KickUser kick user dù họ đang ở instance nào.
// Dùng khi: register conn mới mà thấy user đã có conn cũ ở instance khác.
func (h *Hub) KickUser(userID int32) {
	h.OnKickUser(userID) // local
	// if h.bus != nil {
	// 	go func() {
	// 		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	// 		defer cancel()
	// 		if err := h.bus.PublishKickUser(ctx, userID); err != nil {
	// 			h.log.Warn("publish kick failed", "err", err, "userID", userID)
	// 		}
	// 	}()
	// }

	h.enqueue(publishJob{
		kind:   publishKick,
		userID: userID,
	})
}

// === REGISTER/UNREGISTER — sửa nhẹ để dùng cross-instance kick ===

// register thêm conn sau khi handshake thành công.
// Nếu user này đã có connection cũ → kick cũ trước (chỉ cho phép 1 connection per user).
// Khớp với pattern NestJS: nếu mở 2 tab thì 1 tab bị kick.
func (h *Hub) register(c *Conn) {
	h.mu.Lock()
	// Kick connection cũ nếu có ở instance này.
	if oldConn, exists := h.connsByUser[c.UserID()]; exists {
		h.log.Info("user already connected on this instance, kicking old conn", "userID", c.UserID())
		// Close ngoài lock để tránh deadlock — nhưng ở đây oldConn.Close()
		// chỉ close channel, không gọi back vào hub → an toàn.
		oldConn.Close()
		// Cleanup oldConn khỏi room.
		h.removeFromRoomLocked(oldConn)
	}
	h.connsByUser[c.UserID()] = c
	h.addToRoomLocked(c)
	h.mu.Unlock()

	// Kick conn cũ ở instance KHÁC nếu có.
	// Note: cũng publish kể cả khi đã kick local — vì user có thể có conn ở cả 2 instance.
	// Bus sẽ skip echo về chính mình.
	// if h.bus != nil {
	// 	go func(uid int32) {
	// 		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	// 		defer cancel()
	// 		if err := h.bus.PublishKickUser(ctx, uid); err != nil {
	// 			h.log.Warn("publish kick on register failed", "err", err)
	// 		}
	// 	}(c.UserID())
	// }
	h.enqueue(publishJob{
		kind:   publishKick,
		userID: c.UserID(),
	})

	h.log.Info("conn registered", "userID", c.UserID(), "mapID", c.MapID())
}

// unregister gọi khi conn close (read loop return).
func (h *Hub) unregister(c *Conn) {
	var needCleanup bool
	var mapID string

	h.mu.Lock()
	if existing, ok := h.connsByUser[c.UserID()]; ok && existing == c {
		delete(h.connsByUser, c.UserID())
	}
	h.removeFromRoomLocked(c)

	if c.MapID() != "" && h.manager != nil {
		needCleanup = true
		mapID = c.MapID()
	}
	h.mu.Unlock()

	// Cleanup ngoài lock — tránh nested lock
	if needCleanup {
		h.manager.RemovePlayerFromMap(mapID, c.UserID())
	}
}

// MoveToRoom đổi map của conn. Gọi khi NestJS bắn event setMap qua Redis pub/sub
// (sau này implement) — Phase 1 chưa cần.
func (h *Hub) MoveToRoom(c *Conn, newMap string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeFromRoomLocked(c)
	c.SetMapID(newMap)
	h.addToRoomLocked(c)
}

func (h *Hub) addToRoomLocked(c *Conn) {
	if c.MapID() == "" {
		return
	}
	room, ok := h.roomsByMap[c.MapID()]
	if !ok {
		room = make(map[*Conn]struct{})
		h.roomsByMap[c.MapID()] = room
	}
	room[c] = struct{}{}
}

func (h *Hub) removeFromRoomLocked(c *Conn) {
	if c.MapID() == "" {
		return
	}
	room, ok := h.roomsByMap[c.MapID()]
	if !ok {
		return
	}
	delete(room, c)
	if len(room) == 0 {
		delete(h.roomsByMap, c.MapID())
	}
}

// Stats trả về metric cho monitoring.
func (h *Hub) Stats() (totalConns int, totalRooms int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connsByUser), len(h.roomsByMap)
}

func (h *Hub) publishWorker() {
	for job := range h.publishCh {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond) // giảm từ 1s → 200ms
		start := time.Now()

		switch job.kind {
		case publishBroadcast:
			if err := h.bus.PublishBroadcast(ctx, job.mapID, job.data, job.excludeUserID); err != nil {
				h.log.Warn("publish broadcast failed", "err", err, "mapID", job.mapID, "elapsed", time.Since(start))
			}
		case publishKick:
			if err := h.bus.PublishKickUser(ctx, job.userID); err != nil {
				h.log.Warn("publish kick failed", "err", err, "userID", job.userID, "elapsed", time.Since(start))
			}
		}

		// Log nếu worker bị block quá lâu — đây là dấu hiệu NATS chậm/fail
		if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
			h.log.Warn("publish worker slow — NATS latency cao?", "elapsed", elapsed, "kind", job.kind)
		}

		cancel()
	}
}

func (h *Hub) enqueue(job publishJob) {
	if h.bus == nil {
		return
	}
	select {
	case h.publishCh <- job:
	default:
		h.log.Warn("publish channel full, dropping",
			"kind", job.kind,
			"mapID", job.mapID,
			"queueLen", len(h.publishCh),
		)
	}
}

func (h *Hub) Close() {
	if h.publishCh != nil {
		close(h.publishCh) // worker tự thoát khi channel closed
	}
}
