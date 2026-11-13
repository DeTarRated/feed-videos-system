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

func (s *HotFeedSnapshotStore) GetPage(ctx context.Context, snapshotID string, offset int, limit int) (*video.HotFeedSnapshotPage, error) {
	snapshot, err := s.Get(ctx, snapshotID)
	if err != nil || snapshot == nil {
		return nil, err
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(snapshot.IDs) {
		return &video.HotFeedSnapshotPage{SnapshotID: snapshot.ID, Offset: offset, IDs: []uint{}, HasMore: false}, nil
	}
	end := offset + limit
	if end > len(snapshot.IDs) {
		end = len(snapshot.IDs)
	}
	return &video.HotFeedSnapshotPage{
		SnapshotID: snapshot.ID,
		Offset:     end,
		IDs:        snapshot.IDs[offset:end],
		HasMore:    end < len(snapshot.IDs),
	}, nil
}

func HotFeedSnapshotKey(snapshotID string) string {
	return fmt.Sprintf("feed:hot:snapshot:%s", snapshotID)
}
