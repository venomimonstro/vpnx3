package config

import (
	"crypto/sha256"
	"encoding/hex"
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
	ReleaseSigningKey      string
	AccessLeaseTTL         time.Duration
	TrialDays              int
	ClientDNS              []string
	WireGuardMTU           int
	WireGuardKeepalive     int
	WireGuardGateway       string
	YooKassaShopID         string
	YooKassaSecretKey      string
	YooKassaReturnURL      string
	ArtifactStorage        string
	ArtifactDir            string
	ArtifactS3Endpoint     string
	ArtifactS3Region       string
	ArtifactS3Bucket       string
	ArtifactS3AccessKey    string
	ArtifactS3SecretKey    string
	ArtifactS3TempDir      string
	ArtifactMaxBytes       int64
	ArtifactRetentionDays  int
	ArtifactTransferTimeout time.Duration
	TrustedProxyCIDRs      []string
	RegistrationRateKey    string
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
		ReleaseSigningKey:      strings.TrimSpace(os.Getenv("VPNX3_RELEASE_SIGNING_KEY")),
		AccessLeaseTTL:         duration("VPNX3_ACCESS_LEASE_TTL", 6*time.Hour),
		TrialDays:              intEnv("VPNX3_TRIAL_DAYS", 7),
		ClientDNS:              csvEnv("VPNX3_CLIENT_DNS"),
		WireGuardMTU:           intEnv("VPNX3_WG_CLIENT_MTU", 1280),
		WireGuardKeepalive:     intEnv("VPNX3_WG_KEEPALIVE", 25),
		WireGuardGateway:       env("VPNX3_WG_GATEWAY_IPV4","10.66.0.1"),
		YooKassaShopID:         strings.TrimSpace(os.Getenv("VPNX3_YOOKASSA_SHOP_ID")),
		YooKassaSecretKey:      strings.TrimSpace(os.Getenv("VPNX3_YOOKASSA_SECRET_KEY")),
		YooKassaReturnURL:      strings.TrimSpace(os.Getenv("VPNX3_YOOKASSA_RETURN_URL")),
		ArtifactStorage:        strings.ToLower(env("VPNX3_ARTIFACT_STORAGE","local")),
		ArtifactDir:            env("VPNX3_ARTIFACT_DIR","/var/lib/vpnx3/artifacts"),
		ArtifactS3Endpoint:     strings.TrimSpace(os.Getenv("VPNX3_S3_ENDPOINT")),
		ArtifactS3Region:       strings.TrimSpace(os.Getenv("VPNX3_S3_REGION")),
		ArtifactS3Bucket:       strings.TrimSpace(os.Getenv("VPNX3_S3_BUCKET")),
		ArtifactS3AccessKey:    strings.TrimSpace(os.Getenv("VPNX3_S3_ACCESS_KEY")),
		ArtifactS3SecretKey:    strings.TrimSpace(os.Getenv("VPNX3_S3_SECRET_KEY")),
		ArtifactS3TempDir:      env("VPNX3_S3_TEMP_DIR","/var/lib/vpnx3/s3tmp"),
		ArtifactMaxBytes:       int64(intEnv("VPNX3_ARTIFACT_MAX_MB",300))*1024*1024,
		ArtifactRetentionDays:  intEnv("VPNX3_ARTIFACT_RETENTION_DAYS",30),
		ArtifactTransferTimeout: duration("VPNX3_ARTIFACT_TRANSFER_TIMEOUT",30*time.Minute),
		TrustedProxyCIDRs:      csvEnv("VPNX3_TRUSTED_PROXY_CIDRS"),
		RegistrationRateKey:    strings.TrimSpace(os.Getenv("VPNX3_REGISTRATION_RATE_KEY")),
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
	if cfg.RegistrationRateKey=="" {
		sum:=sha256.Sum256([]byte("vpnx3-registration-rate-v1\x00"+cfg.AccessSigningKey))
		cfg.RegistrationRateKey=hex.EncodeToString(sum[:])
	}
	if len(cfg.RegistrationRateKey)<32 {
		return Config{}, fmt.Errorf("VPNX3_REGISTRATION_RATE_KEY must be at least 32 characters when explicitly set")
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
	if ip:=net.ParseIP(cfg.WireGuardGateway);ip==nil||ip.To4()==nil {
		return Config{}, fmt.Errorf("VPNX3_WG_GATEWAY_IPV4 must be a valid IPv4 address")
	}
	if cfg.ArtifactStorage!="local" && cfg.ArtifactStorage!="s3" {
		return Config{}, fmt.Errorf("VPNX3_ARTIFACT_STORAGE must be local or s3")
	}
	if cfg.ArtifactStorage=="s3" {
		if !strings.HasPrefix(cfg.ArtifactS3Endpoint,"https://") ||
			cfg.ArtifactS3Region=="" || cfg.ArtifactS3Bucket=="" ||
			cfg.ArtifactS3AccessKey=="" || cfg.ArtifactS3SecretKey=="" {
			return Config{}, fmt.Errorf("S3 artifact storage requires https endpoint, region, bucket, access key and secret key")
		}
	}
	if cfg.ArtifactTransferTimeout < time.Minute || cfg.ArtifactTransferTimeout > 2*time.Hour {
		return Config{}, fmt.Errorf("VPNX3_ARTIFACT_TRANSFER_TIMEOUT must be between 1m and 2h")
	}
	if cfg.ArtifactRetentionDays < 1 || cfg.ArtifactRetentionDays > 3650 {
		return Config{}, fmt.Errorf("VPNX3_ARTIFACT_RETENTION_DAYS must be between 1 and 3650")
	}
	if cfg.ArtifactMaxBytes < 1<<20 || cfg.ArtifactMaxBytes > 2<<30 {
		return Config{}, fmt.Errorf("VPNX3_ARTIFACT_MAX_MB is outside safe limits")
	}
	configuredYooKassa:=cfg.YooKassaShopID!="" || cfg.YooKassaSecretKey!="" || cfg.YooKassaReturnURL!=""
	if configuredYooKassa && (cfg.YooKassaShopID=="" || cfg.YooKassaSecretKey=="" || !strings.HasPrefix(cfg.YooKassaReturnURL,"https://")) {
		return Config{}, fmt.Errorf("YooKassa configuration requires shop id, secret key and https return URL")
	}
	for _, cidr := range cfg.TrustedProxyCIDRs {
		if _,_,err:=net.ParseCIDR(cidr);err!=nil {
			return Config{}, fmt.Errorf("VPNX3_TRUSTED_PROXY_CIDRS contains invalid CIDR %q",cidr)
		}
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
