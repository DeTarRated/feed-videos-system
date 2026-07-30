package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var syncDirtyScript = goredis.NewScript(`
	local users_key = KEYS[1]
	local stats_key = KEYS[2]
	local dirty_key = KEYS[3]
	local video_id  = ARGV[1]

	local users = redis.call('SMEMBERS', users_key)
	local count = redis.call('HGET', stats_key, 'like_count')
	redis.call('SREM', dirty_key, video_id)

	local result = { count or '0' }
	for i, u in ipairs(users) do
		result[#result + 1] = u
	end
	return result
`)

type LikeStateCache struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewLikeStateCache(client *goredis.Client, ttl time.Duration) *LikeStateCache {
	return &LikeStateCache{client: client, ttl: ttl}
}

// DirtyVideoSnapshot represents the state of a dirty video at sync time.
type DirtyVideoSnapshot struct {
	VideoID   uint
	LikeCount int64
	UserIDs   []uint
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

func (c *LikeStateCache) IncrementCommentCount(ctx context.Context, videoID uint, delta int64) (int64, error) {
	if c == nil || c.client == nil {
		return 0, nil
	}
	count, err := c.client.HIncrBy(ctx, LikeVideoStatsKey(videoID), "comment_count", delta).Result()
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

func (c *LikeStateCache) GetCommentCount(ctx context.Context, videoID uint) (int64, error) {
	if c == nil || c.client == nil {
		return 0, nil
	}
	value, err := c.client.HGet(ctx, LikeVideoStatsKey(videoID), "comment_count").Int64()
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

// SyncDirtyVideo atomically reads the user set and like count for a video,
// then removes the video from the dirty set. It returns a snapshot suitable
// for batch MySQL persistence.
//
// The Lua script eliminates the race between reading users and clearing
// the dirty flag: a new like arriving mid-sync will SADD the dirty flag
// again, so the video stays queued for the next cycle.
func (c *LikeStateCache) SyncDirtyVideo(ctx context.Context, videoID uint) (*DirtyVideoSnapshot, error) {
	if c == nil || c.client == nil {
		return nil, nil
	}

	keys := []string{
		LikeVideoUsersKey(videoID),
		LikeVideoStatsKey(videoID),
		LikeDirtyVideosKey(),
	}

	result, err := syncDirtyScript.Run(ctx, c.client, keys, videoID).Result()
	if err != nil {
		return nil, err
	}

	parts, ok := result.([]interface{})
	if !ok || len(parts) == 0 {
		return &DirtyVideoSnapshot{VideoID: videoID}, nil
	}

	countStr := toString(parts[0])
	count, _ := strconv.ParseInt(countStr, 10, 64)

	userIDs := make([]uint, 0, len(parts)-1)
	for _, p := range parts[1:] {
		uid, err := strconv.ParseUint(toString(p), 10, 64)
		if err == nil && uid > 0 {
			userIDs = append(userIDs, uint(uid))
		}
	}

	return &DirtyVideoSnapshot{
		VideoID:   videoID,
		LikeCount: count,
		UserIDs:   userIDs,
	}, nil
}

// DirtyVideos returns up to limit random video IDs from the dirty set.
// Uses SRANDMEMBER for efficient sampling without fetching the entire set.
func (c *LikeStateCache) DirtyVideos(ctx context.Context, limit int64) ([]uint, error) {
	if c == nil || c.client == nil {
		return nil, nil
	}
	ids, err := c.client.SRandMemberN(ctx, LikeDirtyVideosKey(), limit).Result()
	if err != nil {
		return nil, err
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

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
