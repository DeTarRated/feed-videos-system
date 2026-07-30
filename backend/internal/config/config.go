package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv              string
	HTTPAddr            string
	MySQLDSN            string
	StorageDir          string
	JWTSecret           string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	VideoDetailCacheTTL time.Duration
	RateLimitWindow     time.Duration
	RabbitMQURL         string
	RabbitMQExchange    string
	RabbitMQQueue       string
	RabbitMQRoutingKey  string
	RabbitMQRetryQueue  string
	RabbitMQDLQ         string
}

func Load() Config {
	return Config{
		AppEnv:              getEnv("APP_ENV", "development"),
		HTTPAddr:            getEnv("HTTP_ADDR", ":8080"),
		MySQLDSN:            getEnv("MYSQL_DSN", "root:password@tcp(127.0.0.1:3306)/feed_videos?parseTime=true&charset=utf8mb4&loc=Local"),
		StorageDir:          getEnv("STORAGE_DIR", "./storage"),
		JWTSecret:           getEnv("JWT_SECRET", "dev-secret"),
		RedisAddr:           getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:       getEnv("REDIS_PASSWORD", ""),
		RedisDB:             getEnvInt("REDIS_DB", 0),
		VideoDetailCacheTTL: getEnvDuration("VIDEO_DETAIL_CACHE_TTL", 10*time.Minute),
		RateLimitWindow:     getEnvDuration("RATE_LIMIT_WINDOW", time.Minute),
		RabbitMQURL:         getEnv("RABBITMQ_URL", "amqp://feed:feed@127.0.0.1:5672/"),
		RabbitMQExchange:    getEnv("RABBITMQ_EXCHANGE", "feed.events"),
		RabbitMQQueue:       getEnv("RABBITMQ_QUEUE", "feed.events"),
		RabbitMQRoutingKey:  getEnv("RABBITMQ_ROUTING_KEY", "feed.event"),
		RabbitMQRetryQueue:  getEnv("RABBITMQ_RETRY_QUEUE", "feed.events.retry"),
		RabbitMQDLQ:         getEnv("RABBITMQ_DEAD_LETTER_QUEUE", "feed.events.dlq"),
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
	if c.RedisAddr == "" {
		return fmt.Errorf("redis addr is required")
	}
	if c.VideoDetailCacheTTL <= 0 {
		return fmt.Errorf("video detail cache ttl must be positive")
	}
	if c.RateLimitWindow <= 0 {
		return fmt.Errorf("rate limit window must be positive")
	}
	if c.RabbitMQURL == "" {
		return fmt.Errorf("rabbitmq url is required")
	}
	if c.RabbitMQExchange == "" {
		return fmt.Errorf("rabbitmq exchange is required")
	}
	if c.RabbitMQQueue == "" {
		return fmt.Errorf("rabbitmq queue is required")
	}
	if c.RabbitMQRoutingKey == "" {
		return fmt.Errorf("rabbitmq routing key is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
