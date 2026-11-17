package cache

import (
	"context"
	"fmt"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

// LikeStateCache exposes the online like state written by the interaction
// replay script. Persistence is handled by the command worker's MySQL
// transaction, so there is no dirty-set / batch sync path anymore.
type LikeStateCache struct {
	client *goredis.Client
}

func NewLikeStateCache(client *goredis.Client) *LikeStateCache {
	return &LikeStateCache{client: client}
}

// LikeUserState returns the cached like state and its operation version.
// A missing hash returns an error so callers fall back to MySQL.
func (c *LikeStateCache) LikeUserState(ctx context.Context, videoID, userID uint) (liked bool, opVersion uint64, err error) {
	if c == nil || c.client == nil {
		return false, 0, fmt.Errorf("like state cache unavailable")
	}
	values, err := c.client.HMGet(ctx, LikeUserStateKey(videoID, userID), "liked", "op_version").Result()
	if err != nil {
		return false, 0, err
	}
	if values[0] == nil {
		return false, 0, fmt.Errorf("like user state not found")
	}
	liked = values[0].(string) == "1"
	if values[1] != nil {
		opVersion, _ = strconv.ParseUint(values[1].(string), 10, 64)
	}
	return liked, opVersion, nil
}

func (c *LikeStateCache) GetLikeCount(ctx context.Context, videoID uint) (int64, error) {
	if c == nil || c.client == nil {
		return 0, fmt.Errorf("like state cache unavailable")
	}
	return c.client.HGet(ctx, LikeVideoStatsKey(videoID), "like_count").Int64()
}

func LikeUserStateKey(videoID, userID uint) string {
	return fmt.Sprintf("like:state:%d:%d", videoID, userID)
}

func LikeVideoStatsKey(videoID uint) string {
	return fmt.Sprintf("video:stats:%d", videoID)
}
