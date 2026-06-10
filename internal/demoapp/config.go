package demoapp

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Addr           string
	AdminToken     string
	TempDir        string
	MaxMemoryBytes int64
}

func DefaultConfig() Config {
	return Config{
		Addr:           envString("DEMO_API_ADDR", "127.0.0.1:8080"),
		AdminToken:     os.Getenv("DEMO_ADMIN_TOKEN"),
		TempDir:        envString("DEMO_TEMP_DIR", filepath.Join(os.TempDir(), "prometheus-observability-demo")),
		MaxMemoryBytes: envInt64("DEMO_MAX_MEMORY_MB", 48) * 1024 * 1024,
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
