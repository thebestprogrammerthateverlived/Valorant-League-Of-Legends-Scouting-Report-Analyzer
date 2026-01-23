package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	RedisURL       string
	GridAPIKey     string
	DatabaseURL    string
	TrustedProxies string
}

func Load() (*Config, error) {
	// Load from the.env file
	if err := godotenv.Load(); err != nil {
		// It's okay if .env is missing in production, as long as env vars are set
		fmt.Printf("Warning: .env file not found: %v\n", err)
	}

	cfg := &Config{
		Port:           os.Getenv("PORT"),
		RedisURL:       os.Getenv("REDIS_URL"),
		GridAPIKey:     os.Getenv("GRID_API_KEY"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		TrustedProxies: os.Getenv("TRUSTED_PROXIES"),
	}

	// Validate all required fields present
	if cfg.Port == "" {
		return nil, fmt.Errorf("PORT environment variable is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL environment variable is required")
	}
	if cfg.GridAPIKey == "" {
		return nil, fmt.Errorf("GRID_API_KEY environment variable is required")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	return cfg, nil
}
