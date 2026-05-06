package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/DANG-PH/game-service-go/internal/app"
	"github.com/DANG-PH/game-service-go/internal/config"
)

func main() {
	_ = godotenv.Load()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	slog.SetDefault(log)

	cfg, err := config.Load()

	if err != nil {
		log.Error("config load failed", "err", err)
		os.Exit(1)
	}

	a, err := app.New(cfg, log)
	if err != nil {
		log.Error("app init failed", "err", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)

	go func() {
		errCh <- a.Run()
	}()

	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info("signal received", "signal", sig.String())

	case err := <-errCh:
		log.Error("server failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.Shutdown(ctx); err != nil {
		log.Error("shutdown failed", "err", err)
		os.Exit(1)
	}

	log.Info("server stopped")
}
