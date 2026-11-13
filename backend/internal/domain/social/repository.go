package social

import (
	"context"

	"feed-videos-system/backend/internal/domain/video"
)

type Repository interface {
	LikeVideo(ctx context.Context, userID uint, videoID uint) error
	UnlikeVideo(ctx context.Context, userID uint, videoID uint) error
	BatchUpsertLikes(ctx context.Context, videoID uint, userIDs []uint) error
	DeleteLikesNotInSet(ctx context.Context, videoID uint, userIDs []uint) error
	IsLiked(ctx context.Context, userID uint, videoID uint) (bool, error)
	CountVideoLikes(ctx context.Context, videoID uint) (int64, error)
	CountAuthorReceivedLikes(ctx context.Context, authorID uint) (int64, error)
	ListLikedVideos(ctx context.Context, userID uint, limit int) ([]video.Video, error)
	CreateComment(ctx context.Context, comment *Comment) error
	FindComment(ctx context.Context, commentID string) (*Comment, error)
	ListComments(ctx context.Context, videoID uint, limit int) ([]Comment, error)
	DeleteComment(ctx context.Context, commentID string, userID uint) error
	CountVideoComments(ctx context.Context, videoID uint) (int64, error)
	LastCommittedVersion(ctx context.Context, key string) (uint64, error)
	FollowUser(ctx context.Context, followerID uint, followingID uint) error
	UnfollowUser(ctx context.Context, followerID uint, followingID uint) error
	IsFollowing(ctx context.Context, followerID uint, followingID uint) (bool, error)
	CountFollowers(ctx context.Context, userID uint) (int64, error)
	CountFollowing(ctx context.Context, userID uint) (int64, error)
	ListFollowers(ctx context.Context, userID uint, limit int) ([]FollowUser, error)
	ListFollowing(ctx context.Context, userID uint, limit int) ([]FollowUser, error)
}
