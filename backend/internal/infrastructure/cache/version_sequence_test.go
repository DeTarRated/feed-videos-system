package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

type stubVersionSource struct {
	last uint64
	err  error
}

func (s *stubVersionSource) LastCommittedVersion(ctx context.Context, key string) (uint64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.last, nil
}

func setupSequencer(t *testing.T, source VersionSource) *VersionSequencer {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewVersionSequencer(client, source)
}

func TestVersionSequenceIncrementsWithoutFallback(t *testing.T) {
	seq := setupSequencer(t, nil)
	ctx := context.Background()

	first, err := seq.Next(ctx, "like:1:2")
	if err != nil {
		t.Fatalf("first next: %v", err)
	}
	second, err := seq.Next(ctx, "like:1:2")
	if err != nil {
		t.Fatalf("second next: %v", err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("unexpected sequence values: %d, %d", first, second)
	}
}

func TestVersionSequenceColdStartUsesRedisFirst(t *testing.T) {
	seq := setupSequencer(t, &stubVersionSource{last: 0})
	ctx := context.Background()

	value, err := seq.Next(ctx, "comment:abc")
	if err != nil {
		t.Fatalf("cold start next: %v", err)
	}
	if value < 1 {
		t.Fatalf("expected positive cold start version, got %d", value)
	}
}

func TestVersionSequenceDoesNotRegress(t *testing.T) {
	seq := setupSequencer(t, &stubVersionSource{last: 0})
	ctx := context.Background()

	first, err := seq.Next(ctx, "comment:abc")
	if err != nil {
		t.Fatalf("first next: %v", err)
	}
	second, err := seq.Next(ctx, "comment:abc")
	if err != nil {
		t.Fatalf("second next: %v", err)
	}
	third, err := seq.Next(ctx, "comment:abc")
	if err != nil {
		t.Fatalf("third next: %v", err)
	}
	if !(first < second && second < third) {
		t.Fatalf("expected monotonic sequence, got %d,%d,%d", first, second, third)
	}
}

func TestVersionSequenceConcurrentNextDoesNotShareResult(t *testing.T) {
	seq := setupSequencer(t, &stubVersionSource{last: 0})
	ctx := context.Background()
	const workers = 8
	results := make(chan uint64, workers)
	for i := 0; i < workers; i++ {
		go func() {
			v, err := seq.Next(ctx, "like:9:9")
			if err != nil {
				t.Errorf("next: %v", err)
				return
			}
			results <- v
		}()
	}
	seen := make(map[uint64]bool)
	for i := 0; i < workers; i++ {
		v := <-results
		if seen[v] {
			t.Fatalf("duplicate version allocated: %d", v)
		}
		seen[v] = true
	}
}
