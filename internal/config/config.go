package config

import (
    "fmt"
    "os"

    "github.com/joho/godotenv"
)

type Config struct {
    Port           string
    Environment    string // NEW: "development" or "production"
    RedisURL       string
    GridAPIKey     string
    DatabaseURL    string
    TrustedProxies string
}

func Load() (*Config, error) {
    // Load .env file (OK if it fails in production)
    if err := godotenv.Load(); err != nil {
        fmt.Printf("Warning: .env file not found: %v\n", err)
    }

    cfg := &Config{
        Port:           getEnv("PORT", "8080"),
        Environment:    getEnv("ENVIRONMENT", "development"),
        RedisURL:       os.Getenv("REDIS_URL"),
        GridAPIKey:     os.Getenv("GRID_API_KEY"),
        DatabaseURL:    os.Getenv("DATABASE_URL"),
        TrustedProxies: os.Getenv("TRUSTED_PROXIES"),
    }

    // Validate required fields
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

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}