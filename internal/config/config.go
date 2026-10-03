package config

import (
	"fmt"
	"log/slog"
	"net"
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
	ClientDNS              []string
	WireGuardMTU           int
	WireGuardKeepalive     int
	YooKassaShopID         string
	YooKassaSecretKey      string
	YooKassaReturnURL      string
	ArtifactDir            string
	ArtifactMaxBytes       int64
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
		AccessLeaseTTL:         duration("VPNX3_ACCESS_LEASE_TTL", 6*time.Hour),
		TrialDays:              intEnv("VPNX3_TRIAL_DAYS", 7),
		ClientDNS:              csvEnv("VPNX3_CLIENT_DNS"),
		WireGuardMTU:           intEnv("VPNX3_WG_CLIENT_MTU", 1280),
		WireGuardKeepalive:     intEnv("VPNX3_WG_KEEPALIVE", 25),
		YooKassaShopID:         strings.TrimSpace(os.Getenv("VPNX3_YOOKASSA_SHOP_ID")),
		YooKassaSecretKey:      strings.TrimSpace(os.Getenv("VPNX3_YOOKASSA_SECRET_KEY")),
		YooKassaReturnURL:      strings.TrimSpace(os.Getenv("VPNX3_YOOKASSA_RETURN_URL")),
		ArtifactDir:            env("VPNX3_ARTIFACT_DIR","/var/lib/vpnx3/artifacts"),
		ArtifactMaxBytes:       int64(intEnv("VPNX3_ARTIFACT_MAX_MB",300))*1024*1024,
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
	if cfg.WireGuardMTU < 576 || cfg.WireGuardMTU > 1500 {
		return Config{}, fmt.Errorf("VPNX3_WG_CLIENT_MTU must be between 576 and 1500")
	}
	if cfg.WireGuardKeepalive < 0 || cfg.WireGuardKeepalive > 120 {
		return Config{}, fmt.Errorf("VPNX3_WG_KEEPALIVE must be between 0 and 120")
	}
	if cfg.ArtifactMaxBytes < 1<<20 || cfg.ArtifactMaxBytes > 2<<30 {
		return Config{}, fmt.Errorf("VPNX3_ARTIFACT_MAX_MB is outside safe limits")
	}
	configuredYooKassa:=cfg.YooKassaShopID!="" || cfg.YooKassaSecretKey!="" || cfg.YooKassaReturnURL!=""
	if configuredYooKassa && (cfg.YooKassaShopID=="" || cfg.YooKassaSecretKey=="" || !strings.HasPrefix(cfg.YooKassaReturnURL,"https://")) {
		return Config{}, fmt.Errorf("YooKassa configuration requires shop id, secret key and https return URL")
	}
	for _, dns := range cfg.ClientDNS {
		if net.ParseIP(dns) == nil {
			return Config{}, fmt.Errorf("VPNX3_CLIENT_DNS contains invalid IP %q", dns)
		}
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
	if raw == "" { return fallback }
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if value, err := time.ParseDuration(raw); err == nil { return value }
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

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" { return fallback }
	v, err := strconv.Atoi(raw)
	if err != nil { return fallback }
	return v
}

func csvEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" { return []string{} }
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" { out = append(out, v) }
	}
	return out
}
