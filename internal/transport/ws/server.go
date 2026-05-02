package ws

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DANG-PH/game-service-go/internal/auth"
	"github.com/DANG-PH/game-service-go/internal/shared/messages"
	"github.com/DANG-PH/game-service-go/internal/shared/protocol"
)

// Server là HTTP handler upgrade WebSocket và xử lý handshake.
// Sau handshake thành công, conn được register vào Hub và 2 goroutine read/write chạy.
type Server struct {
	log      *slog.Logger
	upgrader websocket.Upgrader
	hub      *Hub
	auth     *auth.Authenticator
	handler  *Handler
}

func NewServer(log *slog.Logger, hub *Hub, auth *auth.Authenticator, handler *Handler) *Server {
	return &Server{
		log: log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// CheckOrigin: cho phép mọi origin vì client là LibGDX (không phải browser).
			// Browser sẽ enforce same-origin từ phía nó. Game client native không.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		hub:     hub,
		auth:    auth,
		handler: handler,
	}
}

// ServeHTTP implement http.Handler. (Duck Typing)
// Đăng ký vào router: mux.Handle("/ws-game", server)
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Warn("upgrade failed", "err", err)
		return
	}

	// Disable Nagle — giảm latency thực tế
	if tc, ok := wsConn.UnderlyingConn().(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}

	// Wrap với Conn struct của mình.
	conn := NewConn(wsConn, s.hub, s.log)

	// Handshake phải hoàn thành trong 5 giây — chống slowloris attack.
	if err := s.doHandshake(conn); err != nil {
		s.log.Warn("handshake failed", "err", err, "remote", r.RemoteAddr)
		conn.Close()
		return
	}

	// Register conn vào hub.
	s.hub.register(conn)

	// Spawn 2 goroutine: read và write.
	// writeLoop chạy trước để sẵn sàng cho read dùng (nếu read chạy trc có case cần write thì sai)
	go conn.writeLoop()
	conn.readLoop(s.handler) // chạy trong goroutine của ServeHTTP — block tới khi conn close.
}

// doHandshake đọc 1 packet đầu tiên, verify, reply Ack/Nack.
//
// Tại sao có handshake riêng thay vì đặt JWT trong query string?
//  1. Query string log lại trong access log → leak token.
//  2. Handshake binary cho phép gửi nhiều thông tin (version, sessionId) mà không
//     bloated URL.
//  3. Pattern này dễ extend (vd handshake response chứa server time, snapshot, ...).
func (s *Server) doHandshake(c *Conn) error {
	// Đặt deadline cho handshake.
	c.ws.SetReadDeadline(time.Now().Add(5 * time.Second))

	msgType, data, err := c.ws.ReadMessage()
	if err != nil {
		return err
	}

	if msgType != websocket.BinaryMessage || len(data) < 1 || data[0] != protocol.MsgHandshake {
		s.sendNack(c, protocol.NackReasonInternal)
		return websocket.ErrBadHandshake
	}

	var hs messages.Handshake
	if err := hs.Decode(data[1:]); err != nil {
		s.sendNack(c, protocol.NackReasonInternal)
		return err
	}

	// Check version.
	if hs.ProtocolVersion != protocol.PROTOCOL_VERSION {
		s.sendNack(c, protocol.NackReasonVersion)
		s.log.Info("version mismatch", "client", hs.ProtocolVersion, "server", protocol.PROTOCOL_VERSION)
		return websocket.ErrBadHandshake
	}

	// Verify auth.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	authResult, err := s.auth.Verify(ctx, hs.Token, hs.GameSessionID, hs.UserID)
	if err != nil {
		switch err {
		case auth.ErrInvalidToken, auth.ErrUserIDMismatch:
			s.sendNack(c, protocol.NackReasonAuth)
		case auth.ErrInvalidSession:
			s.sendNack(c, protocol.NackReasonSession)
		default:
			s.sendNack(c, protocol.NackReasonInternal)
		}
		return err
	}

	c.SetUserID(authResult.UserID)

	// Reply Ack.
	c.ws.SetWriteDeadline(time.Now().Add(time.Second))
	if err := c.ws.WriteMessage(websocket.BinaryMessage, messages.EncodeHandshakeAck()); err != nil {
		return err
	}

	// Reset deadline cho readLoop.
	c.ws.SetReadDeadline(time.Time{})

	return nil
}

func (s *Server) sendNack(c *Conn, reason uint8) {
	c.ws.SetWriteDeadline(time.Now().Add(time.Second))
	c.ws.WriteMessage(websocket.BinaryMessage, messages.EncodeHandshakeNack(reason))
}
