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
	Environment            string
	HTTPAddr               string
	DatabaseURL            string
	LogLevel               slog.Level
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	IdleTimeout            time.Duration
	AdminSessionTTL        time.Duration
	BootstrapOwnerEmail    string
	BootstrapOwnerPassword string
	ConfigSigningKey       string
	AccessSigningKey       string
	AccessLeaseTTL         time.Duration
	TrialDays              int
}

func Load() (Config, error) {
	cfg := Config{
		Environment:            env("VPNX3_ENV", "development"),
		HTTPAddr:               env("VPNX3_HTTP_ADDR", ":8080"),
		DatabaseURL:            os.Getenv("VPNX3_DATABASE_URL"),
		LogLevel:               parseLogLevel(env("VPNX3_LOG_LEVEL", "info")),
		ReadTimeout:            duration("VPNX3_HTTP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout:           duration("VPNX3_HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:            duration("VPNX3_HTTP_IDLE_TIMEOUT", 60*time.Second),
		AdminSessionTTL:        duration("VPNX3_ADMIN_SESSION_TTL", 12*time.Hour),
		BootstrapOwnerEmail:    strings.TrimSpace(os.Getenv("VPNX3_BOOTSTRAP_OWNER_EMAIL")),
		BootstrapOwnerPassword: os.Getenv("VPNX3_BOOTSTRAP_OWNER_PASSWORD"),
		ConfigSigningKey:       strings.TrimSpace(os.Getenv("VPNX3_CONFIG_SIGNING_KEY")),
		AccessSigningKey:       strings.TrimSpace(os.Getenv("VPNX3_ACCESS_SIGNING_KEY")),
		AccessLeaseTTL:         duration("VPNX3_ACCESS_LEASE_TTL",6*time.Hour),
		TrialDays:              intEnv("VPNX3_TRIAL_DAYS",7),
	}

	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return Config{}, fmt.Errorf("VPNX3_HTTP_ADDR must not be empty")
	}
	if cfg.ConfigSigningKey == "" {
		return Config{}, fmt.Errorf("VPNX3_CONFIG_SIGNING_KEY must be set")
	}
	if cfg.AccessSigningKey == "" {
		return Config{}, fmt.Errorf("VPNX3_ACCESS_SIGNING_KEY must be set")
	}
	if cfg.AccessLeaseTTL < 15*time.Minute || cfg.AccessLeaseTTL > 24*time.Hour {
		return Config{}, fmt.Errorf("VPNX3_ACCESS_LEASE_TTL must be between 15m and 24h")
	}
	if cfg.TrialDays < 0 || cfg.TrialDays > 30 {
		return Config{}, fmt.Errorf("VPNX3_TRIAL_DAYS must be between 0 and 30")
	}
	if cfg.AdminSessionTTL < 15*time.Minute || cfg.AdminSessionTTL > 7*24*time.Hour {
		return Config{}, fmt.Errorf("VPNX3_ADMIN_SESSION_TTL must be between 15m and 168h")
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

func intEnv(key string,fallback int) int {
	raw:=strings.TrimSpace(os.Getenv(key))
	if raw=="" { return fallback }
	v,err:=strconv.Atoi(raw)
	if err!=nil { return fallback }
	return v
}
