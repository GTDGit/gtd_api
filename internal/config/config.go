package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
// It is the single source of truth for runtime parameters.
type Config struct {
	Port      string
	Env       string
	JWTSecret string

	DB        DatabaseConfig
	Redis     RedisConfig
	Digiflazz DigiflazzConfig
	Worker    WorkerConfig
}

// DatabaseConfig contains PostgreSQL connection parameters.
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// RedisConfig contains Redis connection parameters.
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// DigiflazzConfig contains credentials and secrets for Digiflazz integration.
type DigiflazzConfig struct {
	Username       string
	KeyProduction  string
	KeyDevelopment string
	WebhookSecret  string
}

// WorkerConfig contains interval configuration for background workers.
type WorkerConfig struct {
	SyncInterval     time.Duration
	RetryInterval    time.Duration
	CallbackInterval time.Duration
}

// Load reads configuration from environment variables. If a .env file exists
// in the working directory, it will be loaded first. It returns a populated
// Config or an error with a human-friendly message.
func Load() (*Config, error) {
	// Load .env if present; ignore error if file is missing so that production
	// environments relying solely on real environment variables keep working.
	_ = godotenv.Load()

	cfg := &Config{}

	// Server
	cfg.Port = getEnv("PORT", "8080")
	cfg.Env = getEnv("ENV", "development")
	cfg.JWTSecret = getEnv("JWT_SECRET", "")

	// Database
	cfg.DB = DatabaseConfig{
		Host:     getEnv("DB_HOST", ""),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", ""),
		Password: getEnv("DB_PASSWORD", ""),
		Name:     getEnv("DB_NAME", ""),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}

	// Redis
	cfg.Redis = RedisConfig{
		Host:     getEnv("REDIS_HOST", "redis"),
		Port:     getEnv("REDIS_PORT", "6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       getEnvInt("REDIS_DB", 0),
	}

	// Digiflazz
	cfg.Digiflazz = DigiflazzConfig{
		Username:       getEnv("DIGIFLAZZ_USERNAME", ""),
		KeyProduction:  getEnv("DIGIFLAZZ_KEY_PRODUCTION", ""),
		KeyDevelopment: getEnv("DIGIFLAZZ_KEY_DEVELOPMENT", ""),
		WebhookSecret:  getEnv("DIGIFLAZZ_WEBHOOK_SECRET", ""),
	}

	// Workers (durations)
	var err error
	if cfg.Worker.SyncInterval, err = parseDurationEnv("SYNC_INTERVAL", "15m"); err != nil {
		return nil, fmt.Errorf("invalid SYNC_INTERVAL: %w", err)
	}
	if cfg.Worker.RetryInterval, err = parseDurationEnv("RETRY_INTERVAL", "15m"); err != nil {
		return nil, fmt.Errorf("invalid RETRY_INTERVAL: %w", err)
	}
	if cfg.Worker.CallbackInterval, err = parseDurationEnv("CALLBACK_RETRY_INTERVAL", "1m"); err != nil {
		return nil, fmt.Errorf("invalid CALLBACK_RETRY_INTERVAL: %w", err)
	}

	// Basic validation for DB parameters — keeps messages concise and helpful.
	if cfg.DB.Host == "" || cfg.DB.User == "" || cfg.DB.Name == "" {
		return nil, errors.New("database configuration incomplete: ensure DB_HOST, DB_USER, and DB_NAME are set")
	}

	// Validate JWT_SECRET
	if cfg.JWTSecret == "" {
		return nil, errors.New("JWT_SECRET must be set for authentication")
	}

	return cfg, nil
}

// getEnv returns the value of an environment variable or a default if empty.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getEnvInt returns the value of an environment variable as an integer or a default if empty/invalid.
func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

// parseDurationEnv reads an environment variable and parses it as time.Duration.
// If the variable is empty, it falls back to the provided default value.
func parseDurationEnv(key, def string) (time.Duration, error) {
	raw := getEnv(key, def)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration must be >= 0")
	}
	return d, nil
}
