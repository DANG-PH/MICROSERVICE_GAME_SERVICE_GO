package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DANG-PH/game-service-go/internal/config"
	"github.com/DANG-PH/game-service-go/internal/transport/http/middleware"
)

type App struct {
	cfg    *config.Config
	server *http.Server
}

func New(cfg *config.Config) *App {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: thêm DB ping ở đây khi có DB
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})

	// Chain middleware: recovery bắt panic → logger ghi log → mux xử lý
	handler := middleware.Recovery(middleware.Logger(mux))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &App{cfg: cfg, server: server}
}

func (a *App) Run() error {
	errCh := make(chan error, 1)

	go func() {
		fmt.Printf("server running on :%d (env: %s)\n", a.cfg.App.Port, a.cfg.App.Env)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-quit:
		fmt.Println("shutting down...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	fmt.Println("server stopped")
	return nil
}
