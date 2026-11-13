package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with previous request")
)

type RepublishPublisher interface {
	PublishCommandWithConfirm(ctx context.Context, routingKey string, body []byte, timeout time.Duration) error
}

type IdempotencyStore struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewIdempotencyStore(client *goredis.Client, ttl time.Duration) *IdempotencyStore {
	return &IdempotencyStore{client: client, ttl: ttl}
}

type IdempotencyRecord struct {
	RequestFingerprint string          `json:"requestFingerprint"`
	Command            json.RawMessage `json:"command"`
	Body               string          `json:"body"`
	RoutingKey         string          `json:"routingKey"`
}

func (s *IdempotencyStore) Get(ctx context.Context, key string) (*IdempotencyRecord, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("idempotency store unavailable")
	}
	value, err := s.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var record IdempotencyRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func IdempotencyKey(actorID uint, operation string, key string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s", actorID, operation, key)))
	return "idem:" + hex.EncodeToString(sum[:])
}

func (s *IdempotencyStore) Acquire(ctx context.Context, key string, fingerprint string, routingKey string, command []byte, body []byte) error {
	if s == nil || s.client == nil {
		return errors.New("idempotency store unavailable")
	}
	record := IdempotencyRecord{
		RequestFingerprint: fingerprint,
		Command:            json.RawMessage(command),
		Body:               string(body),
		RoutingKey:         routingKey,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	lua := goredis.NewScript(`
		local key = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local value = ARGV[2]
		local fingerprint = ARGV[3]

		local existing = redis.call('GET', key)
		if existing then
			local decoded = cjson.decode(existing)
			if decoded.requestFingerprint ~= fingerprint then
				return 2
			end
			return 1
		end

		local ok = redis.call('SET', key, value, 'NX', 'PX', ttl)
		if not ok then
			return 3
		end
		return 0
	`)
	result, err := lua.Run(ctx, s.client, []string{key}, int64(s.ttl/time.Millisecond), string(data), fingerprint).Result()
	if err != nil {
		return err
	}
	code, ok := result.(int64)
	if !ok {
		if f, isFloat := result.(float64); isFloat {
			code = int64(f)
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("unexpected idempotency result: %v", result)
	}
	switch code {
	case 0, 1:
		return nil
	case 2:
		return ErrIdempotencyConflict
	default:
		return fmt.Errorf("idempotency acquire lost race")
	}
}

func (s *IdempotencyStore) Republish(ctx context.Context, key string, publisher RepublishPublisher) error {
	record, err := s.Get(ctx, key)
	if err != nil {
		return err
	}
	return publisher.PublishCommandWithConfirm(ctx, record.RoutingKey, []byte(record.Body), 3*time.Second)
}
