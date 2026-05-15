package player

import (
	"context"
	"fmt"
	"strconv"
	"time"

	pb "github.com/DANG-PH/game-service-go/internal/protocol/pb"
	"github.com/redis/go-redis/v9"
)

// Service xử lý business logic player.
// Phase 1 chỉ có HandleMove. Sau này thêm load/save khi connect/disconnect.
//
// Pattern: Service không biết WebSocket — nó chỉ làm logic.
// Handler ở ws layer gọi Service rồi quyết định broadcast gì.
// Tách lớp này giúp test Service không cần WebSocket.
type Service struct {
	redis *redis.Client
}

func NewService(rdb *redis.Client) *Service {
	return &Service{redis: rdb}
}

// HandleMove update Redis state và trả về PlayerSync để broadcast.
//
// Khớp với logic NestJS player-move:
// 1. SET dirty:{userId} EX 600 NX  — đánh dấu cần save DB (NestJS có cron 20s flush)
// 2. HSET GAME:PLAYER:{userId} {...} — update full state
//
// Tại sao Go cũng làm dirty flag?
// Vì NestJS đang có cron đọc dirty flag để flush DB. Nếu Go không SET → cron không biết
// player đã move → không save DB → mất data khi server crash.
//
// Tối ưu sau (Phase 2): batch HSET pipeline mỗi tick thay vì mỗi move.
// Cách này dùng Cho cách 1 và cách 2 (Đọc ở Cuối file ticker.go để biết, có trade off là N round trip per player)
func (s *Service) HandleMove(ctx context.Context, userID int32, m *pb.PlayerMove) error {
	key := fmt.Sprintf("GAME:PLAYER:%d", userID)
	dirtyKey := fmt.Sprintf("dirty:%d", userID)

	// Convert trangthai từ byte enum sang string để khớp với NestJS Redis format.
	trangthaiStr := TrangthaiToString(m.Trangthai)

	// Pipeline: gửi nhiều command trong 1 round-trip.
	// Khác MULTI/EXEC ở chỗ pipeline KHÔNG atomic — nếu cần atomic thì dùng TxPipeline().
	// Player-move không cần atomic (write nhiều field, không có read-modify-write).
	pipe := s.redis.Pipeline()

	// SET dirty NX — chỉ set nếu chưa có. NestJS comment giải thích kỹ TTL 600s.
	pipe.SetNX(ctx, dirtyKey, time.Now().UnixMilli(), 600*time.Second)

	// HSET full state. Dùng map[string]interface{} cho gọn.
	pipe.HSet(ctx, key, map[string]interface{}{
		"x":              strconv.FormatFloat(float64(m.X), 'f', -1, 32),
		"y":              strconv.FormatFloat(float64(m.Y), 'f', -1, 32),
		"trangthai":      trangthaiStr,
		"dir":            int(m.Dir),
		"dau":            m.Dau,
		"than":           m.Than,
		"chan":           m.ChanField,
		"timeChoHienBay": strconv.FormatFloat(float64(m.TimeChoHienBay), 'f', -1, 32),
		"lechDauX":       strconv.FormatFloat(float64(m.LechDauX), 'f', -1, 32),
		"lechDauY":       strconv.FormatFloat(float64(m.LechDauY), 'f', -1, 32),
		"lechThanX":      strconv.FormatFloat(float64(m.LechThanX), 'f', -1, 32),
		"lechThanY":      strconv.FormatFloat(float64(m.LechThanY), 'f', -1, 32),
		"lechChanX":      strconv.FormatFloat(float64(m.LechChanX), 'f', -1, 32),
		"lechChanY":      strconv.FormatFloat(float64(m.LechChanY), 'f', -1, 32),
		"dangMangVanBay": m.DangMangVanBay,
		"tenVanBay":      m.TenVanBay,
		"rong":           strconv.FormatFloat(float64(m.Rong), 'f', -1, 32),
		"cao":            strconv.FormatFloat(float64(m.Cao), 'f', -1, 32),
		"avatar":         m.Avatar,
	})

	_, err := pipe.Exec(ctx)
	return err
}

// Cần struct riêng vì k thể thêm userID vào PlayerMove (cái này do client gửi lên, gửi thêm userID sẽ sai về mặt protocol)
type PlayerMoveWithID struct {
	UserID int32
	Move   *pb.PlayerMove
}

// HandleMoveBatch flush toàn bộ dirty players vào Redis trong 1 round-trip duy nhất.
//
// KHÁC HandleMove:
//
//	HandleMove: 1 player → 1 pipeline → 1 round-trip   (dùng cho per-move nếu cần)
//	HandleMoveBatch: N player → 1 pipeline → 1 round-trip (dùng cho timer flush 2s)
//
// TẠI SAO 1 ROUND-TRIP:
//
//	Pipeline gom tất cả command (SetNX + HSet) của mọi player vào 1 TCP packet.
//	Redis nhận, xử lý tuần tự, trả kết quả 1 lần.
//	Không phải N lần gửi-nhận mà là 1 lần gửi-nhận cho N player.
//
//	Ví dụ 500 dirty player:
//	  HandleMove loop:    500 round-trip × 0.5ms = 250ms
//	  HandleMoveBatch:    1  round-trip  × 0.5ms = 0.5ms
func (s *Service) HandleMoveBatch(ctx context.Context, players []PlayerMoveWithID) error {
	if len(players) == 0 {
		return nil
	}

	pipe := s.redis.Pipeline()
	now := time.Now().UnixMilli()

	for _, p := range players {
		key := fmt.Sprintf("GAME:PLAYER:%d", p.UserID)
		dirtyKey := fmt.Sprintf("dirty:%d", p.UserID)
		trangthaiStr := TrangthaiToString(p.Move.Trangthai)

		pipe.SetNX(ctx, dirtyKey, now, 600*time.Second)
		pipe.HSet(ctx, key, map[string]interface{}{
			"x":              strconv.FormatFloat(float64(p.Move.X), 'f', -1, 32),
			"y":              strconv.FormatFloat(float64(p.Move.Y), 'f', -1, 32),
			"trangthai":      trangthaiStr,
			"dir":            int(p.Move.Dir),
			"dau":            p.Move.Dau,
			"than":           p.Move.Than,
			"chan":           p.Move.ChanField,
			"timeChoHienBay": strconv.FormatFloat(float64(p.Move.TimeChoHienBay), 'f', -1, 32),
			"lechDauX":       strconv.FormatFloat(float64(p.Move.LechDauX), 'f', -1, 32),
			"lechDauY":       strconv.FormatFloat(float64(p.Move.LechDauY), 'f', -1, 32),
			"lechThanX":      strconv.FormatFloat(float64(p.Move.LechThanX), 'f', -1, 32),
			"lechThanY":      strconv.FormatFloat(float64(p.Move.LechThanY), 'f', -1, 32),
			"lechChanX":      strconv.FormatFloat(float64(p.Move.LechChanX), 'f', -1, 32),
			"lechChanY":      strconv.FormatFloat(float64(p.Move.LechChanY), 'f', -1, 32),
			"dangMangVanBay": p.Move.DangMangVanBay,
			"tenVanBay":      p.Move.TenVanBay,
			"rong":           strconv.FormatFloat(float64(p.Move.Rong), 'f', -1, 32),
			"cao":            strconv.FormatFloat(float64(p.Move.Cao), 'f', -1, 32),
			"avatar":         p.Move.Avatar,
		})
	}

	_, err := pipe.Exec(ctx)
	return err
}
