package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func (r *SocialRepository) BatchUpsertLikes(ctx context.Context, videoID uint, userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}
	now := time.Now()
	items := make([]social.Like, 0, len(userIDs))
	for _, uid := range userIDs {
		items = append(items, social.Like{
			UserID:    uid,
			VideoID:   videoID,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "video_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"deleted_at": nil,
			"updated_at": now,
		}),
	}).Create(&items).Error
}

func (r *SocialRepository) DeleteLikesNotInSet(ctx context.Context, videoID uint, userIDs []uint) error {
	now := time.Now()
	if len(userIDs) == 0 {
		return r.db.WithContext(ctx).
			Model(&social.Like{}).
			Where("video_id = ? AND deleted_at IS NULL", videoID).
			Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
	}
	return r.db.WithContext(ctx).
		Model(&social.Like{}).
		Where("video_id = ? AND deleted_at IS NULL AND user_id NOT IN ?", videoID, userIDs).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
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

func (r *SocialRepository) CreateComment(ctx context.Context, comment *social.Comment) error {
	return r.db.WithContext(ctx).Create(comment).Error
}

func (r *SocialRepository) FindComment(ctx context.Context, commentID string) (*social.Comment, error) {
	var item social.Comment
	err := r.db.WithContext(ctx).
		Table("comments").
		Select("comments.*, users.nickname AS user_nickname").
		Joins("JOIN users ON users.id = comments.user_id").
		Where("comments.id = ? AND comments.deleted_at IS NULL", commentID).
		First(&item).Error
	return &item, err
}

func (r *SocialRepository) ListComments(ctx context.Context, videoID uint, limit int) ([]social.Comment, error) {
	var items []social.Comment
	err := r.db.WithContext(ctx).
		Table("comments").
		Select("comments.*, users.nickname AS user_nickname").
		Joins("JOIN users ON users.id = comments.user_id").
		Where("comments.video_id = ? AND comments.deleted_at IS NULL", videoID).
		Order("comments.created_at DESC").
		Order("comments.id DESC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *SocialRepository) DeleteComment(ctx context.Context, commentID string, userID uint) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&social.Comment{}).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", commentID, userID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *SocialRepository) CountVideoComments(ctx context.Context, videoID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&social.Comment{}).
		Where("video_id = ? AND deleted_at IS NULL", videoID).
		Count(&count).Error
	return count, err
}

func (r *SocialRepository) FollowUser(ctx context.Context, followerID uint, followingID uint) error {
	now := time.Now()
	item := social.Follow{
		FollowerID:  followerID,
		FollowingID: followingID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "follower_id"}, {Name: "following_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"deleted_at": nil,
			"updated_at": now,
		}),
	}).Create(&item).Error
}

func (r *SocialRepository) UnfollowUser(ctx context.Context, followerID uint, followingID uint) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&social.Follow{}).
		Where("follower_id = ? AND following_id = ? AND deleted_at IS NULL", followerID, followingID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
}

func (r *SocialRepository) IsFollowing(ctx context.Context, followerID uint, followingID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&social.Follow{}).
		Where("follower_id = ? AND following_id = ? AND deleted_at IS NULL", followerID, followingID).
		Count(&count).Error
	return count > 0, err
}

func (r *SocialRepository) CountFollowers(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&social.Follow{}).
		Where("following_id = ? AND deleted_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

func (r *SocialRepository) CountFollowing(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&social.Follow{}).
		Where("follower_id = ? AND deleted_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

func (r *SocialRepository) LikeStateWithVersion(ctx context.Context, userID, videoID uint) (liked bool, opVersion uint64, err error) {
	var like social.Like
	err = r.db.WithContext(ctx).
		Model(&social.Like{}).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		First(&like).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, 0, nil
		}
		return false, 0, err
	}
	return like.DeletedAt == nil, like.OpVersion, nil
}

func (r *SocialRepository) LikeOpVersion(ctx context.Context, userID, videoID uint) (uint64, error) {
	var version uint64
	err := r.db.WithContext(ctx).
		Model(&social.Like{}).
		Select("COALESCE(MAX(op_version), 0)").
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Scan(&version).Error
	return version, err
}

func (r *SocialRepository) LastCommittedVersion(ctx context.Context, key string) (uint64, error) {
	if strings.HasPrefix(key, "like:") {
		parts := strings.Split(key, ":")
		if len(parts) != 3 {
			return 0, nil
		}
		var userID, videoID uint64
		if _, err := fmt.Sscan(parts[1], &userID); err != nil {
			return 0, nil
		}
		if _, err := fmt.Sscan(parts[2], &videoID); err != nil {
			return 0, nil
		}
		var version uint64
		err := r.db.WithContext(ctx).
			Model(&social.Like{}).
			Select("COALESCE(MAX(op_version), 0)").
			Where("user_id = ? AND video_id = ?", userID, videoID).
			Scan(&version).Error
		return version, err
	}
	if strings.HasPrefix(key, "comment:") {
		commentID := strings.TrimPrefix(key, "comment:")
		var version uint64
		err := r.db.WithContext(ctx).
			Model(&social.Comment{}).
			Select("COALESCE(MAX(op_version), 0)").
			Where("id = ?", commentID).
			Scan(&version).Error
		return version, err
	}
	return 0, nil
}

func (r *SocialRepository) ListFollowers(ctx context.Context, userID uint, limit int) ([]social.FollowUser, error) {
	var items []social.FollowUser
	err := r.db.WithContext(ctx).
		Table("users").
		Select("users.id, users.username, users.nickname, users.avatar_url, users.bio").
		Joins("JOIN follows ON follows.follower_id = users.id").
		Where("follows.following_id = ? AND follows.deleted_at IS NULL", userID).
		Order("follows.updated_at DESC").
		Order("users.id DESC").
		Limit(limit).
		Scan(&items).Error
	return items, err
}

func (r *SocialRepository) ListFollowing(ctx context.Context, userID uint, limit int) ([]social.FollowUser, error) {
	var items []social.FollowUser
	err := r.db.WithContext(ctx).
		Table("users").
		Select("users.id, users.username, users.nickname, users.avatar_url, users.bio").
		Joins("JOIN follows ON follows.following_id = users.id").
		Where("follows.follower_id = ? AND follows.deleted_at IS NULL", userID).
		Order("follows.updated_at DESC").
		Order("users.id DESC").
		Limit(limit).
		Scan(&items).Error
	return items, err
}
