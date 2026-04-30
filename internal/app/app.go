package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	redisclient "github.com/DANG-PH/game-service-go/internal/infra/redis"

	"github.com/DANG-PH/game-service-go/internal/transport/ws"

	"github.com/DANG-PH/game-service-go/internal/game/player"
	"github.com/DANG-PH/game-service-go/internal/game/state"

	"github.com/DANG-PH/game-service-go/internal/config"
)

// App gom mọi component và quản lý lifecycle (start/stop).
type App struct {
	cfg    *config.Config
	log    *slog.Logger
	server *http.Server
	bus    ws.BusInterface // cross-instance message bus, cần Stop() khi shutdown
	ticker *ws.Ticker
}

// New khởi tạo app: load deps, wire components.
// Nếu khởi tạo fail → return error → main.go log và exit.
func New(cfg *config.Config, log *slog.Logger) (*App, error) {
	// Redis client cho business logic (player state, session, ...).
	rdb, err := redisclient.New(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("init redis: %w", err)
	}
	log.Info("redis connected", "url", cfg.RedisURL)

	var bus ws.BusInterface
	if cfg.UseNATS {
		log.Info("using NATS bus")
		bus, err = ws.NewNATSBus(cfg.NATSURL, log)
	} else {
		log.Info("using Redis bus")
		bus, err = ws.NewBus(cfg.RedisURL, log)
	}
	if err != nil {
		return nil, fmt.Errorf("init bus: %w", err)
	}

	// State manager — lưu snapshot mới nhất của mọi player theo map.
	// Tick loop sẽ đọc state này để broadcast.
	stateManager := state.NewManager()

	// Hub - quản lý WebSocket connections, wire bus để broadcast cross-instance.
	hub := ws.NewHub(log, bus)

	// Auth - verify JWT + gameSession.
	auth := ws.NewAuthenticator(cfg.JWTSecret, rdb)

	// Player service - logic xử lý move, update Redis.
	playerService := player.NewService(rdb)

	// Handler - dispatch message theo msgType.
	handler := ws.NewHandler(
		log,
		hub,
		stateManager,
		playerService,
	)

	// Tick loop — broadcast snapshot mỗi 50ms (20Hz).
	// Tách interval ra config nếu sau này muốn tune theo môi trường:
	//   - Dev: 100ms (đỡ noisy log)
	//   - Prod: 50ms (smooth gameplay)
	//   - Stress test: 33ms (30Hz)
	ticker := ws.NewTicker(log, hub, stateManager, time.Second/time.Duration(cfg.TickRate))

	// WebSocket server - HTTP upgrade endpoint.
	wsServer := ws.NewServer(log, hub, auth, handler)

	// Mux - đăng ký routes.
	mux := http.NewServeMux()
	mux.Handle("/ws-game", wsServer)

	// Healthcheck cho k8s/load balancer.
	// Thêm nodeID để debug multi-instance dễ hơn (curl healthcheck biết hit instance nào).
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		totalConns, totalRooms := hub.Stats()
		fmt.Fprintf(w, "ok\nnode=%s\nconns=%d\nrooms=%d\n", bus.NodeID(), totalConns, totalRooms)
	})

	// HTTP server với timeout config.
	// ReadHeaderTimeout chống slowloris (client gửi header chậm để giữ connection).
	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		// KHÔNG set ReadTimeout/WriteTimeout — sẽ kill WebSocket connection.
		// WebSocket có ping/pong riêng để detect dead connection.
	}

	return &App{
		cfg:    cfg,
		log:    log,
		server: server,
		bus:    bus,
		ticker: ticker,
	}, nil
}

// Run start HTTP server và bus subscriber. Block tới khi server fail hoặc Shutdown được gọi.
func (a *App) Run() error {
	// Start bus subscriber TRƯỚC khi accept WebSocket connection.
	// Nếu bus chưa start mà có conn vào → broadcast từ instance khác bị miss.
	// Dùng Background context — bus chạy đến khi Stop() được gọi.
	if err := a.bus.Start(context.Background()); err != nil {
		return fmt.Errorf("start bus: %w", err)
	}

	a.ticker.Start(context.Background())
	a.log.Info("tick loop started")

	a.log.Info("server starting", "port", a.cfg.HTTPPort, "nodeID", a.bus.NodeID())
	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown graceful — đợi current connection xong rồi mới close.
// Đặt timeout để không treo mãi nếu có connection stuck.
//
// Thứ tự shutdown quan trọng:
//  1. HTTP server stop accept conn mới + đợi conn hiện tại
//  2. Tick loop stop (không broadcast nữa, không ai nhận anyway vì conn đã close)
//  3. Bus stop
//
// Nếu đảo ngược: bus stop trước → conn còn lại broadcast sẽ fail publish → log noise.
// Tại sao tick loop stop SAU HTTP server?
// - Trong lúc server.Shutdown() đang đợi conn close, conn vẫn có thể nhận snapshot cuối.
// - Stop tick trước → conn nhận trải nghiệm "đứng hình" trong vài giây cuối → xấu hơn.
// - Stop tick sau → smooth tới phút cuối.
func (a *App) Shutdown(ctx context.Context) error {
	a.log.Info("server shutting down")

	if err := a.server.Shutdown(ctx); err != nil {
		// Vẫn cố gắng stop bus dù server shutdown lỗi.
		a.log.Warn("http server shutdown error", "err", err)
	}

	a.ticker.Stop()
	a.log.Info("tick loop stopped")

	a.bus.Stop()
	a.log.Info("bus stopped")
	return nil
}
