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

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-stopCh
		cancel()
	}()

	// Start periodic batch MySQL sync.
	go syncLoop(ctx, db, likeCache, 30*time.Second)

	if err := consumeEvents(ctx, mqConn, cfg, likeCache); err != nil {
		log.Printf("worker stopped with error: %v", err)
	}
}

func consumeEvents(ctx context.Context, conn *mq.Connection, cfg config.Config, likeCache *cache.LikeStateCache) error {
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
			if event.Type != social.EventLikeRequested && event.Type != social.EventLikeCancelRequested {
				log.Printf("skip unsupported event type=%s", event.Type)
				_ = msg.Ack(false)
				continue
			}
			if err := handleLikeEvent(ctx, likeCache, event); err != nil {
				log.Printf("handle like event: %v", err)
				_ = msg.Nack(false, true)
				continue
			}
			if err := msg.Ack(false); err != nil {
				log.Printf("ack event: %v", err)
			}
		}
	}
}

func handleLikeEvent(ctx context.Context, likeCache *cache.LikeStateCache, event social.Event) error {
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
