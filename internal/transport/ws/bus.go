package ws

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	redisclient "github.com/DANG-PH/game-service-go/internal/infra/redis"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Bus là cross-instance message bus dùng Redis Pub/Sub.
//
// MÔ HÌNH:
//
//	Instance A: BroadcastToMap("MAP:1", packet)
//	  1. Local fan-out: gửi tới conn local trong MAP:1 trên A
//	  2. Publish lên Redis channel "gamebus:broadcast" với payload chứa mapID + packet + originNode=A
//	Instance B (subscriber):
//	  1. Nhận message, decode
//	  2. Nếu originNode == B → skip (đã local broadcast rồi)
//	  3. Nếu originNode != B → local broadcast tới conn trong MAP:1 trên B
//
// TẠI SAO 1 CHANNEL CHUNG (gamebus:broadcast) THAY VÌ 1 CHANNEL/MAP?
// - Đơn giản: không cần dynamic SUBSCRIBE/UNSUBSCRIBE khi user đổi map
// - Đủ tốt cho < ~50 instance. Nếu nhiều hơn → shard theo mapID hash.
// - Filter ở instance side rẻ hơn nhiều so với network round-trip subscribe.
//
// TẠI SAO 2 REDIS CLIENT?
// go-redis: 1 connection vào subscribe mode thì block, không publish được nữa.
// Giống ioredis ở NestJS: pubClient.duplicate() → subClient.

type Bus struct {
	log    *slog.Logger
	nodeID string

	pub *redis.Client
	sub *redis.Client

	// handler được gọi khi nhận message từ instance khác.
	// Set sau NewBus() qua SetHandler — vì Hub cần reference Bus để publish,
	// và Bus cần reference Hub để dispatch → tránh circular bằng pattern 2 phase:
	// 1) NewBus() tạo bus với handler nil
	// 2) NewHub(bus) tạo hub, gọi bus.SetHandler(hub)
	handler BusHandler

	// Lifecycle
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Channel names. Để 1 prefix chung dễ debug bằng `redis-cli PSUBSCRIBE gamebus:*`.
const (
	chanBroadcast = "gamebus:broadcast"
	chanKickUser  = "gamebus:kick"
)

// Message types — byte đầu của payload.
const (
	msgTypeBroadcast byte = 1
	msgTypeKick      byte = 2
)

// NewBus khởi tạo bus với 2 Redis client riêng.
// pubURL và subURL thường giống nhau — vẫn cần parse 2 lần để có 2 connection độc lập.
//
// Gọi 1 lần duy nhất ở app.go::New(), trước khi tạo Hub.
// Sinh nodeID UUID mới — mỗi instance Go phải có nodeID khác nhau để echo prevention hoạt động.
func NewBus(redisURL string, log *slog.Logger) (*Bus, error) {
	pub, err := redisclient.New(redisURL)
	if err != nil {
		return nil, fmt.Errorf("create pub client: %w", err)
	}
	sub, err := redisclient.New(redisURL)
	if err != nil {
		pub.Close()
		return nil, fmt.Errorf("create sub client: %w", err)
	}

	return &Bus{
		log:    log,
		nodeID: uuid.NewString(),
		pub:    pub,
		sub:    sub,
	}, nil
}

// SetHandler gắn handler. Hub gọi sau NewBus().
// Chỉ nên gọi 1 lần — không có lock vì khi đã Start() thì handler không nên đổi.
//
// Gọi từ hub.go::NewHub() — Hub tự register chính nó làm handler (bus.SetHandler(h)).
// Pattern 2-phase construction này tránh circular dependency:
// Bus cần Hub callback, Hub cần Bus để publish → tách thành 2 bước tạo.
func (b *Bus) SetHandler(h BusHandler) {
	b.handler = h
}

// Start chạy subscriber loop. Gọi 1 lần khi service start.
//
// Gọi từ app.go::Run(), TRƯỚC server.ListenAndServe().
// Nếu start sau khi accept connection → broadcast từ instance khác bị miss
// trong khoảng thời gian đó.
//
// Subscriber loop chạy trong goroutine riêng, đọc từ pubsub.Channel()
// và gọi b.dispatch() cho mỗi message nhận được.
func (b *Bus) Start(ctx context.Context) error {
	if b.handler == nil {
		return errors.New("handler not set, call SetHandler first")
	}

	// KHÔNG có subCtx và cancel: Stop() không có cách signal goroutine dưới exit
	// → wg.Wait() block mãi → app không shutdown được.
	//
	// Có vài cách khác để signal goroutine exit, ngoài context:
	//   Cách 1: Close channel data — dựa vào go-redis tự close `ch` khi context cancel,
	//           rồi for...range tự exit. Hoạt động được nhưng phụ thuộc behavior thư viện,
	//           version mới đổi là break.
	//   Cách 2: Done channel riêng — tự tạo `done := make(chan struct{})`, select case <-done
	//           và Stop gọi close(done). OK cho code đơn giản, nhưng nhiều goroutine phải
	//           share cùng done channel sẽ phức tạp hơn context tree.
	//   Cách 3 (đang dùng): context chuẩn Go — pattern phổ biến nhất, propagate cancel
	//           xuống context con, mọi thư viện Go đều support.
	subCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel

	pubsub := b.sub.Subscribe(subCtx, chanBroadcast, chanKickUser)

	// Đợi subscribe confirm — fail fast nếu Redis lỗi.
	if _, err := pubsub.Receive(subCtx); err != nil {
		pubsub.Close()
		return fmt.Errorf("redis subscribe: %w", err)
	}

	b.log.Info("redis bus subscriber started", "nodeID", b.nodeID)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		// Nếu chỉ đóng b.sub, connection bị cắt đột ngột — Redis vẫn nghĩ client đang subscribe đến khi timeout. Không sạch.
		// Có thể lược bỏ không?
		// Về mặt thực tế, nếu app exit ngay sau Stop() thì có thể bỏ pubsub.Close(), vì OS sẽ cleanup hết. Nhưng:
		// Nếu app không exit (ví dụ Stop+Start lại) → pubsub không close → leak
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case <-subCtx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				b.dispatch([]byte(msg.Payload))
			}
		}
	}()

	return nil
}

// Stop cleanup. Đợi subscriber loop exit.
//
// Gọi từ app.go::Shutdown(), SAU server.Shutdown().
// Thứ tự quan trọng: nếu Stop bus trước, các conn còn lại broadcast sẽ fail publish → log noise.
//
// Cancel context → subscriber loop nhận signal exit qua subCtx.Done().
// wg.Wait() đảm bảo goroutine đã thực sự kết thúc trước khi đóng client.
func (b *Bus) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.pub.Close()
	b.sub.Close()
}

// NodeID expose cho debug/log.
//
// Được dùng ở app.go healthcheck endpoint — debug load balancer routing
// (curl /health nhiều lần thấy nodeID khác nhau = LB đang round-robin đúng).
func (b *Bus) NodeID() string { return b.nodeID }

// dispatch parse payload, skip nếu origin chính mình, gọi handler.
//
// Gọi từ subscriber loop trong Start(), mỗi lần Redis push 1 message.
// Đây là entry point của INBOUND path — message từ instance khác đi vào đây
// và route tới đúng handler method (OnBroadcast/OnSendToUser/OnKickUser).
func (b *Bus) dispatch(payload []byte) {
	// Format chung: [16 byte originNode UUID][1 byte msgType][...body]
	if len(payload) < 17 {
		b.log.Warn("bus payload too short", "len", len(payload))
		return
	}

	originNode := string(payload[:16]) // raw bytes UUID, không cần parse string
	if originNode == b.nodeIDBytes() {
		// Echo từ chính mình → skip.
		// Vì mọi instance subscribe cùng channel, instance publish cũng nhận lại
		// message của chính mình. Đã local broadcast trong Hub rồi nên skip.
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

// nodeIDBytes trả về nodeID dưới dạng 16 byte raw để so sánh nhanh.
// Cache lại lần đầu — uuid.NewString() đã encode dưới dạng string,
// nhưng ta muốn binary 16 byte cho gọn payload.
// Implementation đơn giản: dùng uuid.Parse() lấy [16]byte, convert sang string.
//
// Dùng ở 2 chỗ:
//  1. dispatch() — compare với originNode 16-byte trong payload header để skip echo
//  2. publish() — embed vào header payload khi gửi đi
func (b *Bus) nodeIDBytes() string {
	id, _ := uuid.Parse(b.nodeID) // Đưa về dạng 16 byte [16]byte
	return string(id[:])          // Convert sang slice []byte
}

// === PUBLISH METHODS — Hub gọi vào ===
// Đây là entry point của OUTBOUND path — Hub gọi để gửi message tới instance khác.

// PublishBroadcast gửi broadcast cross-instance.
// excludeUserID = 0 nghĩa là không exclude ai.
//
// Gọi từ hub.go::BroadcastToMap(), sau khi đã local fan-out xong.
// Hot path: được gọi mỗi lần có move/action trong game (20-60Hz × user count).
func (b *Bus) PublishBroadcast(ctx context.Context, mapID string, data []byte, excludeUserID int32) error {
	body := encodeBroadcast(mapID, data, excludeUserID)
	return b.publish(ctx, chanBroadcast, msgTypeBroadcast, body)
}

// PublishKickUser publish kick command để instance nào có user đó sẽ kick.
//
// Gọi từ hub.go ở 2 chỗ:
//  1. register() — khi user connect mới, kick conn cũ ở instance khác
//  2. KickUser() — admin/system kick, không biết user đang ở instance nào
func (b *Bus) PublishKickUser(ctx context.Context, userID int32) error {
	body := encodeKick(userID)
	return b.publish(ctx, chanKickUser, msgTypeKick, body)
}

// publish helper internal — build payload với header nodeID + msgType, gọi pub.Publish.
//
// Được gọi bởi 3 method Publish* ở trên (DRY).
// Format: [16 byte nodeID][1 byte msgType][body]
func (b *Bus) publish(ctx context.Context, channel string, msgType byte, body []byte) error {
	// Format: [16 byte nodeID][1 byte msgType][body]
	payload := make([]byte, 0, 17+len(body))
	payload = append(payload, []byte(b.nodeIDBytes())...)
	payload = append(payload, msgType)
	payload = append(payload, body...)
	return b.pub.Publish(ctx, channel, payload).Err()
}

// === ENCODING/DECODING ===
// Format được thiết kế cho binary efficiency, không dùng JSON.
//
// Đây là package-level function (không method của Bus) vì stateless —
// chỉ thao tác bytes, không cần state của Bus.
//
// encode* được dùng ở: PublishBroadcast/PublishSendToUser/PublishKickUser (outbound)
// decode* được dùng ở: dispatch() (inbound)

// encodeBroadcast serialize broadcast body sang bytes.
// Body format: [4 byte excludeUserID int32][2 byte mapIDLen][mapID][...data]
//
// Dùng big-endian (network byte order) — convention chuẩn cho protocol.
func encodeBroadcast(mapID string, data []byte, excludeUserID int32) []byte {
	mapBytes := []byte(mapID)
	buf := make([]byte, 4+2+len(mapBytes)+len(data))
	binary.BigEndian.PutUint32(buf[0:4], uint32(excludeUserID))
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(mapBytes)))
	copy(buf[6:6+len(mapBytes)], mapBytes)
	copy(buf[6+len(mapBytes):], data)
	return buf
}

// decodeBroadcast parse bytes về fields. Inverse của encodeBroadcast.
// Validate length để tránh panic khi payload bị corrupt.
func decodeBroadcast(body []byte) (mapID string, data []byte, excludeUserID int32, err error) {
	if len(body) < 6 {
		return "", nil, 0, errors.New("broadcast body too short")
	}
	excludeUserID = int32(binary.BigEndian.Uint32(body[0:4]))
	mapLen := binary.BigEndian.Uint16(body[4:6])
	if len(body) < int(6+mapLen) {
		return "", nil, 0, errors.New("broadcast body truncated")
	}
	mapID = string(body[6 : 6+mapLen])
	data = body[6+mapLen:]
	return
}

// encodeKick serialize kick body — chỉ cần userID, không có data đi kèm.
// Body format: [4 byte userID]
func encodeKick(userID int32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(userID))
	return buf
}

// decodeKick parse bytes về userID. Inverse của encodeKick.
func decodeKick(body []byte) (int32, error) {
	if len(body) < 4 {
		return 0, errors.New("kick body too short")
	}
	return int32(binary.BigEndian.Uint32(body[0:4])), nil
}
