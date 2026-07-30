package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type VideoDetailCache struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewVideoDetailCache(client *goredis.Client, ttl time.Duration) *VideoDetailCache {
	return &VideoDetailCache{client: client, ttl: ttl}
}

func (c *VideoDetailCache) Get(ctx context.Context, videoID uint, dest any) (bool, error) {
	if c == nil || c.client == nil {
		return false, nil
	}

	value, err := c.client.Get(ctx, VideoDetailKey(videoID)).Result()
	if err != nil {
		if err == goredis.Nil {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal([]byte(value), dest); err != nil {
		return false, err
	}
	return true, nil
}

func (c *VideoDetailCache) Set(ctx context.Context, videoID uint, value any) error {
	if c == nil || c.client == nil {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, VideoDetailKey(videoID), data, c.ttl).Err()
}

func (c *VideoDetailCache) Delete(ctx context.Context, videoID uint) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Del(ctx, VideoDetailKey(videoID)).Err()
}

func VideoDetailKey(videoID uint) string {
	return fmt.Sprintf("video:detail:%d", videoID)
}
