package mysql

import (
	"context"

	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/video"
)

type VideoRepository struct {
	db *gorm.DB
}

func NewVideoRepository(db *gorm.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

func (r *VideoRepository) Create(ctx context.Context, v *video.Video) error {
	return r.db.WithContext(ctx).Create(v).Error
}

func (r *VideoRepository) FindByID(ctx context.Context, id uint) (*video.Video, error) {
	var v video.Video
	if err := r.db.WithContext(ctx).First(&v, id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *VideoRepository) ListLatest(ctx context.Context, limit int) ([]video.Video, error) {
	var items []video.Video
	err := r.db.WithContext(ctx).
		Order("published_at DESC").
		Order("id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *VideoRepository) ListByAuthor(ctx context.Context, authorID uint, limit int) ([]video.Video, error) {
	var items []video.Video
	err := r.db.WithContext(ctx).
		Where("author_id = ?", authorID).
		Order("published_at DESC").
		Order("id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *VideoRepository) CountByAuthor(ctx context.Context, authorID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&video.Video{}).
		Where("author_id = ?", authorID).
		Count(&count).Error
	return count, err
}
