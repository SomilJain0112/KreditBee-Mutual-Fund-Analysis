package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	MFAPI    MFAPIConfig
	Sync     SyncConfig
}

type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type DatabaseConfig struct {
	Path            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type MFAPIConfig struct {
	BaseURL         string
	RequestsPerSec  int
	RequestsPerMin  int
	RequestsPerHour int
	BlockDuration   time.Duration
	Timeout         time.Duration
}

type SyncConfig struct {
	BackfillBatchSize int
	RetryAttempts     int
	RetryDelay        time.Duration
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            getEnv("SERVER_HOST", "0.0.0.0"),
			Port:            getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:     getEnvDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getEnvDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: getEnvDuration("SERVER_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Database: DatabaseConfig{
			Path:            getEnv("DATABASE_PATH", "data/mf_analytics.db"),
			MaxOpenConns:    getEnvInt("DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		MFAPI: MFAPIConfig{
			BaseURL:         getEnv("MFAPI_BASE_URL", "https://api.mfapi.in"),
			RequestsPerSec:  getEnvInt("MFAPI_REQUESTS_PER_SEC", 2),
			RequestsPerMin:  getEnvInt("MFAPI_REQUESTS_PER_MIN", 50),
			RequestsPerHour: getEnvInt("MFAPI_REQUESTS_PER_HOUR", 300),
			BlockDuration:   getEnvDuration("MFAPI_BLOCK_DURATION", 5*time.Minute),
			Timeout:         getEnvDuration("MFAPI_TIMEOUT", 10*time.Second),
		},
		Sync: SyncConfig{
			BackfillBatchSize: getEnvInt("SYNC_BACKFILL_BATCH_SIZE", 10),
			RetryAttempts:     getEnvInt("SYNC_RETRY_ATTEMPTS", 3),
			RetryDelay:        getEnvDuration("SYNC_RETRY_DELAY", 5*time.Second),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
