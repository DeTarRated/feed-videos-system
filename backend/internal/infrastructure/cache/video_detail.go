package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type VideoDetailCache struct {
	client *goredis.Client
	ttl    time.Duration
	group  singleflight.Group
}

func NewVideoDetailCache(client *goredis.Client, ttl time.Duration) *VideoDetailCache {
	return &VideoDetailCache{client: client, ttl: ttl}
}

func (c *VideoDetailCache) Do(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	value, err, _ := c.group.Do(key, func() (interface{}, error) {
		return fn(ctx)
	})
	if err != nil {
		return nil, err
	}
	return value, nil
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
	ttl := c.ttl + time.Duration(rand.Intn(30))*time.Second
	return c.client.Set(ctx, VideoDetailKey(videoID), data, ttl).Err()
}

func (c *VideoDetailCache) GetMany(ctx context.Context, videoIDs []uint) (map[uint][]byte, []uint, error) {
	if c == nil || c.client == nil || len(videoIDs) == 0 {
		return nil, videoIDs, nil
	}
	keys := make([]string, 0, len(videoIDs))
	for _, id := range videoIDs {
		keys = append(keys, VideoDetailKey(id))
	}
	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, videoIDs, err
	}
	hit := make(map[uint][]byte, len(videoIDs))
	missing := make([]uint, 0)
	for i, raw := range values {
		if raw == nil {
			missing = append(missing, videoIDs[i])
			continue
		}
		hit[videoIDs[i]] = []byte(fmt.Sprintf("%v", raw))
	}
	return hit, missing, nil
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

func VideoDetailEmptyKey(videoID uint) string {
	return fmt.Sprintf("video:detail:%d:empty", videoID)
}

func (c *VideoDetailCache) SetEmpty(ctx context.Context, videoID uint) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Set(ctx, VideoDetailEmptyKey(videoID), "1", 30*time.Second).Err()
}

func (c *VideoDetailCache) IsEmpty(ctx context.Context, videoID uint) (bool, error) {
	if c == nil || c.client == nil {
		return false, nil
	}
	exists, err := c.client.Exists(ctx, VideoDetailEmptyKey(videoID)).Result()
	return exists > 0, err
}
