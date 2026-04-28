package ws

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

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

	// Callback gọi khi nhận message từ instance khác.
	// Hub set callback này sau khi NewBus().
	onBroadcast  func(mapID string, data []byte, excludeUserID int32)
	onSendToUser func(userID int32, data []byte)
	onKickUser   func(userID int32)

	// Lifecycle
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Channel names. Để 1 prefix chung dễ debug bằng `redis-cli PSUBSCRIBE gamebus:*`.
const (
	chanBroadcast  = "gamebus:broadcast"
	chanSendToUser = "gamebus:send"
	chanKickUser   = "gamebus:kick"
)

// Message types — byte đầu của payload.
const (
	msgTypeBroadcast byte = 1
	msgTypeSend      byte = 2
	msgTypeKick      byte = 3
)

// NewBus khởi tạo bus với 2 Redis client riêng.
// pubURL và subURL thường giống nhau — vẫn cần parse 2 lần để có 2 connection độc lập.
func NewBus(redisURL string, log *slog.Logger) (*Bus, error) {
	pub, err := newRedisClient(redisURL)
	if err != nil {
		return nil, fmt.Errorf("create pub client: %w", err)
	}
	sub, err := newRedisClient(redisURL)
	if err != nil {
		pub.Close()
		return nil, fmt.Errorf("create sub client: %w", err)
	}

	return &Bus{
		log:    log,
		nodeID: uuid.NewString(), // unique mỗi instance
		pub:    pub,
		sub:    sub,
	}, nil
}

func newRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// SetHandlers gắn callback. Hub gọi sau NewBus().
func (b *Bus) SetHandlers(
	onBroadcast func(mapID string, data []byte, excludeUserID int32),
	onSendToUser func(userID int32, data []byte),
	onKickUser func(userID int32),
) {
	b.onBroadcast = onBroadcast
	b.onSendToUser = onSendToUser
	b.onKickUser = onKickUser
}

// Start chạy subscriber loop. Gọi 1 lần khi service start.
func (b *Bus) Start(ctx context.Context) error {
	if b.onBroadcast == nil || b.onSendToUser == nil || b.onKickUser == nil {
		return errors.New("handlers not set, call SetHandlers first")
	}

	subCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel

	pubsub := b.sub.Subscribe(subCtx, chanBroadcast, chanSendToUser, chanKickUser)

	// Đợi subscribe confirm — fail fast nếu Redis lỗi.
	if _, err := pubsub.Receive(subCtx); err != nil {
		pubsub.Close()
		return fmt.Errorf("redis subscribe: %w", err)
	}

	b.log.Info("redis bus subscriber started", "nodeID", b.nodeID)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
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
				b.dispatch(msg.Channel, []byte(msg.Payload))
			}
		}
	}()

	return nil
}

// Stop cleanup. Đợi subscriber loop exit.
func (b *Bus) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.pub.Close()
	b.sub.Close()
}

// NodeID expose cho debug/log.
func (b *Bus) NodeID() string { return b.nodeID }

// dispatch parse payload, skip nếu origin chính mình, gọi callback Hub.
func (b *Bus) dispatch(channel string, payload []byte) {
	// Format chung: [16 byte originNode UUID][1 byte msgType][...body]
	if len(payload) < 17 {
		b.log.Warn("bus payload too short", "len", len(payload))
		return
	}

	originNode := string(payload[:16]) // raw bytes UUID, không cần parse string
	if originNode == b.nodeIDBytes() {
		// Echo từ chính mình → skip.
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
		b.onBroadcast(mapID, data, excludeUserID)

	case msgTypeSend:
		userID, data, err := decodeSendToUser(body)
		if err != nil {
			b.log.Warn("decode sendToUser failed", "err", err)
			return
		}
		b.onSendToUser(userID, data)

	case msgTypeKick:
		userID, err := decodeKick(body)
		if err != nil {
			b.log.Warn("decode kick failed", "err", err)
			return
		}
		b.onKickUser(userID)

	default:
		b.log.Warn("unknown msg type", "type", msgType)
	}
}

// nodeIDBytes trả về nodeID dưới dạng 16 byte raw để so sánh nhanh.
// Cache lại lần đầu — uuid.NewString() đã encode dưới dạng string,
// nhưng ta muốn binary 16 byte cho gọn payload.
// Implementation đơn giản: dùng uuid.Parse() lấy [16]byte, convert sang string.
func (b *Bus) nodeIDBytes() string {
	id, _ := uuid.Parse(b.nodeID)
	return string(id[:])
}

// === PUBLISH METHODS — Hub gọi vào ===

// PublishBroadcast gửi broadcast cross-instance.
// excludeUserID = 0 nghĩa là không exclude ai.
func (b *Bus) PublishBroadcast(ctx context.Context, mapID string, data []byte, excludeUserID int32) error {
	body := encodeBroadcast(mapID, data, excludeUserID)
	return b.publish(ctx, chanBroadcast, msgTypeBroadcast, body)
}

func (b *Bus) PublishSendToUser(ctx context.Context, userID int32, data []byte) error {
	body := encodeSendToUser(userID, data)
	return b.publish(ctx, chanSendToUser, msgTypeSend, body)
}

func (b *Bus) PublishKickUser(ctx context.Context, userID int32) error {
	body := encodeKick(userID)
	return b.publish(ctx, chanKickUser, msgTypeKick, body)
}

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

// Broadcast body: [4 byte excludeUserID int32][2 byte mapIDLen][mapID][...data]
func encodeBroadcast(mapID string, data []byte, excludeUserID int32) []byte {
	mapBytes := []byte(mapID)
	buf := make([]byte, 4+2+len(mapBytes)+len(data))
	binary.BigEndian.PutUint32(buf[0:4], uint32(excludeUserID))
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(mapBytes)))
	copy(buf[6:6+len(mapBytes)], mapBytes)
	copy(buf[6+len(mapBytes):], data)
	return buf
}

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

// SendToUser body: [4 byte userID][...data]
func encodeSendToUser(userID int32, data []byte) []byte {
	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[0:4], uint32(userID))
	copy(buf[4:], data)
	return buf
}

func decodeSendToUser(body []byte) (userID int32, data []byte, err error) {
	if len(body) < 4 {
		return 0, nil, errors.New("send body too short")
	}
	userID = int32(binary.BigEndian.Uint32(body[0:4]))
	data = body[4:]
	return
}

// Kick body: [4 byte userID]
func encodeKick(userID int32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(userID))
	return buf
}

func decodeKick(body []byte) (int32, error) {
	if len(body) < 4 {
		return 0, errors.New("kick body too short")
	}
	return int32(binary.BigEndian.Uint32(body[0:4])), nil
}
