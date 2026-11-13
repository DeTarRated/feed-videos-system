package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func setupIdempotency(t *testing.T) (*IdempotencyStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewIdempotencyStore(client, time.Minute), mr
}

func TestIdempotencyAcquireAndRepublishSameCommand(t *testing.T) {
	store, _ := setupIdempotency(t)
	ctx := context.Background()
	key := IdempotencyKey(1, "like.create", "key-1")
	body := []byte(`{"schemaVersion":1}`)
	publisher := &fakePublisher{}

	if err := store.Acquire(ctx, key, "fp-1", "rk-like", []byte(`{"operationId":"op-1"}`), body); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := store.Acquire(ctx, key, "fp-1", "rk-like", []byte(`{"operationId":"op-1"}`), body); err != nil {
		t.Fatalf("same acquire: %v", err)
	}
	if err := store.Republish(ctx, key, publisher); err != nil {
		t.Fatalf("republish: %v", err)
	}
	if len(publisher.bodies) != 1 || string(publisher.bodies[0]) != string(body) {
		t.Fatalf("unexpected publisher bodies: %v", publisher.bodies)
	}
}

func TestIdempotencyFingerprintConflict(t *testing.T) {
	store, _ := setupIdempotency(t)
	ctx := context.Background()
	key := IdempotencyKey(1, "like.create", "key-1")

	if err := store.Acquire(ctx, key, "fp-1", "rk-like", []byte(`{"operationId":"op-1"}`), []byte(`body-1`)); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	err := store.Acquire(ctx, key, "fp-2", "rk-like", []byte(`{"operationId":"op-2"}`), []byte(`body-2`))
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

type fakePublisher struct {
	bodies [][]byte
}

func (f *fakePublisher) PublishCommandWithConfirm(ctx context.Context, routingKey string, body []byte, timeout time.Duration) error {
	f.bodies = append(f.bodies, body)
	return nil
}
