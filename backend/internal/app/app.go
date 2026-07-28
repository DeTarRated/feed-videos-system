package app

import (
	"context"

	"gorm.io/gorm"

	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/domain/auth"
	"feed-videos-system/backend/internal/domain/user"
	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/infrastructure/mysql"
	"feed-videos-system/backend/internal/infrastructure/storage"
)

type App struct {
	Config  config.Config
	DB      *gorm.DB
	Storage *storage.LocalStore

	Auth   *auth.Service
	Users  *user.Service
	Videos *video.Service
}

func New(cfg config.Config, db *gorm.DB) *App {
	userRepo := mysql.NewUserRepository(db)
	videoRepo := mysql.NewVideoRepository(db)
	store := storage.NewLocalStore(cfg.StorageDir)

	return &App{
		Config:  cfg,
		DB:      db,
		Storage: store,
		Auth:    auth.NewService(cfg.JWTSecret),
		Users:   user.NewService(userRepo),
		Videos:  video.NewService(videoRepo),
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	_ = ctx
	if a.DB == nil {
		return nil
	}
	sqlDB, err := a.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
