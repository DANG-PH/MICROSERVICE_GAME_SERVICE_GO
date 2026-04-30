package ws

import (
	"context"
	"log/slog"
	"time"

	"github.com/DANG-PH/game-service-go/internal/game/state"
	"github.com/DANG-PH/game-service-go/internal/shared/messages"
)

type Ticker struct {
	log     *slog.Logger
	hub     *Hub
	manager *state.Manager

	interval time.Duration
	stopCh   chan struct{}

	// === DEBUG STATS ===
	debugTickCount      int64
	debugBroadcastCount int64
	debugSkipCount      int64
	debugLastStatsTime  time.Time
}

func NewTicker(log *slog.Logger, hub *Hub, manager *state.Manager, interval time.Duration) *Ticker {
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	return &Ticker{
		log:                log,
		hub:                hub,
		manager:            manager,
		interval:           interval,
		stopCh:             make(chan struct{}),
		debugLastStatsTime: time.Now(),
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

func (t *Ticker) tick() {
	start := time.Now()
	maps := t.manager.AllMaps()

	totalPackets := 0
	totalDirty := 0

	for _, ms := range maps {
		dirty := ms.CollectDirty()
		if len(dirty) == 0 {
			t.debugSkipCount++
			continue
		}

		totalDirty += len(dirty)

		// === LOG MỖI BROADCAST (verbose) ===
		// Comment lại sau khi debug xong vì log nhiều
		userIDs := make([]int32, len(dirty))
		for i, p := range dirty {
			userIDs[i] = p.UserID
		}
		t.log.Info("tick broadcast",
			"mapID", ms.MapID,
			"dirtyCount", len(dirty),
			"userIDs", userIDs,
		)

		syncs := make([]messages.PlayerSync, len(dirty))
		for i := range dirty {
			syncs[i] = *dirty[i].ToSync()
		}

		batch := &messages.PlayerSyncBatch{Players: syncs}
		packet := batch.Encode()

		t.hub.BroadcastToMap(ms.MapID, packet, nil)
		totalPackets++
		t.debugBroadcastCount++
	}

	t.debugTickCount++

	// === STATS MỖI 5 GIÂY ===
	now := time.Now()
	if now.Sub(t.debugLastStatsTime) >= 5*time.Second {
		t.log.Info("ticker stats over 5s",
			"totalTicks", t.debugTickCount,
			"broadcasts", t.debugBroadcastCount,
			"skips", t.debugSkipCount,
			"broadcastRate", float64(t.debugBroadcastCount)/float64(t.debugTickCount)*100,
		)
		t.debugTickCount = 0
		t.debugBroadcastCount = 0
		t.debugSkipCount = 0
		t.debugLastStatsTime = now
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
