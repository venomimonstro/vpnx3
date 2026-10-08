package config

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment            string
	HTTPAddr               string
	DatabaseURL            string
	DatabaseReplicaURL     string
	DatabaseHARequired     bool
	DatabaseMaxConns       int
	DatabaseMinConns       int
	DatabaseConnLifetime   time.Duration
	DatabaseConnJitter     time.Duration
	DatabaseConnIdleTime   time.Duration
	DatabaseStatementTimeout time.Duration
	DatabaseLockTimeout      time.Duration
	DatabaseIdleTxTimeout    time.Duration
	LogLevel               slog.Level
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	IdleTimeout            time.Duration
	DrainDelay             time.Duration
	ShutdownTimeout        time.Duration
	HTTPMaxInflight        int
	HTTPMaxHeaderBytes     int
	HTTPDefaultBodyBytes   int64
	AdminSessionTTL        time.Duration
	AdminSessionBinding    string
	BootstrapOwnerEmail    string
	BootstrapOwnerPassword string
	ConfigSigningKey       string
	AccessSigningKey       string
	ReleaseSigningKey      string
	TrustRootPublicKey    string
	TrustBundleFile       string
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
	BackupStatusFile       string
	OffsiteBackupStatusFile string
	WalOffsiteStatusFile   string
	SecurityExportURL      string
	SecurityExportSigningKey string
	AlertWebhookURL         string
	AlertWebhookSecret      string
}

func Load() (Config, error) {
	cfg := Config{
		Environment:            strings.ToLower(env("VPNX3_ENV", "development")),
		HTTPAddr:               env("VPNX3_HTTP_ADDR", ":8080"),
		DatabaseURL:            os.Getenv("VPNX3_DATABASE_URL"),
		DatabaseReplicaURL:     strings.TrimSpace(os.Getenv("VPNX3_DATABASE_REPLICA_URL")),
		DatabaseHARequired:     boolEnv("VPNX3_DATABASE_HA_REQUIRED",false),
		DatabaseMaxConns:       intEnv("VPNX3_DATABASE_MAX_CONNS",20),
		DatabaseMinConns:       intEnv("VPNX3_DATABASE_MIN_CONNS",2),
		DatabaseConnLifetime:   duration("VPNX3_DATABASE_CONN_LIFETIME",30*time.Minute),
		DatabaseConnJitter:     duration("VPNX3_DATABASE_CONN_JITTER",5*time.Minute),
		DatabaseConnIdleTime:   duration("VPNX3_DATABASE_CONN_IDLE_TIME",5*time.Minute),
		DatabaseStatementTimeout: duration("VPNX3_DATABASE_STATEMENT_TIMEOUT",8*time.Second),
		DatabaseLockTimeout:      duration("VPNX3_DATABASE_LOCK_TIMEOUT",3*time.Second),
		DatabaseIdleTxTimeout:    duration("VPNX3_DATABASE_IDLE_TX_TIMEOUT",15*time.Second),
		LogLevel:               parseLogLevel(env("VPNX3_LOG_LEVEL", "info")),
		ReadTimeout:            duration("VPNX3_HTTP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout:           duration("VPNX3_HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:            duration("VPNX3_HTTP_IDLE_TIMEOUT", 60*time.Second),
		DrainDelay:             duration("VPNX3_DRAIN_DELAY", 5*time.Second),
		ShutdownTimeout:        duration("VPNX3_SHUTDOWN_TIMEOUT", 20*time.Second),
		HTTPMaxInflight:        intEnv("VPNX3_HTTP_MAX_INFLIGHT",512),
		HTTPMaxHeaderBytes:     intEnv("VPNX3_HTTP_MAX_HEADER_KB",32)*1024,
		HTTPDefaultBodyBytes:   int64(intEnv("VPNX3_HTTP_DEFAULT_BODY_KB",1024))*1024,
		AdminSessionTTL:        duration("VPNX3_ADMIN_SESSION_TTL", 12*time.Hour),
		AdminSessionBinding:    strings.ToLower(env("VPNX3_ADMIN_SESSION_BINDING","user-agent")),
		BootstrapOwnerEmail:    strings.TrimSpace(os.Getenv("VPNX3_BOOTSTRAP_OWNER_EMAIL")),
		BootstrapOwnerPassword: os.Getenv("VPNX3_BOOTSTRAP_OWNER_PASSWORD"),
		ConfigSigningKey:       strings.TrimSpace(os.Getenv("VPNX3_CONFIG_SIGNING_KEY")),
		AccessSigningKey:       strings.TrimSpace(os.Getenv("VPNX3_ACCESS_SIGNING_KEY")),
		ReleaseSigningKey:      strings.TrimSpace(os.Getenv("VPNX3_RELEASE_SIGNING_KEY")),
		TrustRootPublicKey:    strings.TrimSpace(os.Getenv("VPNX3_TRUST_ROOT_PUBLIC_KEY")),
		TrustBundleFile:       strings.TrimSpace(os.Getenv("VPNX3_TRUST_BUNDLE_FILE")),
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
		BackupStatusFile:       strings.TrimSpace(os.Getenv("VPNX3_BACKUP_STATUS_FILE")),
		OffsiteBackupStatusFile: strings.TrimSpace(os.Getenv("VPNX3_BACKUP_OFFSITE_STATUS_FILE")),
		WalOffsiteStatusFile:    strings.TrimSpace(os.Getenv("VPNX3_WAL_OFFSITE_STATUS_FILE")),
		SecurityExportURL:      strings.TrimSpace(os.Getenv("VPNX3_SECURITY_EXPORT_URL")),
		SecurityExportSigningKey: strings.TrimSpace(os.Getenv("VPNX3_SECURITY_EXPORT_SIGNING_KEY")),
		AlertWebhookURL:         strings.TrimSpace(os.Getenv("VPNX3_ALERT_WEBHOOK_URL")),
		AlertWebhookSecret:      strings.TrimSpace(os.Getenv("VPNX3_ALERT_WEBHOOK_SECRET")),
	}

	if strings.TrimSpace(cfg.HTTPAddr) == "" {
		return Config{}, fmt.Errorf("VPNX3_HTTP_ADDR must not be empty")
	}
	if cfg.DatabaseMaxConns < 4 || cfg.DatabaseMaxConns > 500 {
		return Config{},fmt.Errorf("VPNX3_DATABASE_MAX_CONNS must be between 4 and 500")
	}
	if cfg.DatabaseMinConns < 0 || cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{},fmt.Errorf("VPNX3_DATABASE_MIN_CONNS must be between 0 and max conns")
	}
	if cfg.DatabaseConnLifetime < 5*time.Minute || cfg.DatabaseConnLifetime > 24*time.Hour {
		return Config{},fmt.Errorf("VPNX3_DATABASE_CONN_LIFETIME must be between 5m and 24h")
	}
	if cfg.DatabaseConnJitter < 0 || cfg.DatabaseConnJitter > cfg.DatabaseConnLifetime/2 {
		return Config{},fmt.Errorf("VPNX3_DATABASE_CONN_JITTER must be non-negative and not exceed half connection lifetime")
	}
	if cfg.DatabaseConnIdleTime < time.Minute || cfg.DatabaseConnIdleTime > time.Hour {
		return Config{},fmt.Errorf("VPNX3_DATABASE_CONN_IDLE_TIME must be between 1m and 1h")
	}
	if cfg.DatabaseStatementTimeout < time.Second || cfg.DatabaseStatementTimeout > 2*time.Minute {
		return Config{},fmt.Errorf("VPNX3_DATABASE_STATEMENT_TIMEOUT must be between 1s and 2m")
	}
	if cfg.DatabaseLockTimeout < 100*time.Millisecond || cfg.DatabaseLockTimeout > 30*time.Second {
		return Config{},fmt.Errorf("VPNX3_DATABASE_LOCK_TIMEOUT must be between 100ms and 30s")
	}
	if cfg.DatabaseLockTimeout >= cfg.DatabaseStatementTimeout {
		return Config{},fmt.Errorf("VPNX3_DATABASE_LOCK_TIMEOUT must be lower than statement timeout")
	}
	if cfg.DatabaseIdleTxTimeout < 5*time.Second || cfg.DatabaseIdleTxTimeout > 10*time.Minute {
		return Config{},fmt.Errorf("VPNX3_DATABASE_IDLE_TX_TIMEOUT must be between 5s and 10m")
	}
	if cfg.ConfigSigningKey == "" {
		return Config{}, fmt.Errorf("VPNX3_CONFIG_SIGNING_KEY must be set")
	}
	if cfg.AccessSigningKey == "" {
		return Config{}, fmt.Errorf("VPNX3_ACCESS_SIGNING_KEY must be set")
	}
	if err:=validateSigningSeeds(cfg.ConfigSigningKey,cfg.AccessSigningKey,cfg.ReleaseSigningKey);err!=nil{
		return Config{},err
	}
	trustConfigured:=cfg.TrustRootPublicKey!=""||cfg.TrustBundleFile!=""
	if trustConfigured{
		if cfg.TrustRootPublicKey==""||cfg.TrustBundleFile==""{
			return Config{},fmt.Errorf("trust bundle requires VPNX3_TRUST_ROOT_PUBLIC_KEY and VPNX3_TRUST_BUNDLE_FILE")
		}
		raw,err:=base64.RawURLEncoding.DecodeString(cfg.TrustRootPublicKey)
		if err!=nil||len(raw)!=ed25519.PublicKeySize{
			return Config{},fmt.Errorf("VPNX3_TRUST_ROOT_PUBLIC_KEY must be a 32-byte Ed25519 public key in base64url")
		}
		if !strings.HasPrefix(cfg.TrustBundleFile,"/"){
			return Config{},fmt.Errorf("VPNX3_TRUST_BUNDLE_FILE must be an absolute path")
		}
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
	if cfg.DrainDelay < 0 || cfg.DrainDelay > 60*time.Second {
		return Config{}, fmt.Errorf("VPNX3_DRAIN_DELAY must be between 0s and 60s")
	}
	if cfg.ShutdownTimeout < 5*time.Second || cfg.ShutdownTimeout > 2*time.Minute {
		return Config{}, fmt.Errorf("VPNX3_SHUTDOWN_TIMEOUT must be between 5s and 2m")
	}
	if cfg.HTTPMaxInflight < 32 || cfg.HTTPMaxInflight > 10000 {
		return Config{}, fmt.Errorf("VPNX3_HTTP_MAX_INFLIGHT must be between 32 and 10000")
	}
	if cfg.HTTPMaxHeaderBytes < 8*1024 || cfg.HTTPMaxHeaderBytes > 128*1024 {
		return Config{}, fmt.Errorf("VPNX3_HTTP_MAX_HEADER_KB must be between 8 and 128")
	}
	if cfg.HTTPDefaultBodyBytes < 16*1024 || cfg.HTTPDefaultBodyBytes > 8*1024*1024 {
		return Config{}, fmt.Errorf("VPNX3_HTTP_DEFAULT_BODY_KB must be between 16 and 8192")
	}
	if cfg.AdminSessionTTL < 15*time.Minute || cfg.AdminSessionTTL > 7*24*time.Hour {
		return Config{}, fmt.Errorf("VPNX3_ADMIN_SESSION_TTL must be between 15m and 168h")
	}
	if cfg.AdminSessionBinding!="off" && cfg.AdminSessionBinding!="user-agent" && cfg.AdminSessionBinding!="network" {
		return Config{},fmt.Errorf("VPNX3_ADMIN_SESSION_BINDING must be off, user-agent or network")
	}
	if cfg.Environment=="production" {
		if cfg.LogLevel==slog.LevelDebug {
			return Config{},fmt.Errorf("VPNX3_LOG_LEVEL=debug is not allowed in production")
		}
		if cfg.AdminSessionTTL>24*time.Hour {
			return Config{},fmt.Errorf("production admin session TTL must not exceed 24h")
		}
		if cfg.AdminSessionBinding=="off" {
			return Config{},fmt.Errorf("production admin session binding must not be off")
		}
		if cfg.ReleaseSigningKey=="" {
			return Config{},fmt.Errorf("VPNX3_RELEASE_SIGNING_KEY must be set in production")
		}
		if cfg.TrustRootPublicKey=="" || cfg.TrustBundleFile=="" {
			return Config{},fmt.Errorf("production requires VPNX3_TRUST_ROOT_PUBLIC_KEY and VPNX3_TRUST_BUNDLE_FILE")
		}
		if !trustConfigured{
			return Config{},fmt.Errorf("production requires offline trust root public key and signed trust bundle")
		}
		if err:=validateProductionDatabaseTransport(cfg.DatabaseURL);err!=nil{return Config{},err}
		if cfg.DatabaseReplicaURL!="" {
			if err:=validateProductionDatabaseTransport(cfg.DatabaseReplicaURL);err!=nil{
				return Config{},fmt.Errorf("VPNX3_DATABASE_REPLICA_URL: %w",err)
			}
		}
		if cfg.DatabaseHARequired && cfg.DatabaseReplicaURL=="" {
			return Config{},fmt.Errorf("VPNX3_DATABASE_HA_REQUIRED=true requires VPNX3_DATABASE_REPLICA_URL")
		}
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
	if cfg.BackupStatusFile!="" && !strings.HasPrefix(cfg.BackupStatusFile,"/") {
		return Config{}, fmt.Errorf("VPNX3_BACKUP_STATUS_FILE must be an absolute path")
	}
	if cfg.OffsiteBackupStatusFile!="" && !strings.HasPrefix(cfg.OffsiteBackupStatusFile,"/") {
		return Config{}, fmt.Errorf("VPNX3_BACKUP_OFFSITE_STATUS_FILE must be an absolute path")
	}
	if cfg.WalOffsiteStatusFile!="" && !strings.HasPrefix(cfg.WalOffsiteStatusFile,"/") {
		return Config{}, fmt.Errorf("VPNX3_WAL_OFFSITE_STATUS_FILE must be an absolute path")
	}
	securityExportConfigured:=cfg.SecurityExportURL!=""||cfg.SecurityExportSigningKey!=""
	if securityExportConfigured {
		if cfg.SecurityExportURL==""||cfg.SecurityExportSigningKey=="" {
			return Config{},fmt.Errorf("security export requires URL and signing key")
		}
		u,err:=url.Parse(cfg.SecurityExportURL)
		if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil {
			return Config{},fmt.Errorf("VPNX3_SECURITY_EXPORT_URL must be a credential-free https URL")
		}
		seed,err:=base64.RawURLEncoding.DecodeString(cfg.SecurityExportSigningKey)
		if err!=nil||len(seed)!=ed25519.SeedSize {
			return Config{},fmt.Errorf("VPNX3_SECURITY_EXPORT_SIGNING_KEY must be a 32-byte Ed25519 seed in base64url without padding")
		}
		if cfg.SecurityExportSigningKey==cfg.ConfigSigningKey||cfg.SecurityExportSigningKey==cfg.AccessSigningKey||cfg.SecurityExportSigningKey==cfg.ReleaseSigningKey {
			return Config{},fmt.Errorf("security export signing key must be independent")
		}
	}
	alertConfigured:=cfg.AlertWebhookURL!=""||cfg.AlertWebhookSecret!=""
	if alertConfigured{
		if cfg.AlertWebhookURL==""||len(cfg.AlertWebhookSecret)<32{
			return Config{},fmt.Errorf("incident alert webhook requires https URL and secret of at least 32 characters")
		}
		u,err:=url.Parse(cfg.AlertWebhookURL)
		if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil{
			return Config{},fmt.Errorf("VPNX3_ALERT_WEBHOOK_URL must be a credential-free https URL")
		}
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

func boolEnv(key string,fallback bool) bool {
	raw:=strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw==""{return fallback}
	switch raw{
	case "1","true","yes","on":return true
	case "0","false","no","off":return false
	default:return fallback
	}
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


func validateSigningSeeds(configSeed,accessSeed,releaseSeed string) error {
	type namedSeed struct{ name,raw string }
	inputs:=[]namedSeed{
		{"VPNX3_CONFIG_SIGNING_KEY",strings.TrimSpace(configSeed)},
		{"VPNX3_ACCESS_SIGNING_KEY",strings.TrimSpace(accessSeed)},
	}
	if strings.TrimSpace(releaseSeed)!=""{
		inputs=append(inputs,namedSeed{"VPNX3_RELEASE_SIGNING_KEY",strings.TrimSpace(releaseSeed)})
	}
	decoded:=make([][]byte,0,len(inputs))
	for _,item:=range inputs{
		seed,err:=base64.RawURLEncoding.DecodeString(item.raw)
		if err!=nil{
			return fmt.Errorf("%s must be base64url without padding: %w",item.name,err)
		}
		if len(seed)!=ed25519.SeedSize{
			return fmt.Errorf("%s must contain exactly %d bytes",item.name,ed25519.SeedSize)
		}
		for i,previous:=range decoded{
			if string(previous)==string(seed){
				return fmt.Errorf("%s must differ from %s",item.name,inputs[i].name)
			}
		}
		decoded=append(decoded,seed)
	}
	return nil
}


func validateProductionDatabaseTransport(raw string) error {
	u,err:=url.Parse(strings.TrimSpace(raw))
	if err!=nil||u.Scheme==""||u.Hostname()==""{
		return fmt.Errorf("VPNX3_DATABASE_URL must be a valid postgres URL in production")
	}
	host:=strings.ToLower(u.Hostname())
	local:=host=="localhost"||host=="127.0.0.1"||host=="::1"
	if local{return nil}
	mode:=strings.ToLower(strings.TrimSpace(u.Query().Get("sslmode")))
	switch mode{
	case "require","verify-ca","verify-full":
		return nil
	default:
		return fmt.Errorf("remote production PostgreSQL requires sslmode=require, verify-ca or verify-full")
	}
}
