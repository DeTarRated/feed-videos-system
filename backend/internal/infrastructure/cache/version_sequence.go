package cache

import (
	"context"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type VersionSource interface {
	LastCommittedVersion(ctx context.Context, key string) (uint64, error)
}

type VersionSequencer struct {
	client *goredis.Client
	source VersionSource
	group  singleflight.Group
}

func NewVersionSequencer(client *goredis.Client, source VersionSource) *VersionSequencer {
	return &VersionSequencer{client: client, source: source}
}

func VersionSequenceKey(key string) string {
	return "seq:" + key
}

func parseUint(value string) uint64 {
	var n uint64
	_, _ = fmt.Sscan(value, &n)
	return n
}

func (s *VersionSequencer) Next(ctx context.Context, key string) (uint64, error) {
	if s == nil || s.client == nil {
		return 0, errors.New("version sequencer unavailable")
	}
	redisKey := VersionSequenceKey(key)
	n, err := s.client.Incr(ctx, redisKey).Result()
	if err == nil && (s.source == nil || n > 1) {
		return uint64(n), nil
	}
	value, flightErr, _ := s.group.Do("version:"+key, func() (interface{}, error) {
		return s.next(ctx, key)
	})
	if flightErr != nil {
		return 0, flightErr
	}
	return value.(uint64), nil
}

func (s *VersionSequencer) next(ctx context.Context, key string) (uint64, error) {
	redisKey := VersionSequenceKey(key)
	n, err := s.client.Incr(ctx, redisKey).Result()
	if err == nil {
		if s.source == nil || n > 1 {
			return uint64(n), nil
		}
		last, sourceErr := s.source.LastCommittedVersion(ctx, key)
		if sourceErr != nil {
			return 0, sourceErr
		}
		if uint64(n) <= last {
			if err := s.client.Set(ctx, redisKey, last, 0).Err(); err != nil {
				return 0, err
			}
			n, err = s.client.Incr(ctx, redisKey).Result()
			if err != nil {
				return 0, err
			}
		}
		return uint64(n), nil
	}
	if err != goredis.Nil {
		// Continue to recovery for other Redis errors.
	}

	if s.source == nil {
		return 0, fmt.Errorf("sequence unavailable for %s: %w", key, err)
	}

	last, sourceErr := s.source.LastCommittedVersion(ctx, key)
	if sourceErr != nil {
		return 0, fmt.Errorf("recover sequence %s: %v (redis error: %w)", key, sourceErr, err)
	}

	current, getErr := s.client.Get(ctx, redisKey).Result()
	if getErr != nil && getErr != goredis.Nil {
		return 0, getErr
	}
	if getErr == goredis.Nil || parseUint(current) < last {
		if err := s.client.SetNX(ctx, redisKey, last, 0).Err(); err != nil {
			return 0, err
		}
	}
	n, err = s.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return 0, err
	}
	return uint64(n), nil
}
