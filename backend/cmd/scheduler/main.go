package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/infrastructure/cache"
	"feed-videos-system/backend/internal/infrastructure/redis"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	redisClient, err := redis.Open(context.Background(), cfg)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	defer redisClient.Close()

	scheduler := cache.NewHotRankScheduler(redisClient, cfg.SchedulerLockTTL, cfg.SchedulerLockRenewInterval)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopCh
		cancel()
	}()

	if err := scheduler.Acquire(ctx); err != nil {
		log.Fatalf("acquire scheduler lock: %v", err)
	}
	defer func() {
		if err := scheduler.Release(context.Background()); err != nil {
			log.Printf("release scheduler lock: %v", err)
		}
	}()

	renewTicker := time.NewTicker(scheduler.RenewInterval())
	defer renewTicker.Stop()

	aggregateTicker := time.NewTicker(time.Minute)
	defer aggregateTicker.Stop()

	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	minuteWindow := cache.NewHotMinuteWindow(redisClient, 70*time.Minute)

	log.Printf("hot rank scheduler active")
	for {
		select {
		case <-ctx.Done():
			return
		case <-renewTicker.C:
			if err := scheduler.Renew(ctx); err != nil {
				log.Printf("renew scheduler lock: %v", err)
				return
			}
		case now := <-aggregateTicker.C:
			if err := slideWindow(ctx, scheduler, minuteWindow, now.UTC(), cfg.SchedulerCatchUpLimit); err != nil {
				log.Printf("slide hot window: %v", err)
			}
		case <-cleanupTicker.C:
			if err := minuteWindow.Cleanup(ctx); err != nil {
				log.Printf("cleanup hot window: %v", err)
			}
		}
	}
}

func slideWindow(ctx context.Context, scheduler *cache.HotRankScheduler, minuteWindow *cache.HotMinuteWindow, now time.Time, catchUpLimit int) error {
	if err := applyWindowSide(ctx, scheduler, minuteWindow, now, "add", -1, catchUpLimit); err != nil {
		return err
	}
	return applyWindowSide(ctx, scheduler, minuteWindow, now, "remove", -61, catchUpLimit)
}

func applyWindowSide(ctx context.Context, scheduler *cache.HotRankScheduler, minuteWindow *cache.HotMinuteWindow, now time.Time, direction string, offsetMinute int, catchUpLimit int) error {
	cursor, err := scheduler.CurrentCursor(ctx, direction)
	if err != nil {
		return err
	}
	start := now.Add(time.Duration(offsetMinute) * time.Minute)
	startKey := cache.FormatMinuteKey(start)
	if cursor == "" {
		cursor = startKey
	}
	for i := 0; i < catchUpLimit; i++ {
		current, err := time.ParseInLocation("200601021504", cursor, time.UTC)
		if err != nil {
			return err
		}
		if direction == "add" && !current.Before(start.Add(time.Minute)) {
			return nil
		}
		if direction == "remove" && !current.Before(start.Add(time.Minute)) {
			return nil
		}
		_, err = scheduler.ApplyMinute(ctx, current, direction, 7*24*time.Hour)
		if err != nil {
			return err
		}
		if direction == "remove" {
			if err := minuteWindow.DeleteBucket(ctx, current); err != nil {
				return err
			}
		}
		next, err := cache.AddMinutesString(cursor, 1)
		if err != nil {
			return err
		}
		cursor = next
	}
	return nil
}
