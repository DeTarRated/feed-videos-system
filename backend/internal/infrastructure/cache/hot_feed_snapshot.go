package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"feed-videos-system/backend/internal/domain/video"
)

type HotFeedSnapshotStore struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewHotFeedSnapshotStore(client *goredis.Client, ttl time.Duration) *HotFeedSnapshotStore {
	return &HotFeedSnapshotStore{client: client, ttl: ttl}
}

func (s *HotFeedSnapshotStore) Create(ctx context.Context, ids []uint) (*video.HotFeedSnapshot, error) {
	if s == nil || s.client == nil {
		return &video.HotFeedSnapshot{IDs: ids, Created: time.Now().UTC()}, nil
	}
	snapshot := &video.HotFeedSnapshot{
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		IDs:     ids,
		Created: time.Now().UTC(),
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if err := s.client.Set(ctx, HotFeedSnapshotKey(snapshot.ID), data, s.ttl).Err(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *HotFeedSnapshotStore) Get(ctx context.Context, snapshotID string) (*video.HotFeedSnapshot, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	value, err := s.client.Get(ctx, HotFeedSnapshotKey(snapshotID)).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var snapshot video.HotFeedSnapshot
	if err := json.Unmarshal([]byte(value), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func HotFeedSnapshotKey(snapshotID string) string {
	return fmt.Sprintf("feed:hot:snapshot:%s", snapshotID)
}
