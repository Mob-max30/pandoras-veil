// Package config loads server configuration from environment variables.
//
// Owner: Pavan (Backend Relay Lead) — server/
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the relay server.
type Config struct {
	// Port the HTTP server listens on.
	Port string

	// RedisAddr is host:port for the Redis instance.
	RedisAddr string
	// RedisPassword for Redis AUTH (empty if unauthenticated).
	RedisPassword string
	// RedisDB selects the Redis logical database.
	RedisDB int

	// DefaultTTL is used when a client omits ttl_seconds on POST /paste.
	DefaultTTL time.Duration
	// MaxTTL is the hard ceiling on any requested TTL, to keep the relay
	// from being used as long-term storage.
	MaxTTL time.Duration
	// MinTTL is the floor on requested TTL.
	MinTTL time.Duration

	// MaxCiphertextBytes bounds the size of an uploaded paste (post base64
	// decode) to prevent abuse of the relay as a blob store.
	MaxCiphertextBytes int64

	// ShutdownGracePeriod bounds how long the server waits for in-flight
	// requests to finish during shutdown.
	ShutdownGracePeriod time.Duration
}

// Load reads configuration from the environment, applying sane defaults for
// anything unset. It never fails — every var has a workable default so the
// server can boot with zero configuration for local/demo use.
func Load() Config {
	return Config{
		Port:                envOr("PANDORA_PORT", "8080"),
		RedisAddr:           envOr("PANDORA_REDIS_ADDR", "localhost:6379"),
		RedisPassword:       envOr("PANDORA_REDIS_PASSWORD", ""),
		RedisDB:             envIntOr("PANDORA_REDIS_DB", 0),
		DefaultTTL:          envDurationOr("PANDORA_DEFAULT_TTL_SECONDS", 15*time.Minute),
		MaxTTL:              envDurationOr("PANDORA_MAX_TTL_SECONDS", 7*24*time.Hour),
		MinTTL:              envDurationOr("PANDORA_MIN_TTL_SECONDS", 30*time.Second),
		MaxCiphertextBytes:  int64(envIntOr("PANDORA_MAX_CIPHERTEXT_BYTES", 2*1024*1024)), // 2 MiB
		ShutdownGracePeriod: envDurationOr("PANDORA_SHUTDOWN_GRACE_SECONDS", 10*time.Second),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// envDurationOr reads an integer number of seconds from the environment and
// returns it as a time.Duration.
func envDurationOr(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Second
}
