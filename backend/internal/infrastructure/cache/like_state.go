package cache

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type LikeStateCache struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewLikeStateCache(client *goredis.Client, ttl time.Duration) *LikeStateCache {
	return &LikeStateCache{client: client, ttl: ttl}
}

func (c *LikeStateCache) AddVideoUser(ctx context.Context, videoID, userID uint) (bool, error) {
	if c == nil || c.client == nil {
		return false, nil
	}
	added, err := c.client.SAdd(ctx, LikeVideoUsersKey(videoID), userID).Result()
	if err != nil {
		return false, err
	}
	if c.ttl > 0 {
		_ = c.client.Expire(ctx, LikeVideoUsersKey(videoID), c.ttl).Err()
	}
	return added > 0, nil
}

func (c *LikeStateCache) RemoveVideoUser(ctx context.Context, videoID, userID uint) (bool, error) {
	if c == nil || c.client == nil {
		return false, nil
	}
	removed, err := c.client.SRem(ctx, LikeVideoUsersKey(videoID), userID).Result()
	if err != nil {
		return false, err
	}
	if c.ttl > 0 {
		_ = c.client.Expire(ctx, LikeVideoUsersKey(videoID), c.ttl).Err()
	}
	return removed > 0, nil
}

func (c *LikeStateCache) AddUserVideo(ctx context.Context, userID, videoID uint) (bool, error) {
	if c == nil || c.client == nil {
		return false, nil
	}
	added, err := c.client.SAdd(ctx, LikeUserVideosKey(userID), videoID).Result()
	if err != nil {
		return false, err
	}
	if c.ttl > 0 {
		_ = c.client.Expire(ctx, LikeUserVideosKey(userID), c.ttl).Err()
	}
	return added > 0, nil
}

func (c *LikeStateCache) RemoveUserVideo(ctx context.Context, userID, videoID uint) (bool, error) {
	if c == nil || c.client == nil {
		return false, nil
	}
	removed, err := c.client.SRem(ctx, LikeUserVideosKey(userID), videoID).Result()
	if err != nil {
		return false, err
	}
	if c.ttl > 0 {
		_ = c.client.Expire(ctx, LikeUserVideosKey(userID), c.ttl).Err()
	}
	return removed > 0, nil
}

func (c *LikeStateCache) IncrementLikeCount(ctx context.Context, videoID uint, delta int64) (int64, error) {
	if c == nil || c.client == nil {
		return 0, nil
	}
	count, err := c.client.HIncrBy(ctx, LikeVideoStatsKey(videoID), "like_count", delta).Result()
	if err != nil {
		return 0, err
	}
	if c.ttl > 0 {
		_ = c.client.Expire(ctx, LikeVideoStatsKey(videoID), c.ttl).Err()
	}
	return count, nil
}

func (c *LikeStateCache) MarkDirty(ctx context.Context, videoID uint) error {
	if c == nil || c.client == nil {
		return nil
	}
	if err := c.client.SAdd(ctx, LikeDirtyVideosKey(), videoID).Err(); err != nil {
		return err
	}
	if c.ttl > 0 {
		_ = c.client.Expire(ctx, LikeDirtyVideosKey(), c.ttl).Err()
	}
	return nil
}

func (c *LikeStateCache) GetLikeCount(ctx context.Context, videoID uint) (int64, error) {
	if c == nil || c.client == nil {
		return 0, nil
	}
	value, err := c.client.HGet(ctx, LikeVideoStatsKey(videoID), "like_count").Int64()
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (c *LikeStateCache) IsLiked(ctx context.Context, videoID, userID uint) (bool, error) {
	if c == nil || c.client == nil {
		return false, nil
	}
	value, err := c.client.SIsMember(ctx, LikeVideoUsersKey(videoID), userID).Result()
	return value, err
}

func (c *LikeStateCache) DirtyVideos(ctx context.Context, limit int64) ([]uint, error) {
	if c == nil || c.client == nil {
		return nil, nil
	}
	ids, err := c.client.SMembers(ctx, LikeDirtyVideosKey()).Result()
	if err != nil {
		return nil, err
	}
	if limit > 0 && int64(len(ids)) > limit {
		ids = ids[:limit]
	}
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		var videoID uint
		if _, err := fmt.Sscan(id, &videoID); err == nil && videoID > 0 {
			result = append(result, videoID)
		}
	}
	return result, nil
}

func (c *LikeStateCache) ClearDirty(ctx context.Context, videoID uint) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.SRem(ctx, LikeDirtyVideosKey(), videoID).Err()
}

func LikeVideoUsersKey(videoID uint) string {
	return fmt.Sprintf("like:video:%d:users", videoID)
}

func LikeUserVideosKey(userID uint) string {
	return fmt.Sprintf("like:user:%d:videos", userID)
}

func LikeVideoStatsKey(videoID uint) string {
	return fmt.Sprintf("video:stats:%d", videoID)
}

func LikeDirtyVideosKey() string {
	return "like:dirty:videos"
}
