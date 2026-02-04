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
	Identity  IdentityConfig
	S3        S3Config
	AWS       AWSConfig     `mapstructure:"aws"`
	Tencent   TencentConfig `mapstructure:"tencent"`
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
	SyncInterval              time.Duration
	RetryInterval             time.Duration
	CallbackInterval          time.Duration
	DigiflazzCallbackInterval time.Duration
	StatusCheckInterval       time.Duration
	StatusCheckStaleAfter     time.Duration
	StatusCheckMaxAge         time.Duration
}

// IdentityConfig contains configuration for Identity OCR services
type IdentityConfig struct {
	GoogleCredentialsPath string
	GoogleProjectID       string
	GroqAPIKey            string
	GroqModel             string
}

// S3Config contains AWS S3 configuration
type S3Config struct {
	Region          string
	Bucket          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
}

// AWSConfig contains AWS general configuration
type AWSConfig struct {
	AccessKeyID       string
	SecretAccessKey   string
	LivenessRegion    string // ap-northeast-1 (Tokyo)
	RekognitionRegion string // ap-southeast-1 (Singapore)
}

// TencentConfig contains Tencent Cloud FaceID configuration
type TencentConfig struct {
	SecretID  string
	SecretKey string
	Region    string // ap-jakarta or others
	RuleID    string // "1" for default
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

	// Identity (Google Vision & Groq)
	cfg.Identity = IdentityConfig{
		GoogleCredentialsPath: getEnv("GOOGLE_APPLICATION_CREDENTIALS", "keys/google/identity.json"),
		GoogleProjectID:       getEnv("GOOGLE_PROJECT_ID", ""),
		GroqAPIKey:            getEnv("GROQ_API_KEY", ""),
		GroqModel:             getEnv("GROQ_MODEL", "llama-3.1-8b-instant"),
	}

	// S3 (AWS Jakarta region)
	cfg.S3 = S3Config{
		Region:          getEnv("S3_REGION", "ap-southeast-3"),
		Bucket:          getEnv("S3_BUCKET", "gerbang-identity"),
		Endpoint:        getEnv("S3_ENDPOINT", "https://s3.ap-southeast-3.amazonaws.com"),
		AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", ""),
		SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
	}

	// AWS General (Liveness & Rekognition)
	cfg.AWS = AWSConfig{
		AccessKeyID:       getEnv("AWS_ACCESS_KEY_ID", ""),
		SecretAccessKey:   getEnv("AWS_SECRET_ACCESS_KEY", ""),
		LivenessRegion:    getEnv("AWS_LIVENESS_REGION", "ap-northeast-1"),
		RekognitionRegion: getEnv("AWS_REKOGNITION_REGION", "ap-southeast-1"),
	}

	// Tencent Cloud (FaceID)
	cfg.Tencent = TencentConfig{
		SecretID:  getEnv("TENCENT_SECRET_ID", ""),
		SecretKey: getEnv("TENCENT_SECRET_KEY", ""),
		Region:    getEnv("TENCENT_REGION", "ap-jakarta"),
		RuleID:    getEnv("TENCENT_RULE_ID", "1"),
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
	if cfg.Worker.DigiflazzCallbackInterval, err = parseDurationEnv("DIGIFLAZZ_CALLBACK_INTERVAL", "30s"); err != nil {
		return nil, fmt.Errorf("invalid DIGIFLAZZ_CALLBACK_INTERVAL: %w", err)
	}
	if cfg.Worker.StatusCheckInterval, err = parseDurationEnv("STATUS_CHECK_INTERVAL", "10s"); err != nil {
		return nil, fmt.Errorf("invalid STATUS_CHECK_INTERVAL: %w", err)
	}
	if cfg.Worker.StatusCheckStaleAfter, err = parseDurationEnv("STATUS_CHECK_STALE_AFTER", "10s"); err != nil {
		return nil, fmt.Errorf("invalid STATUS_CHECK_STALE_AFTER: %w", err)
	}
	if cfg.Worker.StatusCheckMaxAge, err = parseDurationEnv("STATUS_CHECK_MAX_AGE", "5m"); err != nil {
		return nil, fmt.Errorf("invalid STATUS_CHECK_MAX_AGE: %w", err)
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
