package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/DANG-PH/game-service-go/internal/config"

	"github.com/DANG-PH/game-service-go/internal/app"
)

func main() {
	// Load .env nếu có. Production set env vars qua docker/k8s, không cần file .env.
	_ = godotenv.Load()

	// Logger - JSON cho production, dễ ingest vào log aggregator.
	// Dev local có thể đổi sang TextHandler cho dễ đọc.
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(log)

	// Load config.
	cfg, err := config.Load()
	if err != nil {
		log.Error("config load failed", "err", err)
		os.Exit(1)
	}

	// Init app.
	a, err := app.New(cfg, log)
	if err != nil {
		log.Error("app init failed", "err", err)
		os.Exit(1)
	}

	// Run trong goroutine để có thể đồng thời chờ signal shutdown.
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run()
	}()

	// Đợi SIGINT/SIGTERM để graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info("signal received", "signal", sig.String())
	case err := <-errCh:
		log.Error("server failed", "err", err)
		os.Exit(1)
	}

	// Graceful shutdown với timeout 10s.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.Shutdown(ctx); err != nil {
		log.Error("shutdown failed", "err", err)
		os.Exit(1)
	}

	log.Info("server stopped")
}
