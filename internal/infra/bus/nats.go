package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// NATSBus là cross-instance message bus dùng NATS Pub/Sub.
//
// SO VỚI REDIS PUB/SUB:
//   - 1 connection cho cả pub và sub (NATS không có constraint subscribe block)
//   - Publish non-blocking (~µs) — không cần goroutine fire-and-forget
//   - Reconnect built-in robust hơn Redis go-client
//   - Code đơn giản hơn ~40% vì NATS tự manage subscriber goroutine
//
// MÔ HÌNH GIỐNG REDIS BUS:
//   - Local fan-out trước, publish cross-instance sau
//   - Echo prevention bằng nodeID UUID
//   - Binary encoding [16B nodeID][1B msgType][body]
//
// KHÁC BIỆT NHỎ:
//   - Subject dùng dấu chấm: gamebus.broadcast (NATS convention)
//     thay vì gamebus:broadcast (Redis style)
type NATSBus struct {
	log    *slog.Logger
	nodeID string

	// nc: 1 connection cho cả pub và sub.
	// Khác Redis: Redis bắt buộc 2 client riêng vì subscribe block connection.
	// NATS không có constraint này — 1 connection multiplex cả pub và sub được.
	nc *nats.Conn

	// subs: list subscription để cleanup khi Stop.
	// Redis bus không có field tương đương vì subscriber loop tự manage.
	subs []*nats.Subscription

	handler BusHandler

	// mu + started: mutex để Start/Stop idempotent.
	// Gọi Start 2 lần phải fail, Stop 2 lần phải no-op.
	mu      sync.Mutex
	started bool

	// === KHÔNG CÓ SO VỚI REDIS BUS ===
	//
	// Redis bus có:
	//   cancel context.CancelFunc  → signal subscriber goroutine exit
	//   wg     sync.WaitGroup      → đợi goroutine xong trước khi đóng client
	//
	// NATS bus KHÔNG cần 2 field này. Lý do:
	//
	// 1. SUBSCRIBER MODEL KHÁC HOÀN TOÀN
	//
	//    Redis: bạn TỰ code goroutine subscriber loop.
	//      pubsub := sub.Subscribe(ctx, channels...)
	//      go func() {
	//          for {
	//              select {
	//              case <-ctx.Done(): return
	//              case msg := <-pubsub.Channel(): handle(msg)
	//              }
	//          }
	//      }()
	//
	//    → Cần ctx.Done() để signal exit, cần wg để đợi goroutine.
	//
	//    NATS: NATS client TỰ manage goroutine bên trong.
	//      sub, _ := nc.Subscribe(subj, callback)
	//      // NATS đã spawn goroutine, tự loop, tự gọi callback
	//
	//    → Bạn không thấy goroutine. Để dừng: sub.Unsubscribe().
	//      Không cần ctx, không cần wg.
	//
	// 2. SO SÁNH TƯƠNG ĐỒNG
	//
	//    Tương tự DOM event listener trong JavaScript:
	//      button.addEventListener('click', callback)
	//      button.removeEventListener('click', callback)
	//
	//    Bạn không quản lý event loop của browser. Browser quản lý.
	//    Bạn chỉ pass callback và remove khi cần. NATS Go giống vậy.
	//
	// 3. CLEANUP DÙNG nc.Drain()
	//
	//    Redis cleanup: cancel() → wg.Wait() → pub.Close() → sub.Close() (4 bước)
	//    NATS cleanup:  Unsubscribe() → nc.Drain()                        (gộp tất cả)
	//
	//    nc.Drain() làm 4 việc trong 1 lệnh:
	//      a. Unsubscribe tất cả subscription
	//      b. Đợi callback in-flight xử lý xong (= wg.Wait() của Redis)
	//      c. Flush pending publish từ outbound buffer
	//      d. Đóng connection
}

// NATS subject names — dùng dấu chấm theo convention của NATS.
//
// Khác Redis dùng dấu : (gamebus:broadcast).
// NATS dùng . để hỗ trợ wildcard subscription:
//
//	nc.Subscribe("gamebus.>", ...)  → match mọi subject bắt đầu bằng "gamebus."
//	nc.Subscribe("gamebus.*", ...)  → match 1 token sau "gamebus."
//
// Hiện tại không dùng wildcard nhưng giữ format đúng convention NATS.
const (
	subjBroadcast = "gamebus.broadcast"
	subjKickUser  = "gamebus.kick"
)

// NewNATSBus khởi tạo bus với 1 NATS connection.
//
// Khác với Redis: NATS chỉ cần 1 connection cho cả pub/sub.
// Auto reconnect được config sẵn — robust hơn Redis go-client.
func NewNATSBus(natsURL string, log *slog.Logger) (*NATSBus, error) {
	nc, err := nats.Connect(natsURL,
		// === RECONNECT BEHAVIOR ===

		// MaxReconnects(-1): reconnect VÔ HẠN khi mất kết nối.
		//
		// Default của NATS Go client là 60 lần. Với ReconnectWait 2s → tổng 120s
		// rồi từ bỏ → connection chết vĩnh viễn → publish fail mãi → phải restart service.
		//
		// Với -1: NATS server crash bao lâu, sống lại lúc nào, service tự reconnect lúc đó.
		// Self-healing — không cần intervention.
		nats.MaxReconnects(-1),

		// ReconnectWait: thời gian đợi giữa các lần reconnect attempt.
		//
		// 2 giây = balance giữa:
		//   - Quá ngắn (100ms): spam reconnect, network noise, có thể overload NATS lúc recover
		//   - Quá dài (30s): NATS sống lại nhưng client mất 30s mới biết → unavailable lâu
		//
		// NATS thêm jitter mặc định (random offset) → 100 client cùng reconnect không đồng pha
		// → tránh "thundering herd" attack lên NATS server đang recover.
		nats.ReconnectWait(2*time.Second),

		// ReconnectBufSize: buffer publish trong KHI client đang disconnected.
		//
		// Không có equivalent ở Redis go-client.
		//
		// Cách hoạt động:
		//   T=0s:   NATS xuống
		//   T=0-3s: nc.Publish() vẫn được gọi → message lưu vào buffer 8MB này
		//   T=3s:   NATS lên lại, reconnect
		//   T=3.1s: Buffer flush — tất cả message tích lũy được gửi đi
		//
		// 8MB ≈ 55k message (~150 byte/msg) ≈ 1 giây buffer cho game 60Hz × 1000 user.
		// Buffer overflow → publish mới fail. Acceptable vì game realtime drop frame OK.
		nats.ReconnectBufSize(8*1024*1024),

		// === LIFECYCLE HOOKS ===
		//
		// 3 callback dưới đây là EVENT HANDLERS — NATS client TỰ gọi khi sự kiện xảy ra.
		// Bạn KHÔNG gọi trực tiếp. Tương tự DOM event listener trong JavaScript:
		//   - Bạn đăng ký callback (lúc Connect)
		//   - NATS client gọi callback (khi event xảy ra)
		//   - NATS truyền argument vào (connection, subscription, error)
		//
		// AI GỌI: NATS internal goroutine (Reconnect goroutine, Error dispatcher).

		// DisconnectErrHandler — gọi KHI mất kết nối với server.
		//
		// Khi nào trigger:
		//   - NATS server crash
		//   - Network gián đoạn (TCP RST, timeout)
		//   - Firewall close connection
		//
		// Argument truyền vào:
		//   - *nats.Conn: pointer tới client (dùng `_` vì không cần access)
		//   - error: lỗi gây ra disconnect (io.EOF, *net.OpError, ...)
		//
		// Sau callback này, NATS Reconnect goroutine bắt đầu loop retry connect.
		// Không cần code gì để trigger reconnect — NATS tự làm.
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn("nats disconnected", "err", err)
		}),

		// ReconnectHandler — gọi KHI reconnect THÀNH CÔNG sau disconnect.
		//
		// Khi nào trigger:
		//   - Sau DisconnectErrHandler, NATS loop retry. Khi 1 lần connect OK → trigger callback này.
		//
		// Argument truyền vào:
		//   - *nats.Conn: pointer tới client. Lần này dùng tên `c` để gọi method:
		//     - c.ConnectedUrl(): URL của NATS node hiện tại đã reconnect tới
		//       (quan trọng nếu setup cluster nhiều node — biết reconnect vào node nào)
		//     - c.Stats(): thống kê messages, reconnect count
		//
		// Sau callback này:
		//   - NATS tự động RESUBSCRIBE các subscription cũ (không cần code gì)
		//   - NATS tự động FLUSH pending publish từ buffer 8MB ra server
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info("nats reconnected", "url", c.ConnectedUrl())
		}),

		// ErrorHandler — gọi cho ASYNC ERRORS không thuộc disconnect/reconnect.
		//
		// Khi nào trigger:
		//   1. Slow consumer: callback của bạn xử lý chậm → pending queue đầy → NATS drop
		//      message và gọi callback này với err = nats.ErrSlowConsumer
		//   2. Permission denied (nếu bật auth)
		//   3. Server gửi error response
		//   4. Protocol error
		//
		// Argument truyền vào:
		//   - *nats.Conn: client (dùng `_`)
		//   - *nats.Subscription: subscription nào gây error.
		//     CÓ THỂ NIL nếu error là connection-level (không liên quan subscription cụ thể).
		//   - error: error cụ thể
		//
		// Tại sao check `if sub != nil`:
		//   Một số error là connection-level → sub = nil.
		//   Truy cập sub.Subject mà sub là nil → PANIC nil pointer dereference.
		//   Defensive programming: luôn check nil trước khi dereference pointer.
		//
		// Nếu KHÔNG set ErrorHandler:
		//   Async errors bị silent. Subscription không hoạt động mà không biết tại sao.
		//   Set hook → log ra để debug khi production có issue.
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			subj := ""
			if sub != nil {
				subj = sub.Subject
			}
			log.Warn("nats error", "err", err, "subject", subj)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	return &NATSBus{
		log:    log,
		nodeID: uuid.NewString(),
		nc:     nc,
	}, nil
}

func (b *NATSBus) SetHandler(h BusHandler) { b.handler = h }

// Start subscribe các subject. NATS tự manage subscriber goroutine bên trong —
// không cần tự code subscriber loop như Redis.
//
// SO SÁNH VỚI REDIS BUS:
//
//	Redis Start():
//	  1. Tạo subCtx, cancel = context.WithCancel(ctx)  ← Cần để signal exit
//	  2. pubsub := sub.Subscribe(...)
//	  3. wg.Add(1)
//	  4. Spawn goroutine với select { case <-ctx.Done(); case msg := <-ch }
//	  5. Goroutine tự loop đọc message và dispatch
//
//	NATS Start():
//	  1. nc.Subscribe(subj, callback)  ← NATS tự lo phần còn lại
//	  2. Lưu subscription để Stop dùng
//
// → Không cần context cancel, không cần WaitGroup. NATS quản lý goroutine bên trong.
// Tương tự pattern addEventListener trong JavaScript: pass callback, runtime tự xử lý.
func (b *NATSBus) Start(ctx context.Context) error {
	// _ : ctx bị ignore hoàn toàn.
	// NATS không cần ctx để signal exit — dùng sub.Unsubscribe() trong Stop().

	// Redis Bus không có lock này — Start() không idempotent.
	// NATS thêm lock để gọi Start() 2 lần không tạo duplicate subscription.
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return errors.New("bus already started")
	}
	if b.handler == nil {
		return errors.New("handler not set, call SetHandler first")
	}

	// Subscribe — callback sẽ chạy trong goroutine riêng do NATS spawn nội bộ.
	//
	// Mỗi subscription có internal pending queue (default 65536 message).
	// Nếu callback xử lý chậm, message tiếp theo tích trong queue này.
	// Khi queue đầy → ErrorHandler được gọi với nats.ErrSlowConsumer.
	// Tạo slice rỗng, pre-allocate cho 2 phần tử.
	// Lưu subscription để Stop() có thể Unsubscribe từng cái.
	subs := make([]*nats.Subscription, 0, 2)

	subBroadcast, err := b.nc.Subscribe(subjBroadcast, func(m *nats.Msg) {
		// nc.Subscribe(subject, callback):
		//   - subject: tên channel NATS (dùng dấu chấm theo convention)
		//   - callback: function được gọi mỗi khi có message
		// NATS nội bộ spawn goroutine, tự loop đọc TCP, tự gọi callback.
		// Không thấy goroutine ở đây — NATS giấu đi.
		b.dispatch(m.Data)
		// m.Data là []byte — không cần cast như Redis (msg.Payload là string).
	})
	if err != nil {
		return fmt.Errorf("subscribe broadcast: %w", err)
	}
	// Cần gán lại nếu realloc (vượt quá pre-allocate hiện tại), case này k cần nhưng làm cho chắc
	subs = append(subs, subBroadcast)

	subKick, err := b.nc.Subscribe(subjKickUser, func(m *nats.Msg) {
		b.dispatch(m.Data)
	})
	if err != nil {
		// Subscribe broadcast OK nhưng subscribe kick fail → cleanup partial state.
		// Tránh để 1 subscription dangling khi return error.
		subBroadcast.Unsubscribe()
		return fmt.Errorf("subscribe kick: %w", err)
	}
	// Cần gán lại nếu realloc (vượt quá pre-allocate hiện tại), case này k cần nhưng làm cho chắc
	subs = append(subs, subKick)

	b.subs = subs
	b.started = true
	b.log.Info("nats bus started", "nodeID", b.nodeID)
	return nil
}

// Stop graceful shutdown.
//
// nc.Drain() là magic của NATS — 1 lệnh làm 4 việc:
//  1. Unsubscribe tất cả subscription còn lại
//  2. Đợi callback in-flight xử lý xong (= wg.Wait() của Redis)
//  3. Flush pending publish từ outbound buffer ra server
//  4. Đóng connection
//
// SO VỚI REDIS BUS:
//
//	Redis: cancel() → wg.Wait() → pub.Close() → sub.Close()  (4 bước)
//	NATS:  Unsubscribe(loop) → nc.Drain()                     (gộp tất cả)
//
// API đẹp hơn. Nội bộ NATS làm cùng việc nhưng abstract đi.
func (b *NATSBus) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.started {
		return
	}

	// Unsubscribe trước khi Drain — explicit cho rõ intent.
	// Drain cũng tự unsubscribe nhưng ghi tường minh dễ đọc.
	for _, s := range b.subs {
		s.Unsubscribe()
	}

	// Drain block đến khi:
	//   - Pending message in-flight xử lý xong
	//   - Outbound buffer flush hết
	//   - Connection đóng
	//
	// Tương đương `cancel + wg.Wait + Close` của Redis bus, gộp vào 1 lệnh.
	if err := b.nc.Drain(); err != nil {
		b.log.Warn("nats drain error", "err", err)
	}
	b.started = false
	b.log.Info("nats bus stopped")
}

func (b *NATSBus) NodeID() string { return b.nodeID }

// dispatch — copy nguyên xi từ Redis Bus, format payload không đổi.
//
// Được gọi từ subscribe callback. Mỗi callback chạy trong goroutine NATS riêng.
// Không cần lock vì:
//   - NATS đảm bảo 1 subscription chỉ có 1 goroutine xử lý tuần tự
//   - dispatch chỉ đọc state (b.handler, b.nodeID) — không modify
func (b *NATSBus) dispatch(payload []byte) {
	if len(payload) < 17 {
		b.log.Warn("bus payload too short", "len", len(payload))
		return
	}

	originNode := string(payload[:16])
	if originNode == b.nodeIDBytes() {
		// Echo prevention: skip message do chính instance này publish.
		return
	}

	msgType := payload[16]
	body := payload[17:]

	switch msgType {
	case msgTypeBroadcast:
		mapID, data, excludeUserID, err := decodeBroadcast(body)
		if err != nil {
			b.log.Warn("decode broadcast failed", "err", err)
			return
		}
		b.handler.OnBroadcast(mapID, data, excludeUserID)

	case msgTypeKick:
		userID, err := decodeKick(body)
		if err != nil {
			b.log.Warn("decode kick failed", "err", err)
			return
		}
		b.handler.OnKickUser(userID)

	default:
		b.log.Warn("unknown msg type", "type", msgType)
	}
}

func (b *NATSBus) nodeIDBytes() string {
	id, _ := uuid.Parse(b.nodeID)
	return string(id[:])
}

// === PUBLISH METHODS ===
//
// Khác Redis Bus: NATS publish non-blocking (~µs).
// Chỉ ghi vào local buffer của NATS client, background goroutine flush sang server.
// → Hub gọi trực tiếp được, không cần goroutine fire-and-forget như Redis.
//
// Hệ quả: vấn đề goroutine pile-up của Redis Bus tự động biến mất với NATS.

// PublishBroadcast — context bị ignore vì NATS publish không round-trip.
// Giữ signature có ctx để Hub không phải đổi gì khi switch broker.
func (b *NATSBus) PublishBroadcast(ctx context.Context, mapID string, data []byte, excludeUserID int32) error {
	body := encodeBroadcast(mapID, data, excludeUserID)
	return b.publish(subjBroadcast, msgTypeBroadcast, body)
}

func (b *NATSBus) PublishKickUser(ctx context.Context, userID int32) error {
	body := encodeKick(userID)
	return b.publish(subjKickUser, msgTypeKick, body)
}

func (b *NATSBus) publish(subject string, msgType byte, body []byte) error {
	payload := make([]byte, 0, 17+len(body))
	payload = append(payload, []byte(b.nodeIDBytes())...)
	payload = append(payload, msgType)
	payload = append(payload, body...)
	return b.nc.Publish(subject, payload)
}

// Start() chỉ subscribe INTERNAL subjects — các subject dùng cho
// cross-instance communication giữa các Go instance với nhau.
//
// EXTERNAL subjects (từ service khác như NestJS) KHÔNG subscribe ở đây vì:
//   - Bus không biết logic xử lý của từng subject
//   - Coupling Bus với Hub/Manager là sai trách nhiệm
//   - Caller tự subscribe qua method riêng (vd: SubscribePlayerDisconnect)
//
// Khi thêm subject mới:
//   - Go→Go cross-instance → thêm vào Start() + dispatch()
//   - NestJS→Go           → thêm method Subscribe* mới vào interface
func (b *NATSBus) SubscribePlayerDisconnect(handler func(userID int32, mapID string)) error {
	sub, err := b.nc.Subscribe("player.disconnected", func(m *nats.Msg) {
		var payload struct {
			UserID int32  `json:"userId"`
			Map    string `json:"map"`
		}
		if err := json.Unmarshal(m.Data, &payload); err != nil {
			b.log.Warn("decode player.disconnected failed", "err", err)
			return
		}
		handler(payload.UserID, payload.Map)
	})
	if err != nil {
		return fmt.Errorf("subscribe player.disconnected: %w", err)
	}
	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()
	return nil
}

// Encode/decode functions — KHÔNG CẦN copy lại.
// Đã có sẵn trong bus.go (Redis version), package-level function dùng chung được.

// ┌─────────────────────────────────────────────┐
// │         NATS Go Client (nats.Conn)          │
// │                                             │
// │  ┌─────────────────────────────┐            │
// │  │ Reader goroutine            │            │
// │  │ - đọc bytes từ TCP socket   │            │
// │  │ - parse NATS protocol       │            │
// │  │ - dispatch tới sub callback │            │
// │  └─────────────────────────────┘            │
// │                                             │
// │  ┌─────────────────────────────┐            │
// │  │ Writer goroutine            │            │
// │  │ - đọc từ outbound buffer    │            │
// │  │ - ghi xuống TCP socket      │            │
// │  └─────────────────────────────┘            │
// │                                             │
// │  ┌─────────────────────────────┐            │
// │  │ Reconnect goroutine         │            │
// │  │ - khi mất kết nối:          │            │
// │  │   1. Gọi DisconnectErrHandler  ← HERE    │
// │  │   2. Loop retry connect     │            │
// │  │   3. Khi kết nối lại được:  │            │
// │  │      → Gọi ReconnectHandler    ← HERE    │
// │  └─────────────────────────────┘            │
// │                                             │
// │  ┌─────────────────────────────┐            │
// │  │ Error dispatcher            │            │
// │  │ - nhận error từ các nơi     │            │
// │  │ - gọi ErrorHandler          │  ← HERE    │
// │  └─────────────────────────────┘            │
// │                                             │
// └─────────────────────────────────────────────┘
