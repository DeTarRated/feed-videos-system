package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/domain/social"
	"feed-videos-system/backend/internal/infrastructure/cache"
	"feed-videos-system/backend/internal/infrastructure/mq"
	"feed-videos-system/backend/internal/infrastructure/mysql"
	"feed-videos-system/backend/internal/infrastructure/redis"

	amqp "github.com/rabbitmq/amqp091-go"
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

	outcomeRepo := mysql.NewOutcomeRepository(db)
	replayCache := cache.NewInteractionReplayCache(redisClient, cfg.InteractionMarkerTTL)

	errCh := make(chan error, 2)
	go func() {
		errCh <- consumeCommands(ctx, mqConn, cfg, outcomeRepo, replayCache, likeCache, commentCache, videoDetailCache)
	}()
	go func() {
		errCh <- consumeEvents(ctx, mqConn, cfg, likeCache, commentCache, videoDetailCache, minuteWindow, socialRepo)
	}()

	if err := <-errCh; err != nil {
		log.Printf("worker stopped with error: %v", err)
		cancel()
	}
}

func consumeCommands(
	ctx context.Context,
	conn *mq.Connection,
	cfg config.Config,
	outcomeRepo *mysql.OutcomeRepository,
	replayCache *cache.InteractionReplayCache,
	likeCache *cache.LikeStateCache,
	commentCache *cache.CommentCache,
	videoDetailCache *cache.VideoDetailCache,
) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	topology := mq.CommandTopologyConfig{
		Exchange:          cfg.RabbitMQCommandExchange,
		LikeQueue:         cfg.RabbitMQLikeQueue,
		LikeRoutingKey:    cfg.RabbitMQLikeRoutingKey,
		CommentQueue:      cfg.RabbitMQCommentQueue,
		CommentRoutingKey: cfg.RabbitMQCommentRoutingKey,
		RetryQueue:        cfg.RabbitMQCommandRetryQueue,
		RetryTTL:          cfg.RabbitMQCommandRetryTTL,
		DLQ:               cfg.RabbitMQCommandDLQ,
	}
	if err := mq.DeclareCommandTopology(ch, topology); err != nil {
		return err
	}
	if err := ch.Qos(cfg.RabbitMQCommandPrefetch, 0, false); err != nil {
		return err
	}
	likeQueue := cfg.RabbitMQLikeQueue
	commentQueue := cfg.RabbitMQCommentQueue

	likeMsgs, err := ch.Consume(likeQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	commentMsgs, err := ch.Consume(commentQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-likeMsgs:
			if !ok {
				return nil
			}
			if err := handleCommandWithRetry(ctx, msg, outcomeRepo, replayCache, likeCache, commentCache, videoDetailCache, cfg.RabbitMQCommandRetryTTL); err != nil {
				log.Printf("handle like command: %v", err)
				nackCommand(msg, err)
				continue
			}
			_ = msg.Ack(false)
		case msg, ok := <-commentMsgs:
			if !ok {
				return nil
			}
			if err := handleCommandWithRetry(ctx, msg, outcomeRepo, replayCache, likeCache, commentCache, videoDetailCache, cfg.RabbitMQCommandRetryTTL); err != nil {
				log.Printf("handle comment command: %v", err)
				nackCommand(msg, err)
				continue
			}
			_ = msg.Ack(false)
		}
	}
}

func handleCommandWithRetry(
	ctx context.Context,
	msg amqp.Delivery,
	outcomeRepo *mysql.OutcomeRepository,
	replayCache *cache.InteractionReplayCache,
	likeCache *cache.LikeStateCache,
	commentCache *cache.CommentCache,
	videoDetailCache *cache.VideoDetailCache,
	retryTTL time.Duration,
) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		lastErr = handleCommandMessage(ctx, msg, outcomeRepo, replayCache, likeCache, commentCache, videoDetailCache)
		if lastErr == nil {
			return nil
		}
		if isNonRetryableCommandError(lastErr) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryTTL):
		}
	}
	return lastErr
}

func nackCommand(msg amqp.Delivery, cause error) {
	// Command queues are declared with a dead-letter exchange pointing to the retry queue.
	// Nack without requeue routes failed messages to retry; retry TTL routes them to the DLQ.
	_ = cause
	_ = msg.Nack(false, false)
}

func isNonRetryableCommandError(err error) bool {
	var cmdErr *social.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.NonRetryable
	}
	return false
}

func handleCommandMessage(
	ctx context.Context,
	msg amqp.Delivery,
	outcomeRepo *mysql.OutcomeRepository,
	replayCache *cache.InteractionReplayCache,
	likeCache *cache.LikeStateCache,
	commentCache *cache.CommentCache,
	videoDetailCache *cache.VideoDetailCache,
) error {
	cmd, err := social.UnmarshalCommand(msg.Body)
	if err != nil {
		return social.NewNonRetryableCommandError(err)
	}
	now := time.Now().UTC()
	var outcome *mysql.InteractionOutcome
	switch cmd.Type {
	case social.CommandLikeCreate, social.CommandLikeCancel:
		outcome, err = outcomeRepo.ProcessLike(ctx, cmd, now)
	case social.CommandCommentCreate:
		outcome, err = outcomeRepo.ProcessCommentCreate(ctx, cmd, now)
	case social.CommandCommentDelete:
		outcome, err = outcomeRepo.ProcessCommentDelete(ctx, cmd, now)
	default:
		return social.NewNonRetryableCommandError(fmt.Errorf("unsupported command type: %s", cmd.Type))
	}
	if err != nil {
		if errors.Is(err, mysql.ErrOutcomeFingerprintMismatch) {
			return social.NewNonRetryableCommandError(err)
		}
		return err
	}
	if _, err := replayCache.ApplyOutcome(ctx, mysqlOutcomeToReplayView(*outcome, cmd)); err != nil {
		return err
	}

	switch cmd.Type {
	case social.CommandLikeCreate, social.CommandLikeCancel:
		if outcome.DBOutcome == "applied" {
			if err := likeCache.MarkDirty(ctx, cmd.VideoID); err != nil {
				return err
			}
		}
	case social.CommandCommentCreate:
		_ = commentCache.DeleteFirstPage(ctx, cmd.VideoID)
		_ = videoDetailCache.Delete(ctx, cmd.VideoID)
	case social.CommandCommentDelete:
		_ = commentCache.DeleteDetail(ctx, cmd.CommentID)
		_ = commentCache.DeleteFirstPage(ctx, cmd.VideoID)
		_ = videoDetailCache.Delete(ctx, cmd.VideoID)
	}
	return nil
}

func mysqlOutcomeToReplayView(outcome mysql.InteractionOutcome, cmd social.Command) cache.InteractionOutcomeView {
	return cache.InteractionOutcomeView{
		OperationID:  outcome.OperationID,
		VideoID:      outcome.VideoID,
		ActorID:      cmd.ActorID,
		CommandType:  outcome.CommandType,
		OpVersion:    outcome.OpVersion,
		LikeCount:    outcome.LikeCount,
		CommentCount: outcome.CommentCount,
		StatsVersion: outcome.StatsVersion,
		HeatDelta:    outcome.HeatDelta,
		HeatMinute:   outcome.HeatMinute,
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
		legacyCommentID := strconv.FormatUint(uint64(event.CommentID), 10)
		if err := socialRepo.DeleteComment(ctx, legacyCommentID, event.ActorID); err != nil {
			log.Printf("delete comment %s: %v", legacyCommentID, err)
		}
		if _, err := likeCache.IncrementCommentCount(ctx, event.VideoID, -1); err != nil {
			return err
		}
		_ = commentCache.DeleteFirstPage(ctx, event.VideoID)
		_ = commentCache.DeleteDetail(ctx, strconv.FormatUint(uint64(event.CommentID), 10))
		_ = videoDetailCache.Delete(ctx, event.VideoID)
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
