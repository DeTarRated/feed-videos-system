package cache

import (
	"context"
	"fmt"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

const HotVideoRankKey = "rank:video:hot"

type HotVideoRank struct {
	client *goredis.Client
}

func NewHotVideoRank(client *goredis.Client) *HotVideoRank {
	return &HotVideoRank{client: client}
}

func (r *HotVideoRank) SetScore(ctx context.Context, videoID uint, score float64) error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.ZAdd(ctx, HotVideoRankKey, goredis.Z{
		Score:  score,
		Member: strconv.FormatUint(uint64(videoID), 10),
	}).Err()
}

func (r *HotVideoRank) TopIDs(ctx context.Context, limit int) ([]uint, error) {
	if r == nil || r.client == nil || limit <= 0 {
		return []uint{}, nil
	}
	members, err := r.client.ZRevRange(ctx, HotVideoRankKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(members))
	for _, member := range members {
		id, err := strconv.ParseUint(member, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid hot video id")
		}
		ids = append(ids, uint(id))
	}
	return ids, nil
}
