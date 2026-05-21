package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/DANG-PH/game-service-go/internal/auth"
	"github.com/DANG-PH/game-service-go/internal/infra/bus"
	redisclient "github.com/DANG-PH/game-service-go/internal/infra/redis"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/DANG-PH/game-service-go/internal/transport/ws"

	"github.com/DANG-PH/game-service-go/internal/game/loop"
	"github.com/DANG-PH/game-service-go/internal/game/player"
	"github.com/DANG-PH/game-service-go/internal/game/state"

	"github.com/DANG-PH/game-service-go/internal/config"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	server *http.Server
	bus    bus.BusInterface
	ticker *loop.Ticker
	hub    *ws.Hub
}

func New(cfg *config.Config, log *slog.Logger) (*App, error) {
	// Redis client
	rdb, err := redisclient.New(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("init redis: %w", err)
	}
	log.Info("redis connected", "url", cfg.RedisURL)

	// Message Broker
	var b bus.BusInterface
	if cfg.UseNATS {
		log.Info("using NATS bus")
		b, err = bus.NewNATSBus(cfg.NATSURL, log)
	} else {
		log.Info("using Redis bus")
		b, err = bus.NewBus(cfg.RedisURL, log)
	}
	if err != nil {
		return nil, fmt.Errorf("init bus: %w", err)
	}

	// State manager — lưu snapshot mới nhất của mọi player theo map.
	// Tick loop sẽ đọc state này để broadcast.
	stateManager := state.NewManager()

	// Hub - quản lý WebSocket connections, wire bus, stateManager để broadcast cross-instance.
	hub := ws.NewHub(log, b, stateManager)

	// Auth - verify JWT + gameSession, wire rdb.
	auth := auth.NewAuthenticator(cfg.JWTSecret, rdb)

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
	ticker := loop.NewTicker(log, hub, stateManager, playerService, time.Second/time.Duration(cfg.TickRate))

	// WebSocket server - HTTP upgrade endpoint.
	wsServer := ws.NewServer(log, hub, auth, handler)

	// Mux - đăng ký routes.
	mux := http.NewServeMux()
	mux.Handle("/ws-game", wsServer)

	// Healthcheck cho k8s/load balancer.
	// Thêm nodeID để debug multi-instance dễ hơn (curl healthcheck biết hit instance nào).
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		totalConns, totalRooms := hub.Stats()
		fmt.Fprintf(w, "ok\nnode=%s\nconns=%d\nrooms=%d\n", b.NodeID(), totalConns, totalRooms)
	})

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe("0.0.0.0:2112", metricsMux)

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
		bus:    b,
		ticker: ticker,
		hub:    hub,
	}, nil
}

// Run start HTTP server
func (a *App) Run() error {
	// Start bus subscriber TRƯỚC khi accept WebSocket connection.
	// Nếu bus chưa start mà có conn vào → broadcast từ instance khác bị miss.
	// Dùng Background context — bus chạy đến khi Stop() được gọi.
	// TODO: Cần fix lại (anti pattern)
	if err := a.bus.Start(context.Background()); err != nil {
		return fmt.Errorf("start bus: %w", err)
	}

	// TODO: Cần fix lại (anti pattern)
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
func (a *App) Shutdown(ctx context.Context) error {
	// Thứ tự shutdown cũng rất quan trọng
	a.log.Info("server shutting down")

	if err := a.server.Shutdown(ctx); err != nil {
		// Vẫn cố gắng stop bus dù server shutdown lỗi.
		a.log.Warn("http server shutdown error", "err", err)
	}

	// tick loop stop SAU HTTP server
	// - Stop tick trước → conn nhận trải nghiệm "đứng hình" trong vài giây cuối → xấu hơn.
	// - Stop tick sau → smooth tới phút cuối.
	a.ticker.Stop()
	a.log.Info("tick loop stopped")

	// nếu bus stop trước → conn còn lại broadcast sẽ fail publish → log noise.
	a.bus.Stop()
	a.log.Info("bus stopped")

	// Hub close sau cùng để đảm bảo worker drain hết job NATS còn trong channel trước khi thoát.
	a.hub.Close() // drain publish channel, worker tự thoát
	a.log.Info("hub closed")

	return nil
}
