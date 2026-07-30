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
	"feed-videos-system/backend/internal/infrastructure/mysql"
	"feed-videos-system/backend/internal/infrastructure/storage"
)

type App struct {
	Config           config.Config
	DB               *gorm.DB
	Redis            *goredis.Client
	Storage          *storage.LocalStore
	VideoDetailCache *cache.VideoDetailCache
	HotVideoRank     *cache.HotVideoRank
	HotFeedSnapshots *cache.HotFeedSnapshotStore

	Auth   *auth.Service
	Users  *user.Service
	Videos *video.Service
	Social *social.Service
}

func New(cfg config.Config, db *gorm.DB, redisClient *goredis.Client) *App {
	userRepo := mysql.NewUserRepository(db)
	videoRepo := mysql.NewVideoRepository(db)
	socialRepo := mysql.NewSocialRepository(db)
	store := storage.NewLocalStore(cfg.StorageDir)

	return &App{
		Config:           cfg,
		DB:               db,
		Redis:            redisClient,
		Storage:          store,
		VideoDetailCache: cache.NewVideoDetailCache(redisClient, cfg.VideoDetailCacheTTL),
		HotVideoRank:     cache.NewHotVideoRank(redisClient),
		HotFeedSnapshots: cache.NewHotFeedSnapshotStore(redisClient, 5*time.Minute),
		Auth:             auth.NewService(cfg.JWTSecret),
		Users:            user.NewService(userRepo),
		Videos:           video.NewService(videoRepo, cache.NewHotVideoRank(redisClient), cache.NewHotFeedSnapshotStore(redisClient, 5*time.Minute)),
		Social:           social.NewService(socialRepo),
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	_ = ctx
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
