package social

import (
	"context"

	"feed-videos-system/backend/internal/domain/video"
)

type Repository interface {
	LikeVideo(ctx context.Context, userID uint, videoID uint) error
	UnlikeVideo(ctx context.Context, userID uint, videoID uint) error
	IsLiked(ctx context.Context, userID uint, videoID uint) (bool, error)
	CountVideoLikes(ctx context.Context, videoID uint) (int64, error)
	CountAuthorReceivedLikes(ctx context.Context, authorID uint) (int64, error)
	ListLikedVideos(ctx context.Context, userID uint, limit int) ([]video.Video, error)
	CreateComment(ctx context.Context, comment *Comment) error
	FindComment(ctx context.Context, commentID uint) (*Comment, error)
	ListComments(ctx context.Context, videoID uint, limit int) ([]Comment, error)
	DeleteComment(ctx context.Context, commentID uint, userID uint) error
	CountVideoComments(ctx context.Context, videoID uint) (int64, error)
}
