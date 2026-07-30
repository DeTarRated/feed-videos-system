package app

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/domain/auth"
	"feed-videos-system/backend/internal/domain/social"
	"feed-videos-system/backend/internal/domain/user"
	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/infrastructure/cache"
	"feed-videos-system/backend/internal/infrastructure/mq"
	"feed-videos-system/backend/internal/infrastructure/mysql"
	"feed-videos-system/backend/internal/infrastructure/storage"
)

type App struct {
	Config           config.Config
	DB               *gorm.DB
	Redis            *goredis.Client
	MQ               *mq.Connection
	Storage          *storage.LocalStore
	VideoDetailCache *cache.VideoDetailCache
	HotVideoRank     *cache.HotVideoRank
	HotMinuteWindow  *cache.HotMinuteWindow
	HotFeedSnapshots *cache.HotFeedSnapshotStore
	LikeStateCache   *cache.LikeStateCache
	CommentCache     *cache.CommentCache

	Auth   *auth.Service
	Users  *user.Service
	Videos *video.Service
	Social *social.Service
}

func New(cfg config.Config, db *gorm.DB, redisClient *goredis.Client, mqConn *mq.Connection) *App {
	userRepo := mysql.NewUserRepository(db)
	videoRepo := mysql.NewVideoRepository(db)
	socialRepo := mysql.NewSocialRepository(db)
	store := storage.NewLocalStore(cfg.StorageDir)
	publisher := mq.NewPublisher(mqConn, cfg.RabbitMQExchange, cfg.RabbitMQRoutingKey)
	likeStateCache := cache.NewLikeStateCache(redisClient, 30*time.Minute)
	commentCache := cache.NewCommentCache(redisClient, 5*time.Minute)
	hotWindow := cache.NewHotMinuteWindow(redisClient, 70*time.Minute)

	return &App{
		Config:           cfg,
		DB:               db,
		Redis:            redisClient,
		MQ:               mqConn,
		Storage:          store,
		VideoDetailCache: cache.NewVideoDetailCache(redisClient, cfg.VideoDetailCacheTTL),
		HotVideoRank:     cache.NewHotVideoRank(redisClient),
		HotMinuteWindow:  hotWindow,
		HotFeedSnapshots: cache.NewHotFeedSnapshotStore(redisClient, 5*time.Minute),
		LikeStateCache:   likeStateCache,
		CommentCache:     commentCache,
		Auth:             auth.NewService(cfg.JWTSecret),
		Users:            user.NewService(userRepo),
		Videos:           video.NewService(videoRepo, hotWindow, cache.NewHotFeedSnapshotStore(redisClient, 5*time.Minute)),
		Social:           social.NewService(socialRepo, publisher, likeStateCache, commentCache),
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	_ = ctx
	if a.MQ != nil {
		if err := a.MQ.Close(); err != nil {
			return err
		}
	}
	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil {
			return err
		}
	}
	if a.DB == nil {
		return nil
	}
	sqlDB, err := a.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
