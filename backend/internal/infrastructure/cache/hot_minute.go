package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	HotWindowRankKey   = "rank:video:hot:60m"
	hotMinuteKeyPrefix = "hot:video:minute:"
)

// HotMinuteWindow manages per-minute buckets and a 60-minute sliding-window aggregate ZSET.
type HotMinuteWindow struct {
	client *goredis.Client
	ttl    time.Duration // bucket TTL, typically 70 minutes
}

func NewHotMinuteWindow(client *goredis.Client, ttl time.Duration) *HotMinuteWindow {
	return &HotMinuteWindow{client: client, ttl: ttl}
}

// MinuteKey returns the minute-bucket key for a given time.
func MinuteKey(t time.Time) string {
	return fmt.Sprintf("%s%s", hotMinuteKeyPrefix, t.Format("200601021504"))
}

// AddToBucket adds a delta to the video's score in the minute bucket for t.
// Sets TTL on the bucket so it auto-expires after the window.
func (w *HotMinuteWindow) AddToBucket(ctx context.Context, t time.Time, videoID uint, delta int64) error {
	if w == nil || w.client == nil {
		return nil
	}
	key := MinuteKey(t)
	member := strconv.FormatUint(uint64(videoID), 10)
	if _, err := w.client.ZIncrBy(ctx, key, float64(delta), member).Result(); err != nil {
		return err
	}
	return w.client.Expire(ctx, key, w.ttl).Err()
}

// BucketMembers returns all video IDs and their scores in a minute bucket.
func (w *HotMinuteWindow) BucketMembers(ctx context.Context, t time.Time) (map[uint]float64, error) {
	if w == nil || w.client == nil {
		return nil, nil
	}
	key := MinuteKey(t)
	members, err := w.client.ZRangeWithScores(ctx, key, 0, -1).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, err
	}
	result := make(map[uint]float64, len(members))
	for _, m := range members {
		uid, err := strconv.ParseUint(m.Member.(string), 10, 64)
		if err != nil || uid == 0 {
			continue
		}
		result[uint(uid)] = m.Score
	}
	return result, nil
}

// ApplyBucket aggregates or de-aggregates a minute bucket into the aggregate ZSET.
// multiplier should be +1 to add the bucket to the window, or -1 to remove it.
func (w *HotMinuteWindow) ApplyBucket(ctx context.Context, t time.Time, multiplier float64) error {
	if w == nil || w.client == nil {
		return nil
	}
	members, err := w.BucketMembers(ctx, t)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}

	for videoID, score := range members {
		member := strconv.FormatUint(uint64(videoID), 10)
		delta := score * multiplier
		if _, err := w.client.ZIncrBy(ctx, HotWindowRankKey, delta, member).Result(); err != nil {
			return err
		}
	}
	return nil
}

// DeleteBucket removes a minute bucket key.
func (w *HotMinuteWindow) DeleteBucket(ctx context.Context, t time.Time) error {
	if w == nil || w.client == nil {
		return nil
	}
	return w.client.Del(ctx, MinuteKey(t)).Err()
}

// SetScore implements video.RankStore by setting a score directly on the aggregate ZSET.
// Used by RefreshHotScore for newly published videos.
func (w *HotMinuteWindow) SetScore(ctx context.Context, videoID uint, score float64) error {
	if w == nil || w.client == nil {
		return nil
	}
	return w.client.ZAdd(ctx, HotWindowRankKey, goredis.Z{
		Score:  score,
		Member: strconv.FormatUint(uint64(videoID), 10),
	}).Err()
}

// TopIDs returns the top N video IDs from the aggregate ZSET.
func (w *HotMinuteWindow) TopIDs(ctx context.Context, limit int) ([]uint, error) {
	if w == nil || w.client == nil || limit <= 0 {
		return []uint{}, nil
	}
	members, err := w.client.ZRevRange(ctx, HotWindowRankKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(members))
	for _, member := range members {
		id, err := strconv.ParseUint(member, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		ids = append(ids, uint(id))
	}
	return ids, nil
}

// Cleanup removes videos with score ≤ 0 and caps the aggregate ZSET at 5000 members.
func (w *HotMinuteWindow) Cleanup(ctx context.Context) error {
	if w == nil || w.client == nil {
		return nil
	}
	// Remove videos whose score has dropped to 0 or below.
	if err := w.client.ZRemRangeByScore(ctx, HotWindowRankKey, "-inf", "0").Err(); err != nil {
		return err
	}
	// Keep only the top 5000 by rank to bound memory.
	count, err := w.client.ZCard(ctx, HotWindowRankKey).Result()
	if err != nil {
		return err
	}
	if count > 5000 {
		// ZREMRANGEBYRANK 0 0 removes the lowest-ranked (index 0).
		return w.client.ZRemRangeByRank(ctx, HotWindowRankKey, 0, count-5001).Err()
	}
	return nil
}
