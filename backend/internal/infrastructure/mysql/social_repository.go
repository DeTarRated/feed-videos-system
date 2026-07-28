package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"feed-videos-system/backend/internal/domain/social"
	"feed-videos-system/backend/internal/domain/video"
)

type SocialRepository struct {
	db *gorm.DB
}

func NewSocialRepository(db *gorm.DB) *SocialRepository {
	return &SocialRepository{db: db}
}

func (r *SocialRepository) LikeVideo(ctx context.Context, userID uint, videoID uint) error {
	now := time.Now()
	item := social.Like{
		UserID:    userID,
		VideoID:   videoID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "video_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"deleted_at": nil,
			"updated_at": now,
		}),
	}).Create(&item).Error
}

func (r *SocialRepository) UnlikeVideo(ctx context.Context, userID uint, videoID uint) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&social.Like{}).
		Where("user_id = ? AND video_id = ? AND deleted_at IS NULL", userID, videoID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
}

func (r *SocialRepository) IsLiked(ctx context.Context, userID uint, videoID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&social.Like{}).
		Where("user_id = ? AND video_id = ? AND deleted_at IS NULL", userID, videoID).
		Count(&count).Error
	return count > 0, err
}

func (r *SocialRepository) CountVideoLikes(ctx context.Context, videoID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&social.Like{}).
		Where("video_id = ? AND deleted_at IS NULL", videoID).
		Count(&count).Error
	return count, err
}

func (r *SocialRepository) CountAuthorReceivedLikes(ctx context.Context, authorID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("likes").
		Joins("JOIN videos ON videos.id = likes.video_id").
		Where("videos.author_id = ? AND likes.deleted_at IS NULL", authorID).
		Count(&count).Error
	return count, err
}

func (r *SocialRepository) ListLikedVideos(ctx context.Context, userID uint, limit int) ([]video.Video, error) {
	var items []video.Video
	err := r.db.WithContext(ctx).
		Table("videos").
		Select("videos.*").
		Joins("JOIN likes ON likes.video_id = videos.id").
		Where("likes.user_id = ? AND likes.deleted_at IS NULL", userID).
		Order("likes.updated_at DESC").
		Order("videos.id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}
