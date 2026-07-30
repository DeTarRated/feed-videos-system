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
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	if _, err := mysql.Open(cfg.MySQLDSN); err != nil {
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
