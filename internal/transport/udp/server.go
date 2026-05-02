package udp

// const (
// 	maxPacketSize = 2048 // PlayerMove ~150 bytes, buffer rộng để an toàn
// 	tokenSize     = 8
// )

// // UDPServer đọc PlayerMove từ client, update state.Manager.
// //
// // KHÔNG có Hub, KHÔNG có Conn — stateless per-packet.
// // Broadcast vẫn do WS Hub + Ticker lo — không thay đổi gì ở đó.
// //
// // Mỗi packet layout:
// //
// //	[token 8 bytes][msgType 1 byte][payload...]
// //
// // Token thay thế auth header — nhỏ, fixed-size, lookup O(1).
// type UDPServer struct {
// 	conn          *net.UDPConn
// 	sessions      *SessionStore
// 	manager       *state.Manager
// 	playerService *player.Service // giữ nguyên Redis update như WS path
// 	log           *slog.Logger
// }

// func NewUDPServer(
// 	addr string,
// 	sessions *SessionStore,
// 	manager *state.Manager,
// 	playerService *player.Service,
// 	log *slog.Logger,
// ) (*UDPServer, error) {
// 	udpAddr, err := net.ResolveUDPAddr("udp", addr)
// 	if err != nil {
// 		return nil, err
// 	}
// 	conn, err := net.ListenUDP("udp", udpAddr)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// Buffer OS 4MB — tránh kernel drop packet khi burst
// 	// Kernel buffer này tách biệt với Go heap — free
// 	conn.SetReadBuffer(4 * 1024 * 1024)

// 	return &UDPServer{
// 		conn:          conn,
// 		sessions:      sessions,
// 		manager:       manager,
// 		playerService: playerService,
// 		log:           log,
// 	}, nil
// }

// // Run là blocking loop — gọi trong goroutine riêng từ app.go.
// //
// // 1 goroutine đọc UDP (ReadFromUDP block) → dispatch sang goroutine xử lý.
// // Không dùng worker pool vì handlePacket rất nhanh (< 1ms, chỉ update in-memory).
// // Nếu sau này có Redis call trong hot path thì mới cần worker pool.
// func (s *UDPServer) Run() {
// 	buf := make([]byte, maxPacketSize)
// 	for {
// 		n, _, err := s.conn.ReadFromUDP(buf)
// 		if err != nil {
// 			s.log.Warn("udp read error", "err", err)
// 			continue
// 		}

// 		// Token + msgType tối thiểu
// 		if n < tokenSize+1 {
// 			continue
// 		}

// 		// Copy packet ra trước khi dispatch goroutine
// 		// vì buf sẽ bị overwrite ở iteration sau
// 		pkt := make([]byte, n)
// 		copy(pkt, buf[:n])
// 		go s.handlePacket(pkt)
// 	}
// }

// func (s *UDPServer) handlePacket(data []byte) {
// 	// Layout: [token 8 bytes][msgType 1 byte][payload...]
// 	var tok Token
// 	copy(tok[:], data[:tokenSize])

// 	sess, ok := s.sessions.Get(tok)
// 	if !ok {
// 		// Drop silently — token invalid hoặc session đã expire
// 		// Không log để tránh spam khi có packet lạc sau disconnect
// 		return
// 	}

// 	msgType := data[tokenSize]
// 	payload := data[tokenSize+1:]

// 	switch msgType {
// 	case protocol.MsgPlayerMove:
// 		s.handlePlayerMove(sess, payload)
// 	default:
// 		s.log.Warn("udp: unknown msgType", "type", msgType, "userID", sess.UserID)
// 	}
// }

// func (s *UDPServer) handlePlayerMove(sess *Session, payload []byte) {
// 	var m messages.PlayerMove
// 	if err := m.Decode(payload); err != nil {
// 		s.log.Warn("udp: decode PlayerMove failed",
// 			"err", err,
// 			"userID", sess.UserID,
// 		)
// 		return
// 	}

// 	// Update in-memory state — Ticker sẽ broadcast lên WS như cũ
// 	ms := s.manager.GetOrCreateMap(m.MapID)
// 	ms.UpdateFromMove(sess.UserID, &m)

// 	// Redis update — giữ nguyên pattern fire-and-forget như WS Handler
// 	// để NestJS cron vẫn đọc được dirty flag
// 	go func(userID int32, move messages.PlayerMove) {
// 		// Dùng context.Background() thay vì timeout ngắn vì goroutine tách biệt
// 		// Timeout 1s giống WS path
// 		import_ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
// 		defer cancel()
// 		if err := s.playerService.HandleMove(import_ctx, userID, &move); err != nil {
// 			s.log.Warn("udp: redis update failed", "err", err, "userID", userID)
// 		}
// 	}(sess.UserID, m)
// }

// func (s *UDPServer) Close() {
// 	s.conn.Close() // làm ReadFromUDP return error → Run() thoát
// }
