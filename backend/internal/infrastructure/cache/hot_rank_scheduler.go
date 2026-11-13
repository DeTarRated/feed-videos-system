package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const HotRankSchedulerLockKey = "lock:hot-rank:scheduler"

var ErrSchedulerLockLost = errors.New("hot rank scheduler lock lost")

type HotRankScheduler struct {
	client        *goredis.Client
	lockTTL       time.Duration
	renewInterval time.Duration
	ownerToken    string
}

func NewHotRankScheduler(client *goredis.Client, lockTTL, renewInterval time.Duration) *HotRankScheduler {
	return &HotRankScheduler{
		client:        client,
		lockTTL:       lockTTL,
		renewInterval: renewInterval,
		ownerToken:    newOwnerToken(),
	}
}

func newOwnerToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func (s *HotRankScheduler) Acquire(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("hot rank scheduler unavailable")
	}
	ok, err := s.client.SetNX(ctx, HotRankSchedulerLockKey, s.ownerToken, s.lockTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("hot rank scheduler is already active")
	}
	return nil
}

func (s *HotRankScheduler) Renew(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("hot rank scheduler unavailable")
	}
	lua := goredis.NewScript(`
		local key = KEYS[1]
		local token = ARGV[1]
		local ttl = tonumber(ARGV[2])
		if redis.call('GET', key) ~= token then
			return 0
		end
		redis.call('PEXPIRE', key, ttl)
		return 1
	`)
	result, err := lua.Run(ctx, s.client, []string{HotRankSchedulerLockKey}, s.ownerToken, int64(s.lockTTL/time.Millisecond)).Result()
	if err != nil {
		return err
	}
	if result == int64(0) {
		return ErrSchedulerLockLost
	}
	return nil
}

func (s *HotRankScheduler) Release(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	lua := goredis.NewScript(`
		local key = KEYS[1]
		local token = ARGV[1]
		if redis.call('GET', key) ~= token then
			return 0
		end
		return redis.call('DEL', key)
	`)
	_, err := lua.Run(ctx, s.client, []string{HotRankSchedulerLockKey}, s.ownerToken).Result()
	return err
}

func (s *HotRankScheduler) RenewInterval() time.Duration {
	return s.renewInterval
}

var applyMinuteWindowScript = goredis.NewScript(`
	local lock_key = KEYS[1]
	local owner = ARGV[1]
	local direction = ARGV[2]
	local minute = ARGV[3]
	local bucket_key = ARGV[4]
	local rank_key = ARGV[5]
	local cursor_key = ARGV[6]
	local applied_key = ARGV[7]
	local add_applied_key = ARGV[8]
	local ttl = tonumber(ARGV[9])

	if redis.call('GET', lock_key) ~= owner then
		return -1
	end

	if redis.call('EXISTS', applied_key) == 1 then
		return 1
	end

	if direction == 'remove' and redis.call('EXISTS', add_applied_key) == 0 then
		redis.call('SET', applied_key, '1')
		redis.call('PEXPIRE', applied_key, ttl)
		redis.call('SET', cursor_key, minute)
		redis.call('PEXPIRE', cursor_key, ttl)
		return 2
	end

	local members = redis.call('ZRANGE', bucket_key, 0, -1, 'WITHSCORES')
	for i = 1, #members, 2 do
		local delta = tonumber(members[i + 1])
		if direction == 'remove' then
			delta = -delta
		end
		redis.call('ZINCRBY', rank_key, delta, members[i])
	end

	redis.call('SET', applied_key, '1')
	redis.call('PEXPIRE', applied_key, ttl)
	redis.call('SET', cursor_key, minute)
	redis.call('PEXPIRE', cursor_key, ttl)
	return 0
`)

func (s *HotRankScheduler) ApplyMinute(ctx context.Context, minute time.Time, direction string, bucketTTL time.Duration) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("hot rank scheduler unavailable")
	}
	minuteValue := minute.UTC().Format("200601021504")
	keys := []string{
		HotRankSchedulerLockKey,
	}
	args := []any{
		s.ownerToken,
		direction,
		minuteValue,
		MinuteKey(minute),
		HotWindowRankKey,
		hotWindowCursorKey(direction),
		hotWindowAppliedKey(direction, minuteValue),
		hotWindowAppliedKey("add", minuteValue),
		int64(bucketTTL / time.Millisecond),
	}
	result, err := applyMinuteWindowScript.Run(ctx, s.client, keys, args...).Result()
	if err != nil {
		return false, err
	}
	code, ok := result.(int64)
	if !ok {
		if f, isFloat := result.(float64); isFloat {
			code = int64(f)
			ok = true
		}
	}
	if !ok {
		return false, fmt.Errorf("unexpected apply minute result: %v", result)
	}
	if code == -1 {
		return false, ErrSchedulerLockLost
	}
	return code != 0, nil
}

func (s *HotRankScheduler) CurrentCursor(ctx context.Context, direction string) (string, error) {
	if s == nil || s.client == nil {
		return "", nil
	}
	value, err := s.client.Get(ctx, hotWindowCursorKey(direction)).Result()
	if err != nil {
		if err == goredis.Nil {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func hotWindowCursorKey(direction string) string {
	return "hot:window:cursor:" + direction
}

func hotWindowAppliedKey(direction, minute string) string {
	return "hot:window:applied:" + direction + ":" + minute
}

func ParseMinuteKey(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("200601021504", value, time.UTC)
}

func FormatMinuteKey(t time.Time) string {
	return t.UTC().Format("200601021504")
}

func AddMinutesString(minute string, count int) (string, error) {
	t, err := ParseMinuteKey(minute)
	if err != nil {
		return "", err
	}
	return FormatMinuteKey(t.Add(time.Duration(count) * time.Minute)), nil
}

func MinuteStringBefore(a, b string) bool {
	if a == "" {
		return true
	}
	if b == "" {
		return false
	}
	return a < b
}
