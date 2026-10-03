package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment string
	HTTPAddr    string
	DatabaseURL string
	LogLevel    slog.Level
	ReadTimeout time.Duration
	WriteTimeout time.Duration
	IdleTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Environment: env("VPNX3_ENV", "development"),
		HTTPAddr: env("VPNX3_HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("VPNX3_DATABASE_URL"),
		LogLevel: parseLogLevel(env("VPNX3_LOG_LEVEL", "info")),
		ReadTimeout: duration("VPNX3_HTTP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout: duration("VPNX3_HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout: duration("VPNX3_HTTP_IDLE_TIMEOUT", 60*time.Second),
	}

	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return Config{}, fmt.Errorf("VPNX3_HTTP_ADDR must not be empty")
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if value, err := time.ParseDuration(raw); err == nil {
		return value
	}
	return fallback
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
