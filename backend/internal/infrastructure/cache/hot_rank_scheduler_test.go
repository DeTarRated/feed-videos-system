package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func setupScheduler(t *testing.T) (*HotRankScheduler, *miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewHotRankScheduler(client, time.Minute, time.Second), mr, client
}

func TestSchedulerAcquireRenewRelease(t *testing.T) {
	scheduler, _, _ := setupScheduler(t)
	ctx := context.Background()

	if err := scheduler.Acquire(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := scheduler.Renew(ctx); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := scheduler.Release(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestSchedulerCurrentCursorMissing(t *testing.T) {
	scheduler, _, _ := setupScheduler(t)
	ctx := context.Background()

	value, err := scheduler.CurrentCursor(ctx, "add")
	if err != nil {
		t.Fatalf("current cursor: %v", err)
	}
	if value != "" {
		t.Fatalf("expected empty cursor, got %q", value)
	}
}

func TestSchedulerApplyMinuteRequiresLock(t *testing.T) {
	scheduler, _, client := setupScheduler(t)
	ctx := context.Background()

	mrTime, _ := time.ParseInLocation("200601021504", "202401010000", time.UTC)
	applied, err := scheduler.ApplyMinute(ctx, mrTime, "add", time.Hour)
	if err == nil {
		t.Fatalf("expected lock error, got applied=%v", applied)
	}
	_ = client
}

func TestSchedulerApplyMinuteSkipsRemoveWithoutAdd(t *testing.T) {
	scheduler, _, client := setupScheduler(t)
	ctx := context.Background()
	if err := scheduler.Acquire(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	mrTime, _ := time.ParseInLocation("200601021504", "202401010000", time.UTC)
	_ = client.ZAdd(ctx, MinuteKey(mrTime), goredis.Z{Score: 10, Member: "1"}).Err()

	_, err := scheduler.ApplyMinute(ctx, mrTime, "remove", time.Hour)
	if err != nil {
		t.Fatalf("apply remove: %v", err)
	}
	score, _ := client.ZScore(ctx, HotWindowRankKey, "1").Result()
	if score != 0 {
		t.Fatalf("aggregate score should remain 0, got %v", score)
	}
}
