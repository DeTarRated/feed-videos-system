package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv                     string
	HTTPAddr                   string
	MySQLDSN                   string
	StorageDir                 string
	JWTSecret                  string
	RedisAddr                  string
	RedisPassword              string
	RedisDB                    int
	VideoDetailCacheTTL        time.Duration
	RateLimitWindow            time.Duration
	RabbitMQURL                string
	RabbitMQExchange           string
	RabbitMQQueue              string
	RabbitMQRoutingKey         string
	RabbitMQRetryQueue         string
	RabbitMQDLQ                string
	RabbitMQCommandExchange    string
	RabbitMQLikeRoutingKey     string
	RabbitMQCommentRoutingKey  string
	RabbitMQLikeQueue          string
	RabbitMQCommentQueue       string
	RabbitMQCommandRetryQueue  string
	RabbitMQCommandDLQ         string
	RabbitMQCommandRetryTTL    time.Duration
	RabbitMQCommandPrefetch    int
	RabbitMQConfirmTimeout     time.Duration
	IdempotencyTTL             time.Duration
	InteractionMarkerTTL       time.Duration
	SchedulerLockTTL           time.Duration
	SchedulerLockRenewInterval time.Duration
	SchedulerCatchUpLimit      int
}

func Load() Config {
	return Config{
		AppEnv:                     getEnv("APP_ENV", "development"),
		HTTPAddr:                   getEnv("HTTP_ADDR", ":8080"),
		MySQLDSN:                   getEnv("MYSQL_DSN", "root:password@tcp(127.0.0.1:3306)/feed_videos?parseTime=true&charset=utf8mb4&loc=Local"),
		StorageDir:                 getEnv("STORAGE_DIR", "./storage"),
		JWTSecret:                  getEnv("JWT_SECRET", "dev-secret"),
		RedisAddr:                  getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:              getEnv("REDIS_PASSWORD", ""),
		RedisDB:                    getEnvInt("REDIS_DB", 0),
		VideoDetailCacheTTL:        getEnvDuration("VIDEO_DETAIL_CACHE_TTL", 10*time.Minute),
		RateLimitWindow:            getEnvDuration("RATE_LIMIT_WINDOW", time.Minute),
		RabbitMQURL:                getEnv("RABBITMQ_URL", "amqp://feed:feed@127.0.0.1:5672/"),
		RabbitMQExchange:           getEnv("RABBITMQ_EXCHANGE", "feed.events"),
		RabbitMQQueue:              getEnv("RABBITMQ_QUEUE", "feed.events"),
		RabbitMQRoutingKey:         getEnv("RABBITMQ_ROUTING_KEY", "feed.event"),
		RabbitMQRetryQueue:         getEnv("RABBITMQ_RETRY_QUEUE", "feed.events.retry"),
		RabbitMQDLQ:                getEnv("RABBITMQ_DEAD_LETTER_QUEUE", "feed.events.dlq"),
		RabbitMQCommandExchange:    getEnv("RABBITMQ_COMMAND_EXCHANGE", "feed.interaction.commands"),
		RabbitMQLikeRoutingKey:     getEnv("RABBITMQ_LIKE_ROUTING_KEY", "interaction.command.like"),
		RabbitMQCommentRoutingKey:  getEnv("RABBITMQ_COMMENT_ROUTING_KEY", "interaction.command.comment"),
		RabbitMQLikeQueue:          getEnv("RABBITMQ_LIKE_QUEUE", "interaction.command.like.queue"),
		RabbitMQCommentQueue:       getEnv("RABBITMQ_COMMENT_QUEUE", "interaction.command.comment.queue"),
		RabbitMQCommandRetryQueue:  getEnv("RABBITMQ_COMMAND_RETRY_QUEUE", "interaction.command.retry"),
		RabbitMQCommandDLQ:         getEnv("RABBITMQ_COMMAND_DLQ", "interaction.command.dlq"),
		RabbitMQCommandRetryTTL:    getEnvDuration("RABBITMQ_COMMAND_RETRY_TTL", 30*time.Second),
		RabbitMQCommandPrefetch:    getEnvInt("RABBITMQ_COMMAND_PREFETCH", 16),
		RabbitMQConfirmTimeout:     getEnvDuration("RABBITMQ_CONFIRM_TIMEOUT", 3*time.Second),
		IdempotencyTTL:             getEnvDuration("IDEMPOTENCY_TTL", 24*time.Hour),
		InteractionMarkerTTL:       getEnvDuration("INTERACTION_MARKER_TTL", 168*time.Hour),
		SchedulerLockTTL:           getEnvDuration("SCHEDULER_LOCK_TTL", 15*time.Second),
		SchedulerLockRenewInterval: getEnvDuration("SCHEDULER_LOCK_RENEW_INTERVAL", 5*time.Second),
		SchedulerCatchUpLimit:      getEnvInt("SCHEDULER_CATCH_UP_LIMIT", 10),
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
	if c.RabbitMQCommandExchange == "" {
		return fmt.Errorf("rabbitmq command exchange is required")
	}
	if c.RabbitMQLikeRoutingKey == "" {
		return fmt.Errorf("rabbitmq like routing key is required")
	}
	if c.RabbitMQCommentRoutingKey == "" {
		return fmt.Errorf("rabbitmq comment routing key is required")
	}
	if c.RabbitMQConfirmTimeout <= 0 {
		return fmt.Errorf("rabbitmq confirm timeout must be positive")
	}
	if c.IdempotencyTTL <= 0 {
		return fmt.Errorf("idempotency ttl must be positive")
	}
	if c.RabbitMQLikeQueue == "" {
		return fmt.Errorf("rabbitmq like queue is required")
	}
	if c.RabbitMQCommentQueue == "" {
		return fmt.Errorf("rabbitmq comment queue is required")
	}
	if c.InteractionMarkerTTL <= 0 {
		return fmt.Errorf("interaction marker ttl must be positive")
	}
	if c.RabbitMQCommandRetryQueue == "" {
		return fmt.Errorf("rabbitmq command retry queue is required")
	}
	if c.RabbitMQCommandDLQ == "" {
		return fmt.Errorf("rabbitmq command dlq is required")
	}
	if c.RabbitMQCommandRetryTTL <= 0 {
		return fmt.Errorf("rabbitmq command retry ttl must be positive")
	}
	if c.RabbitMQCommandPrefetch <= 0 {
		return fmt.Errorf("rabbitmq command prefetch must be positive")
	}
	if c.SchedulerLockTTL <= 0 {
		return fmt.Errorf("scheduler lock ttl must be positive")
	}
	if c.SchedulerLockRenewInterval <= 0 {
		return fmt.Errorf("scheduler lock renew interval must be positive")
	}
	if c.SchedulerCatchUpLimit <= 0 {
		return fmt.Errorf("scheduler catch up limit must be positive")
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
