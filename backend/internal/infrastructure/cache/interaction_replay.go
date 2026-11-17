package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var applyInteractionScript = goredis.NewScript(`
	local marker_key = KEYS[1]
	local stats_key = KEYS[2]
	local minute_key = KEYS[3]
	local like_user_state_key = KEYS[4]

	local operation_id = ARGV[1]
	local video_id = ARGV[2]
	local actor_id = ARGV[3]
	local command_type = ARGV[4]
	local op_version = tonumber(ARGV[5])
	local like_count = tonumber(ARGV[6])
	local comment_count = tonumber(ARGV[7])
	local stats_version = tonumber(ARGV[8])
	local heat_delta = tonumber(ARGV[9])
	local heat_minute = ARGV[10]
	local ttl_seconds = tonumber(ARGV[11])

	if redis.call('EXISTS', marker_key) == 1 then
		return 0
	end

	local current_stats = redis.call('HMGET', stats_key, 'stats_version')
	local current_version = current_stats[1] and tonumber(current_stats[1]) or 0
	if stats_version < current_version then
		return 0
	end

	if command_type == 'like.create' or command_type == 'like.cancel' then
		local liked = '0'
		if command_type == 'like.create' then
			liked = '1'
		end
		redis.call('HSET', like_user_state_key, 'liked', liked, 'op_version', op_version)
		if ttl_seconds > 0 then
			redis.call('EXPIRE', like_user_state_key, ttl_seconds)
		end
	end

	redis.call('HSET', stats_key,
		'like_count', like_count,
		'comment_count', comment_count,
		'stats_version', stats_version)
	if ttl_seconds > 0 then
		redis.call('EXPIRE', stats_key, ttl_seconds)
	end

	if heat_delta ~= 0 and heat_minute ~= '' then
		redis.call('ZINCRBY', minute_key, heat_delta, video_id)
		redis.call('EXPIRE', minute_key, ttl_seconds)
	end

	redis.call('SET', marker_key, 1)
	redis.call('EXPIRE', marker_key, ttl_seconds)
	return 1
`)

type InteractionReplayCache struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewInteractionReplayCache(client *goredis.Client, ttl time.Duration) *InteractionReplayCache {
	return &InteractionReplayCache{client: client, ttl: ttl}
}

func (c *InteractionReplayCache) ApplyOutcome(ctx context.Context, outcome InteractionOutcomeView) (bool, error) {
	if c == nil || c.client == nil {
		return false, fmt.Errorf("interaction replay cache unavailable")
	}
	keys := []string{
		InteractionMarkerKey(outcome.OperationID),
		LikeVideoStatsKey(outcome.VideoID),
		MinuteBucketKey(outcome.HeatMinute),
		LikeUserStateKey(outcome.VideoID, outcome.ActorID),
	}
	result, err := applyInteractionScript.Run(ctx, c.client, keys,
		outcome.OperationID,
		strconv.FormatUint(uint64(outcome.VideoID), 10),
		strconv.FormatUint(uint64(outcome.ActorID), 10),
		outcome.CommandType,
		strconv.FormatUint(outcome.OpVersion, 10),
		strconv.FormatInt(outcome.LikeCount, 10),
		strconv.FormatInt(outcome.CommentCount, 10),
		strconv.FormatUint(outcome.StatsVersion, 10),
		outcome.HeatDelta,
		outcome.HeatMinute,
		int64(c.ttl/time.Second),
	).Result()
	if err != nil {
		return false, err
	}
	applied, ok := result.(int64)
	if !ok {
		if f, isFloat := result.(float64); isFloat {
			applied = int64(f)
			ok = true
		}
	}
	if !ok {
		return false, fmt.Errorf("unexpected replay result: %v", result)
	}
	return applied == 1, nil
}

func InteractionMarkerKey(operationID string) string {
	return "interaction:applied:" + operationID
}

func MinuteBucketKey(minute string) string {
	if minute == "" {
		return "hot:video:minute:unknown"
	}
	return "hot:video:minute:" + minute
}

type InteractionOutcomeView struct {
	OperationID  string
	VideoID      uint
	ActorID      uint
	CommandType  string
	OpVersion    uint64
	LikeCount    int64
	CommentCount int64
	StatsVersion uint64
	HeatDelta    int64
	HeatMinute   string
}
