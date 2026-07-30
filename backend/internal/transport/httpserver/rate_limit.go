package httpserver

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

type RateLimitRule struct {
	Name         string
	Limit        int64
	Window       time.Duration
	UseUserID    bool
	FallbackToIP bool
}

func RateLimit(client *goredis.Client, rule RateLimitRule) gin.HandlerFunc {
	return func(c *gin.Context) {
		if client == nil || rule.Limit <= 0 || rule.Window <= 0 {
			c.Next()
			return
		}

		identity := rateLimitIdentity(c, rule)
		key := fmt.Sprintf("rate_limit:%s:%s", rule.Name, identity)
		ctx := c.Request.Context()

		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			log.Printf("rate limit incr: %v", err)
			c.Next()
			return
		}
		if count == 1 {
			if err := client.Expire(ctx, key, rule.Window).Err(); err != nil {
				log.Printf("rate limit expire: %v", err)
			}
		}
		if count > rule.Limit {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func rateLimitIdentity(c *gin.Context, rule RateLimitRule) string {
	if rule.UseUserID {
		if userID, ok := CurrentUserID(c); ok {
			return fmt.Sprintf("user:%d", userID)
		}
		if !rule.FallbackToIP {
			return "user:anonymous"
		}
	}
	return "ip:" + sanitizeRateLimitPart(c.ClientIP())
}

func sanitizeRateLimitPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return strings.NewReplacer(":", "_", "/", "_").Replace(value)
}
