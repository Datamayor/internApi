package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	DatabaseURL string

	JWTSecret             string
	JWTExpiryHours        int
	JWTRefreshExpiryHours int

	ServerPort string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	jwtExpiry, _ := strconv.Atoi(getEnv("JWT_EXPIRY_HOURS", "24"))
	jwtRefresh, _ := strconv.Atoi(getEnv("JWT_REFRESH_EXPIRY_HOURS", "168"))

	cfg := &Config{
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		DBHost:                getEnv("DB_HOST", "localhost"),
		DBPort:                getEnv("DB_PORT", "5432"),
		DBUser:                getEnv("DB_USER", "postgres"),
		DBPassword:            getEnv("DB_PASSWORD", ""),
		DBName:                getEnv("DB_NAME", "intern_db"),
		DBSSLMode:             getEnv("DB_SSLMODE", "disable"),
		JWTSecret:             getEnv("JWT_SECRET", ""),
		JWTExpiryHours:        jwtExpiry,
		JWTRefreshExpiryHours: jwtRefresh,
		ServerPort:            getEnv("PORT", getEnv("SERVER_PORT", "8080")),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET must be set in .env")
	}

	return cfg, nil
}

func (c *Config) DBConnectionString() string {
	// If DATABASE_URL is set (Render), use it directly
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	// Otherwise use individual fields (local)
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}