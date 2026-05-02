package loop

import (
	"context"
	"log/slog"
	"time"

	"github.com/DANG-PH/game-service-go/internal/game/player"
	"github.com/DANG-PH/game-service-go/internal/game/state"
	"github.com/DANG-PH/game-service-go/internal/shared/messages"
	"github.com/DANG-PH/game-service-go/internal/transport/ws"
)

type Ticker struct {
	log           *slog.Logger
	hub           *ws.Hub
	manager       *state.Manager
	playerService *player.Service

	interval      time.Duration
	flushInterval time.Duration // thêm, mặc định 2s
	lastFlush     time.Time     // thêm
	stopCh        chan struct{}
}

func NewTicker(log *slog.Logger, hub *ws.Hub, manager *state.Manager, playerService *player.Service, interval time.Duration) *Ticker {
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	return &Ticker{
		log:           log,
		hub:           hub,
		manager:       manager,
		playerService: playerService,
		interval:      interval,
		flushInterval: 2 * time.Second,
		lastFlush:     time.Now(),
		stopCh:        make(chan struct{}),
	}
}

func (t *Ticker) Start(ctx context.Context) {
	go t.run(ctx)
}

func (t *Ticker) Stop() {
	close(t.stopCh)
}

func (t *Ticker) run(ctx context.Context) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	t.log.Info("tick loop started", "interval", t.interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.tick()
		}
	}
}

// tick — mỗi 50ms.
//
// Pattern broadcast:
// - Lấy dirty player từ mỗi map
// - Mỗi player dirty → encode PlayerSync packet
// - Broadcast packet đó tới tất cả conn trong map (KHÔNG exclude chính chủ)
//
// Tại sao broadcast cả về chính chủ?
//   - Server-authoritative: client phải reconcile vị trí của chính mình theo server.
//   - Nếu anti-cheat reject input → server không update state → tick không gửi → client tự
//     biết là bị reject (snap về vị trí cũ).
//   - Trong NestJS cũ exclude chính chủ vì pattern client.to() — đây là khác biệt có chủ ý
//     khi chuyển sang server-authoritative.
func (t *Ticker) tick() {
	start := time.Now()
	maps := t.manager.AllMaps()
	doFlush := time.Since(t.lastFlush) >= t.flushInterval

	// Gom moves bên ngoài for loop để batch 1 lần sau
	var moves []player.PlayerMoveWithID

	totalPackets := 0
	for _, ms := range maps {
		var dirty []state.PlayerState
		var redisDirty []state.PlayerState

		if doFlush {
			dirty, redisDirty = ms.CollectDirtyBoth() // 1 lần lock
		} else {
			dirty = ms.CollectDirty() // 1 lần lock bình thường
		}

		if len(dirty) == 0 && len(redisDirty) == 0 {
			continue
		}

		// Broadcast
		syncs := make([]messages.PlayerSync, len(dirty))
		now := time.Now().UnixMilli()
		for i := range dirty {
			s := dirty[i].ToSync()
			s.ServerTime = now
			syncs[i] = *s
		}
		batch := &messages.PlayerSyncBatch{Players: syncs}
		t.hub.BroadcastToMap(ms.MapID, batch.Encode(), nil)
		totalPackets++

		// Gom RedisDirty — chưa flush, gom hết tất cả map rồi flush 1 lần
		for _, p := range redisDirty {
			moves = append(moves, player.PlayerMoveWithID{
				UserID: p.UserID,
				Move:   p.ToMove(),
			})
		}
	}

	// Flush toàn bộ trong 1 round-trip sau khi đã gom hết
	if doFlush {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := t.playerService.HandleMoveBatch(ctx, moves); err != nil {
				t.log.Warn("redis batch flush failed", "err", err, "count", len(moves))
			}
		}()
		t.lastFlush = time.Now()
	}

	elapsed := time.Since(start)
	if elapsed > t.interval {
		// critical: miss tick
	} else if elapsed > t.interval*3/4 {
		// warning: gần miss
	} else if elapsed > t.interval/2 {
		// info: bắt đầu nặng
	}
}

// =========================================================================
// TẠI SAO CẦN 2 DIRTY FLAG (Dirty vs RedisDirty)?
//
// Vấn đề nếu dùng chung 1 flag:
//   Tick 1 (0ms):   player move → CollectDirty → Dirty=false → broadcast ✅
//   Tick 2 (50ms):  player move → CollectDirty → Dirty=false → broadcast ✅
//   ...
//   Tick 40 (2s):   doFlush=true → CollectDirty trả [] vì đã reset hết → flush không có gì ❌
//
// Giải pháp: tách 2 flag độc lập
//   Dirty      → phục vụ broadcast 20Hz, reset sau mỗi tick
//   RedisDirty → phục vụ Redis flush 0.5Hz, reset sau khi flush
//
// =========================================================================
// SO SÁNH 3 CÁCH FLUSH REDIS:
//
// CÁCH 1 — Per move (cũ):
//   Mỗi packet move → Redis HSet ngay lập tức
//   ✅ Realtime tuyệt đối, NestJS luôn đọc được vị trí mới nhất
//   ❌ 20 move/s × 1000 player = 20,000 Redis write/s
//   ❌ Mỗi write là 1 goroutine riêng → 20,000 goroutine/s → GC pressure cao
//   ❌ Redis là bottleneck — game lag khi Redis chậm dù gameplay in-memory vẫn ok
//   ❌ Không batch → không tận dụng pipeline → nhiều round-trip TCP
//
// CÁCH 2 — Mỗi tick 20Hz:
//   Flush Redis cùng lúc broadcast, mỗi 50ms
//   ✅ Không cần 2 dirty flag — dùng dirty players vừa collect được
//   ✅ Fresh hơn cách 3 (50ms vs 2s)
//   ❌ 20Hz × 1000 player = vẫn 20,000 Redis write/s trong worst case
//   ❌ Redis write block tick loop → miss tick → gameplay giật
//      (tick loop phải chạy < 50ms, Redis write có thể 10-50ms/batch)
//   ❌ Chỉ giảm được goroutine overhead so với per move, không giảm write count
//
// CÁCH 3 — Timer riêng 2s (đang dùng):
//   Gom tất cả dirty trong 2s → flush 1 lần bằng pipeline
//   ✅ Redis write giảm 40x so với per move (từ 20,000/s xuống ~500/s)
//   ✅ Batch pipeline → 1 round-trip TCP cho nhiều player
//   ✅ Tick loop không bị Redis block (flush chạy độc lập)
//   ✅ NestJS cron flush DB chạy mỗi 20s → Redis chỉ cần fresh mỗi 2s là đủ
//   ❌ Vị trí trong Redis có thể trễ tối đa 2s so với in-memory
//      → Chấp nhận được vì Redis chỉ là snapshot cho NestJS, không ảnh hưởng gameplay
//      → Gameplay đọc từ in-memory MapState, không đọc từ Redis
//
// với case 1000 CCU :
// Cách 3
// - giảm x40 Write count
// 		20000+/s (cách 1 vì player có thể cheat, spawn 20000+ goroutine/s),
// 		20000/s (cách 2) -> 500/s (vì cách 2 1000/2s)
// - giảm x20000+, x20000 round trip xuống 1 TCP round trip
// 		giảm tải bằng batch handleMoveBatch để gom round trip thay vì dùng handleMove per player
// =========================================================================
