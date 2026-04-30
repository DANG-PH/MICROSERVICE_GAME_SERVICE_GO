// internal/game/player/service.go
package player

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/DANG-PH/game-service-go/internal/shared/enums"
	"github.com/DANG-PH/game-service-go/internal/shared/messages"
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
func (s *Service) HandleMove(ctx context.Context, userID int32, m *messages.PlayerMove) error {
	key := fmt.Sprintf("GAME:PLAYER:%d", userID)
	dirtyKey := fmt.Sprintf("dirty:%d", userID)

	// Convert trangthai từ uint8 enum sang string để khớp với NestJS Redis format.
	trangthaiStr := enums.TrangthaiToString(m.Trangthai)

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
		"chan":           m.Chan,
		"timeChoHienBay": strconv.FormatFloat(float64(m.TimeChoHienBay), 'f', -1, 32),
		"lechDauX":       strconv.FormatFloat(float64(m.LechDauX), 'f', -1, 32),
		"lechDauY":       strconv.FormatFloat(float64(m.LechDauY), 'f', -1, 32),
		"lechThanX":      strconv.FormatFloat(float64(m.LechThanX), 'f', -1, 32),
		"lechThanY":      strconv.FormatFloat(float64(m.LechThanY), 'f', -1, 32),
		"lechChanX":      strconv.FormatFloat(float64(m.LechChanX), 'f', -1, 32),
		"lechChanY":      strconv.FormatFloat(float64(m.LechChanY), 'f', -1, 32),
		"frameVanBay":    int(m.FrameVanBay),
		"dangMangVanBay": m.DangMangVanBay,
		"tenVanBay":      m.TenVanBay,
		"rong":           strconv.FormatFloat(float64(m.Rong), 'f', -1, 32),
		"cao":            strconv.FormatFloat(float64(m.Cao), 'f', -1, 32),
		"avatar":         m.Avatar,
	})

	_, err := pipe.Exec(ctx)
	return err
}
