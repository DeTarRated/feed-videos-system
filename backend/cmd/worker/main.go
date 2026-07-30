package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/domain/social"
	"feed-videos-system/backend/internal/infrastructure/cache"
	"feed-videos-system/backend/internal/infrastructure/mq"
	"feed-videos-system/backend/internal/infrastructure/mysql"
	"feed-videos-system/backend/internal/infrastructure/redis"

	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	db, err := mysql.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("connect mysql: %v", err)
	}

	redisClient, err := redis.Open(context.Background(), cfg)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	defer redisClient.Close()

	mqConn, err := mq.Open(cfg)
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer mqConn.Close()

	consumer := mq.NewConsumer(mqConn, cfg.RabbitMQQueue, cfg.RabbitMQExchange, cfg.RabbitMQRoutingKey)
	queueName, err := consumer.Setup()
	if err != nil {
		log.Fatalf("setup rabbitmq consumer: %v", err)
	}
	log.Printf("worker listening on queue %s", queueName)

	likeCache := cache.NewLikeStateCache(redisClient, 30*24*time.Hour)
	commentCache := cache.NewCommentCache(redisClient, 5*time.Minute)
	videoDetailCache := cache.NewVideoDetailCache(redisClient, cfg.VideoDetailCacheTTL)
	socialRepo := mysql.NewSocialRepository(db)
	minuteWindow := cache.NewHotMinuteWindow(redisClient, 70*time.Minute)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-stopCh
		cancel()
	}()

	// Start periodic batch MySQL sync for likes.
	go syncLoop(ctx, db, likeCache, 30*time.Second)

	// Start 60-minute sliding window aggregation and cleanup.
	go aggregateLoop(ctx, minuteWindow, time.Minute)
	go cleanupLoop(ctx, minuteWindow, 5*time.Minute)

	if err := consumeEvents(ctx, mqConn, cfg, likeCache, commentCache, videoDetailCache, minuteWindow, socialRepo); err != nil {
		log.Printf("worker stopped with error: %v", err)
	}
}

func consumeEvents(
	ctx context.Context,
	conn *mq.Connection,
	cfg config.Config,
	likeCache *cache.LikeStateCache,
	commentCache *cache.CommentCache,
	videoDetailCache *cache.VideoDetailCache,
	minuteWindow *cache.HotMinuteWindow,
	socialRepo *mysql.SocialRepository,
) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := mq.DeclareExchange(ch, cfg.RabbitMQExchange); err != nil {
		return err
	}
	queue, err := mq.DeclareQueue(ch, cfg.RabbitMQQueue)
	if err != nil {
		return err
	}
	if err := ch.QueueBind(queue.Name, cfg.RabbitMQRoutingKey, cfg.RabbitMQExchange, false, nil); err != nil {
		return err
	}

	msgs, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			var event social.Event
			if err := json.Unmarshal(msg.Body, &event); err != nil {
				log.Printf("decode event: %v", err)
				_ = msg.Nack(false, false)
				continue
			}

			var handleErr error
			switch event.Type {
			case social.EventLikeRequested, social.EventLikeCancelRequested:
				handleErr = handleLikeEvent(ctx, likeCache, minuteWindow, event)
			case social.EventCommentCreated, social.EventCommentDeleted:
				handleErr = handleCommentEvent(ctx, socialRepo, likeCache, commentCache, videoDetailCache, minuteWindow, event)
			default:
				log.Printf("skip unsupported event type=%s", event.Type)
				_ = msg.Ack(false)
				continue
			}

			if handleErr != nil {
				log.Printf("handle event type=%s: %v", event.Type, handleErr)
				_ = msg.Nack(false, true)
				continue
			}
			if err := msg.Ack(false); err != nil {
				log.Printf("ack event: %v", err)
			}
		}
	}
}

func handleLikeEvent(ctx context.Context, likeCache *cache.LikeStateCache, minuteWindow *cache.HotMinuteWindow, event social.Event) error {
	switch event.Type {
	case social.EventLikeRequested:
		added, err := likeCache.AddVideoUser(ctx, event.VideoID, event.ActorID)
		if err != nil {
			return err
		}
		if added {
			if _, err := likeCache.AddUserVideo(ctx, event.ActorID, event.VideoID); err != nil {
				return err
			}
			if _, err := likeCache.IncrementLikeCount(ctx, event.VideoID, 1); err != nil {
				return err
			}
			// Write to minute bucket for hot feed window.
			_ = minuteWindow.AddToBucket(ctx, time.Now(), event.VideoID, 5)
			return likeCache.MarkDirty(ctx, event.VideoID)
		}
		return nil
	case social.EventLikeCancelRequested:
		removed, err := likeCache.RemoveVideoUser(ctx, event.VideoID, event.ActorID)
		if err != nil {
			return err
		}
		if removed {
			if _, err := likeCache.RemoveUserVideo(ctx, event.ActorID, event.VideoID); err != nil {
				return err
			}
			if _, err := likeCache.IncrementLikeCount(ctx, event.VideoID, -1); err != nil {
				return err
			}
			return likeCache.MarkDirty(ctx, event.VideoID)
		}
		return nil
	default:
		return nil
	}
}

func handleCommentEvent(
	ctx context.Context,
	socialRepo *mysql.SocialRepository,
	likeCache *cache.LikeStateCache,
	commentCache *cache.CommentCache,
	videoDetailCache *cache.VideoDetailCache,
	minuteWindow *cache.HotMinuteWindow,
	event social.Event,
) error {
	switch event.Type {
	case social.EventCommentCreated:
		if _, err := likeCache.IncrementCommentCount(ctx, event.VideoID, 1); err != nil {
			return err
		}
		// Write to minute bucket for hot feed window.
		_ = minuteWindow.AddToBucket(ctx, time.Now(), event.VideoID, 3)
		// Invalidate caches.
		_ = commentCache.DeleteFirstPage(ctx, event.VideoID)
		_ = videoDetailCache.Delete(ctx, event.VideoID)
		return nil

	case social.EventCommentDeleted:
		if err := socialRepo.DeleteComment(ctx, event.CommentID, event.ActorID); err != nil {
			log.Printf("delete comment %d: %v", event.CommentID, err)
		}
		if _, err := likeCache.IncrementCommentCount(ctx, event.VideoID, -1); err != nil {
			return err
		}
		_ = commentCache.DeleteFirstPage(ctx, event.VideoID)
		_ = commentCache.DeleteDetail(ctx, event.CommentID)
		_ = videoDetailCache.Delete(ctx, event.VideoID)
		return nil

	default:
		return nil
	}
}

// aggregateLoop periodically slides the 60-minute window:
// adds the bucket from 1 minute ago, removes the bucket from 61 minutes ago.
func aggregateLoop(ctx context.Context, w *cache.HotMinuteWindow, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			addMinute := now.Add(-1 * time.Minute)
			removeMinute := now.Add(-61 * time.Minute)

			if err := w.ApplyBucket(ctx, addMinute, +1); err != nil {
				log.Printf("aggregate add bucket %s: %v", cache.MinuteKey(addMinute), err)
			}
			if err := w.ApplyBucket(ctx, removeMinute, -1); err != nil {
				log.Printf("aggregate remove bucket %s: %v", cache.MinuteKey(removeMinute), err)
			}
			if err := w.DeleteBucket(ctx, removeMinute); err != nil {
				log.Printf("delete bucket %s: %v", cache.MinuteKey(removeMinute), err)
			}
		}
	}
}

// cleanupLoop periodically prunes the aggregate ZSET.
func cleanupLoop(ctx context.Context, w *cache.HotMinuteWindow, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Cleanup(ctx); err != nil {
				log.Printf("cleanup hot window: %v", err)
			}
		}
	}
}

// syncLoop periodically reconciles dirty video like state from Redis to MySQL.
func syncLoop(ctx context.Context, db *gorm.DB, likeCache *cache.LikeStateCache, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	repo := mysql.NewSocialRepository(db)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := syncDirtyVideos(ctx, likeCache, repo); err != nil {
				log.Printf("sync dirty videos: %v", err)
			}
		}
	}
}

// syncDirtyVideos picks a batch of dirty videos, atomically snapshots their
// Redis state, and persists the like relationships to MySQL.
func syncDirtyVideos(ctx context.Context, likeCache *cache.LikeStateCache, repo *mysql.SocialRepository) error {
	dirtyIDs, err := likeCache.DirtyVideos(ctx, 100)
	if err != nil {
		return err
	}
	if len(dirtyIDs) == 0 {
		return nil
	}

	log.Printf("syncing %d dirty videos", len(dirtyIDs))

	for _, videoID := range dirtyIDs {
		snapshot, err := likeCache.SyncDirtyVideo(ctx, videoID)
		if err != nil {
			log.Printf("sync dirty video %d: %v", videoID, err)
			continue
		}
		if snapshot == nil {
			continue
		}

		// Persist current likers to MySQL.
		if err := repo.BatchUpsertLikes(ctx, snapshot.VideoID, snapshot.UserIDs); err != nil {
			log.Printf("batch upsert likes for video %d: %v", videoID, err)
			// Re-mark dirty so we retry next cycle.
			_ = likeCache.MarkDirty(ctx, videoID)
			continue
		}

		// Remove stale likes: users who unliked but still have active rows.
		if err := repo.DeleteLikesNotInSet(ctx, snapshot.VideoID, snapshot.UserIDs); err != nil {
			log.Printf("delete stale likes for video %d: %v", videoID, err)
			// Non-fatal: the stale rows will be cleaned on the next cycle.
		}

		log.Printf("synced video %d: %d likes", videoID, snapshot.LikeCount)
	}
	return nil
}
