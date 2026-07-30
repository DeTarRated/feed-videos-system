package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type CommentCache struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewCommentCache(client *goredis.Client, ttl time.Duration) *CommentCache {
	return &CommentCache{client: client, ttl: ttl}
}

// GetDetail returns a single comment from the cache.
// The caller is responsible for unmarshalling the JSON bytes.
func (c *CommentCache) GetDetail(ctx context.Context, commentID uint) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("cache unavailable")
	}
	return c.client.Get(ctx, CommentDetailKey(commentID)).Bytes()
}

// SetDetail writes a single comment to the cache.
// data should be the JSON-encoded comment.
func (c *CommentCache) SetDetail(ctx context.Context, commentID uint, data []byte) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Set(ctx, CommentDetailKey(commentID), data, c.ttl).Err()
}

// DeleteDetail removes a single comment from the cache.
func (c *CommentCache) DeleteDetail(ctx context.Context, commentID uint) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Del(ctx, CommentDetailKey(commentID)).Err()
}

// GetFirstPage returns the cached first page JSON for a video.
// The caller is responsible for unmarshalling the JSON bytes.
func (c *CommentCache) GetFirstPage(ctx context.Context, videoID uint) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("cache unavailable")
	}
	return c.client.Get(ctx, CommentFirstPageKey(videoID)).Bytes()
}

// SetFirstPage writes the cached first page JSON for a video.
// data should be the JSON-encoded comment slice.
func (c *CommentCache) SetFirstPage(ctx context.Context, videoID uint, data []byte) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Set(ctx, CommentFirstPageKey(videoID), data, 30*time.Second).Err()
}

// DeleteFirstPage removes the first page cache for a video.
func (c *CommentCache) DeleteFirstPage(ctx context.Context, videoID uint) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Del(ctx, CommentFirstPageKey(videoID)).Err()
}

func CommentDetailKey(commentID uint) string {
	return fmt.Sprintf("comment:detail:%d", commentID)
}

func CommentFirstPageKey(videoID uint) string {
	return fmt.Sprintf("comment:first_page:%d", videoID)
}

// UnmarshalComments is a helper to unmarshal a cached comment list.
func UnmarshalComments(data []byte, dest interface{}) error {
	return json.Unmarshal(data, dest)
}
