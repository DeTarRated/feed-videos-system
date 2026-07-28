package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppEnv     string
	HTTPAddr   string
	MySQLDSN   string
	StorageDir string
	JWTSecret  string
}

func Load() Config {
	return Config{
		AppEnv:     getEnv("APP_ENV", "development"),
		HTTPAddr:   getEnv("HTTP_ADDR", ":8080"),
		MySQLDSN:   getEnv("MYSQL_DSN", "root:password@tcp(127.0.0.1:3306)/feed_videos?parseTime=true&charset=utf8mb4&loc=Local"),
		StorageDir: getEnv("STORAGE_DIR", "./storage"),
		JWTSecret:  getEnv("JWT_SECRET", "dev-secret"),
	}
}

func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("http addr is required")
	}
	if c.MySQLDSN == "" {
		return fmt.Errorf("mysql dsn is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("jwt secret is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
