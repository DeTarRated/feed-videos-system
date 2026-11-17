package main

import (
	"context"
	"errors"
	"fmt"
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

	amqp "github.com/rabbitmq/amqp091-go"
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

	log.Printf("worker listening on queues %s, %s", cfg.RabbitMQLikeQueue, cfg.RabbitMQCommentQueue)

	likeCache := cache.NewLikeStateCache(redisClient)
	commentCache := cache.NewCommentCache(redisClient, 5*time.Minute)
	videoDetailCache := cache.NewVideoDetailCache(redisClient, cfg.VideoDetailCacheTTL)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-stopCh
		cancel()
	}()

	outcomeRepo := mysql.NewOutcomeRepository(db)
	replayCache := cache.NewInteractionReplayCache(redisClient, cfg.InteractionMarkerTTL)

	if err := consumeCommands(ctx, mqConn, cfg, outcomeRepo, replayCache, likeCache, commentCache, videoDetailCache); err != nil {
		log.Printf("worker stopped with error: %v", err)
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
		// Redis state is already updated by the replay script.
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
