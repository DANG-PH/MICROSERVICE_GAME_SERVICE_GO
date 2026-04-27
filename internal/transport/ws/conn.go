package ws

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Conn wrap websocket.Conn với:
// - userID, mapID gắn sau khi handshake
// - send channel buffered để write không block read goroutine
// - read/write goroutine riêng (pattern chuẩn của gorilla)
//
// TẠI SAO 2 GOROUTINE?
// Gorilla websocket KHÔNG thread-safe — concurrent write từ 2 goroutine = panic.
// Pattern: 1 goroutine read (gọi conn.ReadMessage), 1 goroutine write (gọi conn.WriteMessage).
// Goroutine khác muốn gửi → push vào send channel, write goroutine pick lên.
type Conn struct {
	ws  *websocket.Conn
	hub *Hub
	log *slog.Logger

	// Set sau khi handshake thành công
	userID int32
	mapID  string

	// Buffered channel cho outbound messages
	// Buffer 256 — đủ cho burst, đầy thì close conn (slow client = kick).
	send chan []byte

	// Đảm bảo close chỉ chạy 1 lần.
	closeOnce sync.Once
}

const (
	// Time để write 1 message tới peer.
	writeWait = 10 * time.Second

	// Time để đọc pong từ peer. Phải lớn hơn pingPeriod.
	pongWait = 60 * time.Second

	// Gửi ping mỗi pingPeriod giây. Phải nhỏ hơn pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Max size 1 message client gửi lên. 4KB đủ cho mọi packet game.
	maxMessageSize = 4096
)

func NewConn(ws *websocket.Conn, hub *Hub, log *slog.Logger) *Conn {
	return &Conn{
		ws:   ws,
		hub:  hub,
		log:  log,
		send: make(chan []byte, 256),
	}
}

// Send push message vào send channel.
// Non-blocking: nếu channel đầy → close conn (slow client).
// Đây là backpressure mechanism quan trọng — không để slow client làm chậm broadcast tới mọi người.
func (c *Conn) Send(data []byte) {
	select {
	case c.send <- data:
	default:
		// Channel đầy → client quá chậm, kick.
		c.log.Warn("send channel full, closing conn", "userID", c.userID)
		c.Close()
	}
}

// Close đóng conn an toàn (idempotent).
func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		close(c.send)
		c.ws.Close()
	})
}

// readLoop chạy trong goroutine riêng.
// Đọc message từ WebSocket, gọi handler để xử lý.
// Khi có lỗi (client disconnect, bad packet) → return → defer close conn.
func (c *Conn) readLoop(handler MessageHandler) {
	defer func() {
		c.hub.unregister(c)
		c.Close()
	}()

	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	// Khi nhận pong từ client → reset read deadline.
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		messageType, data, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Warn("websocket read error", "err", err, "userID", c.userID)
			}
			return
		}

		// Game protocol chỉ dùng binary messages.
		if messageType != websocket.BinaryMessage {
			c.log.Warn("non-binary message received, ignoring", "type", messageType)
			continue
		}

		if len(data) == 0 {
			continue
		}

		// Delegate cho handler. Handler chịu trách nhiệm route theo msgType byte đầu.
		handler.Handle(c, data)
	}
}

// writeLoop chạy trong goroutine riêng.
// Đọc message từ send channel, ghi xuống WebSocket.
// Cũng gửi ping định kỳ để giữ connection alive.
func (c *Conn) writeLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// send channel closed → conn đang close.
				c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.ws.WriteMessage(websocket.BinaryMessage, message); err != nil {
				c.log.Warn("websocket write error", "err", err, "userID", c.userID)
				return
			}

		case <-ticker.C:
			// Ping định kỳ. Nếu write fail → conn dead → return → close.
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// MessageHandler interface cho phép swap implementation (test, real).
type MessageHandler interface {
	Handle(conn *Conn, data []byte)
}

// UserID getter — đọc-only sau handshake.
func (c *Conn) UserID() int32 { return c.userID }
func (c *Conn) MapID() string { return c.mapID }

// Setters - chỉ gọi 1 lần lúc handshake.
func (c *Conn) SetUserID(id int32) { c.userID = id }
func (c *Conn) SetMapID(m string)  { c.mapID = m }
