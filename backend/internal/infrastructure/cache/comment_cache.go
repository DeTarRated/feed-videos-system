package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type CommentCache struct {
	client *goredis.Client
	ttl    time.Duration
	group  singleflight.Group
}

func (c *CommentCache) Do(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	value, err, _ := c.group.Do(key, func() (interface{}, error) {
		return fn(ctx)
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}

func NewCommentCache(client *goredis.Client, ttl time.Duration) *CommentCache {
	return &CommentCache{client: client, ttl: ttl}
}

// GetDetail returns a single comment from the cache.
// The caller is responsible for unmarshalling the JSON bytes.
func (c *CommentCache) GetDetail(ctx context.Context, commentID string) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("cache unavailable")
	}
	return c.client.Get(ctx, CommentDetailKey(commentID)).Bytes()
}

// SetDetail writes a single comment to the cache.
// data should be the JSON-encoded comment.
func (c *CommentCache) SetDetail(ctx context.Context, commentID string, data []byte) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Set(ctx, CommentDetailKey(commentID), data, c.ttl).Err()
}

// DeleteDetail removes a single comment from the cache.
func (c *CommentCache) DeleteDetail(ctx context.Context, commentID string) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Del(ctx, CommentDetailKey(commentID)).Err()
}

// GetFirstPage returns the cached first page JSON for a video.
// The caller is responsible for unmarshalling the JSON bytes.
func (c *CommentCache) GetFirstPage(ctx context.Context, videoID uint) ([]byte, error) {
	return c.GetFirstPageWithLimit(ctx, videoID, 0)
}

func (c *CommentCache) GetFirstPageWithLimit(ctx context.Context, videoID uint, limit int) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("cache unavailable")
	}
	return c.client.Get(ctx, CommentFirstPageKeyWithLimit(videoID, limit)).Bytes()
}

// SetFirstPage writes the cached first page JSON for a video.
// data should be the JSON-encoded comment slice.
func (c *CommentCache) SetFirstPage(ctx context.Context, videoID uint, data []byte) error {
	return c.SetFirstPageWithLimit(ctx, videoID, 0, data)
}

func (c *CommentCache) SetFirstPageWithLimit(ctx context.Context, videoID uint, limit int, data []byte) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Set(ctx, CommentFirstPageKeyWithLimit(videoID, limit), data, 30*time.Second).Err()
}

// DeleteFirstPage removes the first page cache for a video.
func (c *CommentCache) DeleteFirstPage(ctx context.Context, videoID uint) error {
	if c == nil || c.client == nil {
		return nil
	}
	pattern := fmt.Sprintf("comment:first_page:%d:*", videoID)
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		_ = c.client.Del(ctx, iter.Val()).Err()
	}
	return iter.Err()
}

func CommentDetailKey(commentID string) string {
	return fmt.Sprintf("comment:detail:%s", commentID)
}

func CommentFirstPageKey(videoID uint) string {
	return fmt.Sprintf("comment:first_page:%d", videoID)
}

func CommentFirstPageKeyWithLimit(videoID uint, limit int) string {
	return fmt.Sprintf("comment:first_page:%d:%d", videoID, limit)
}

// UnmarshalComments is a helper to unmarshal a cached comment list.
func UnmarshalComments(data []byte, dest interface{}) error {
	return json.Unmarshal(data, dest)
}
