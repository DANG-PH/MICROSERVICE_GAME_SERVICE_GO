package ws

import (
	"context"
	"log/slog"
	"time"

	"github.com/DANG-PH/game-service-go/internal/game/state"
)

type Ticker struct {
	log     *slog.Logger
	hub     *Hub
	manager *state.Manager

	interval time.Duration
	stopCh   chan struct{}
}

func NewTicker(log *slog.Logger, hub *Hub, manager *state.Manager, interval time.Duration) *Ticker {
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	return &Ticker{
		log:      log,
		hub:      hub,
		manager:  manager,
		interval: interval,
		stopCh:   make(chan struct{}),
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

	totalPackets := 0
	for _, ms := range maps {
		dirty := ms.CollectDirty()
		if len(dirty) == 0 {
			continue
		}

		for i := range dirty {
			p := &dirty[i]
			sync := p.ToSync()
			packet := sync.Encode()

			// Để giữ parity NestJS (exclude chính chủ): bạn cần lookup conn theo UserID
			// rồi pass vào BroadcastToMap. Hiện tại conn lookup nằm trong Hub:
			//   hub.BroadcastToMapExcludeUser(ms.MapID, packet, p.UserID)
			// Mình thêm helper này dưới.
			t.hub.BroadcastToMapExcludeUser(ms.MapID, packet, p.UserID)
			totalPackets++
		}
	}

	elapsed := time.Since(start)
	if elapsed > t.interval/2 {
		t.log.Warn("tick slow",
			"elapsed", elapsed,
			"maps", len(maps),
			"packets", totalPackets,
		)
	}
}
