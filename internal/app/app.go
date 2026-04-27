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

	"github.com/DANG-PH/game-service-go/internal/config"
)

// App gom mọi component và quản lý lifecycle (start/stop).
type App struct {
	cfg    *config.Config
	log    *slog.Logger
	server *http.Server
}

// New khởi tạo app: load deps, wire components.
// Nếu khởi tạo fail → return error → main.go log và exit.
func New(cfg *config.Config, log *slog.Logger) (*App, error) {
	// Redis.
	rdb, err := redisclient.New(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("init redis: %w", err)
	}
	log.Info("redis connected", "url", cfg.RedisURL)

	// Hub - quản lý WebSocket connections.
	hub := ws.NewHub(log)

	// Auth - verify JWT + gameSession.
	auth := ws.NewAuthenticator(cfg.JWTSecret, rdb)

	// Player service - logic xử lý move, update Redis.
	playerService := player.NewService(rdb)

	// Handler - dispatch message theo msgType.
	handler := ws.NewHandler(log, hub, playerService)

	// WebSocket server - HTTP upgrade endpoint.
	wsServer := ws.NewServer(log, hub, auth, handler)

	// Mux - đăng ký routes.
	mux := http.NewServeMux()
	mux.Handle("/ws-game", wsServer)

	// Healthcheck cho k8s/load balancer.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		totalConns, totalRooms := hub.Stats()
		fmt.Fprintf(w, "ok\nconns=%d\nrooms=%d\n", totalConns, totalRooms)
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
	}, nil
}

// Run start HTTP server. Block tới khi server fail hoặc Shutdown được gọi.
func (a *App) Run() error {
	a.log.Info("server starting", "port", a.cfg.HTTPPort)
	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown graceful — đợi current connection xong rồi mới close.
// Đặt timeout để không treo mãi nếu có connection stuck.
func (a *App) Shutdown(ctx context.Context) error {
	a.log.Info("server shutting down")
	return a.server.Shutdown(ctx)
}
