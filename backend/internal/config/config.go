// Package config parses service configuration from environment variables
// per docs/ARCHITECTURE.md §10.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// devPlaceholderSecret is the throwaway JWT secret shipped in docker-compose
// and .env.example for zero-friction local boot. It is refused in production.
const devPlaceholderSecret = "dev-secret-change-me-please-0123456789abcdef"

// Config holds every setting the Discurd services read from the environment.
type Config struct {
	Environment    string // "development" (default) | "production"
	Port           string
	ServiceName    string
	ScyllaHosts    []string
	ScyllaKeyspace string
	RedisAddr      string
	NATSURL        string

	MinioEndpoint string
	MinioUser     string
	MinioPassword string
	MinioUseSSL   bool

	LiveKitAPIKey    string
	LiveKitAPISecret string
	LiveKitWSURL     string

	GiphyAPIKey    string
	TenorAPIKey    string
	TenorClientKey string

	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	MaxUploadMB int
	CORSOrigins []string
	LogLevel    string
}

// Load reads the environment, applying the contract defaults. The only
// required variable is JWT_SECRET.
func Load(defaultServiceName string) (*Config, error) {
	cfg := &Config{
		Environment:      getenv("ENV", "development"),
		Port:             getenv("PORT", "8080"),
		ServiceName:      getenv("SERVICE_NAME", defaultServiceName),
		ScyllaHosts:      splitCSV(getenv("SCYLLA_HOSTS", "scylla")),
		ScyllaKeyspace:   getenv("SCYLLA_KEYSPACE", "discurd"),
		RedisAddr:        getenv("REDIS_ADDR", "redis:6379"),
		NATSURL:          getenv("NATS_URL", "nats://nats:4222"),
		MinioEndpoint:    getenv("MINIO_ENDPOINT", "minio:9000"),
		MinioUser:        getenv("MINIO_ROOT_USER", "minioadmin"),
		MinioPassword:    getenv("MINIO_ROOT_PASSWORD", "minioadmin"),
		LiveKitAPIKey:    getenv("LIVEKIT_API_KEY", "devkey"),
		LiveKitAPISecret: getenv("LIVEKIT_API_SECRET", "devsecret_change_me_please_0123456789abcdef"),
		LiveKitWSURL:     getenv("LIVEKIT_WS_URL", "ws://localhost/livekit"),
		GiphyAPIKey:      getenv("GIPHY_API_KEY", ""),
		TenorAPIKey:      getenv("TENOR_API_KEY", ""),
		TenorClientKey:   getenv("TENOR_CLIENT_KEY", "discurd"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		LogLevel:         getenv("LOG_LEVEL", "info"),
		CORSOrigins:      splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173")),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	// In production, refuse to boot with a weak or built-in signing secret —
	// a guessable secret lets anyone forge access tokens for any account.
	if cfg.Environment == "production" {
		if cfg.JWTSecret == devPlaceholderSecret {
			return nil, fmt.Errorf("refusing to start in production with the built-in development JWT_SECRET; set a unique value (e.g. `openssl rand -hex 32`)")
		}
		if len(cfg.JWTSecret) < 32 {
			return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters in production")
		}
	}

	var err error
	if cfg.MinioUseSSL, err = strconv.ParseBool(getenv("MINIO_USE_SSL", "false")); err != nil {
		return nil, fmt.Errorf("MINIO_USE_SSL: %w", err)
	}
	if cfg.AccessTokenTTL, err = time.ParseDuration(getenv("ACCESS_TOKEN_TTL", "15m")); err != nil {
		return nil, fmt.Errorf("ACCESS_TOKEN_TTL: %w", err)
	}
	if cfg.RefreshTokenTTL, err = time.ParseDuration(getenv("REFRESH_TOKEN_TTL", "168h")); err != nil {
		return nil, fmt.Errorf("REFRESH_TOKEN_TTL: %w", err)
	}
	if cfg.MaxUploadMB, err = strconv.Atoi(getenv("MAX_UPLOAD_MB", "25")); err != nil {
		return nil, fmt.Errorf("MAX_UPLOAD_MB: %w", err)
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
