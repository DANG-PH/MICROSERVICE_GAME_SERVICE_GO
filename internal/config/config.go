package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config gom toàn bộ config của service.
// Pattern này gọi là "config struct" — load 1 lần lúc start, inject xuống các module.
// Tránh đọc os.Getenv rải rác khắp code (khó test, khó audit).
type Config struct {
	HTTPPort  string
	RedisURL  string
	JWTSecret string
	LogLevel  string
	UseNATS   bool
	NATSURL   string
	TickRate  int
}

// Load đọc env vars. Trả về error nếu thiếu config bắt buộc.
// Fail fast: thà crash lúc start còn hơn chạy với config sai rồi crash giữa chừng.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPPort:  getEnv("HTTP_PORT", "3001"),
		RedisURL:  getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		LogLevel:  getEnv("LOG_LEVEL", "info"),
		UseNATS:   getEnvBool("USE_NATS", false),
		NATSURL:   getEnv("NATS_URL", "nats://localhost:4222"),
		TickRate:  getEnvInt("TICK_RATE", 20),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

// string -> string
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// string -> int
func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

// string -> bool
func getEnvBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return defaultValue
}
