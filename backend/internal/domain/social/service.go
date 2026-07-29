package social

import (
	"context"
	"fmt"
	"strings"

	"feed-videos-system/backend/internal/domain/video"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) LikeVideo(ctx context.Context, userID uint, videoID uint) (*LikeStatus, error) {
	if userID == 0 || videoID == 0 {
		return nil, fmt.Errorf("user id and video id are required")
	}
	if err := s.repo.LikeVideo(ctx, userID, videoID); err != nil {
		return nil, err
	}
	return s.Status(ctx, userID, videoID)
}

func (s *Service) UnlikeVideo(ctx context.Context, userID uint, videoID uint) (*LikeStatus, error) {
	if userID == 0 || videoID == 0 {
		return nil, fmt.Errorf("user id and video id are required")
	}
	if err := s.repo.UnlikeVideo(ctx, userID, videoID); err != nil {
		return nil, err
	}
	return s.Status(ctx, userID, videoID)
}

func (s *Service) Status(ctx context.Context, userID uint, videoID uint) (*LikeStatus, error) {
	if videoID == 0 {
		return nil, fmt.Errorf("video id is required")
	}
	liked := false
	var err error
	if userID > 0 {
		liked, err = s.repo.IsLiked(ctx, userID, videoID)
		if err != nil {
			return nil, err
		}
	}
	count, err := s.repo.CountVideoLikes(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return &LikeStatus{VideoID: videoID, Liked: liked, LikeCount: count}, nil
}

func (s *Service) CountAuthorReceivedLikes(ctx context.Context, authorID uint) (int64, error) {
	if authorID == 0 {
		return 0, fmt.Errorf("author id is required")
	}
	return s.repo.CountAuthorReceivedLikes(ctx, authorID)
}

func (s *Service) LikedVideos(ctx context.Context, userID uint, limit int) ([]video.Video, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user id is required")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.repo.ListLikedVideos(ctx, userID, limit)
}

func (s *Service) CreateComment(ctx context.Context, userID uint, videoID uint, content string) (*Comment, error) {
	content = strings.TrimSpace(content)
	if userID == 0 || videoID == 0 {
		return nil, fmt.Errorf("user id and video id are required")
	}
	if content == "" {
		return nil, fmt.Errorf("comment content is required")
	}
	if len([]rune(content)) > 500 {
		return nil, fmt.Errorf("comment content is too long")
	}

	comment := &Comment{UserID: userID, VideoID: videoID, Content: content}
	if err := s.repo.CreateComment(ctx, comment); err != nil {
		return nil, err
	}
	return s.repo.FindComment(ctx, comment.ID)
}

func (s *Service) ListComments(ctx context.Context, videoID uint, limit int) ([]Comment, error) {
	if videoID == 0 {
		return nil, fmt.Errorf("video id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListComments(ctx, videoID, limit)
}

func (s *Service) DeleteComment(ctx context.Context, commentID uint, userID uint) error {
	if commentID == 0 || userID == 0 {
		return fmt.Errorf("comment id and user id are required")
	}
	return s.repo.DeleteComment(ctx, commentID, userID)
}

func (s *Service) CountVideoComments(ctx context.Context, videoID uint) (int64, error) {
	if videoID == 0 {
		return 0, fmt.Errorf("video id is required")
	}
	return s.repo.CountVideoComments(ctx, videoID)
}

func (s *Service) FollowUser(ctx context.Context, followerID uint, followingID uint) (*FollowStatus, error) {
	if err := validateFollowIDs(followerID, followingID); err != nil {
		return nil, err
	}
	if err := s.repo.FollowUser(ctx, followerID, followingID); err != nil {
		return nil, err
	}
	return s.FollowStatus(ctx, followerID, followingID)
}

func (s *Service) UnfollowUser(ctx context.Context, followerID uint, followingID uint) (*FollowStatus, error) {
	if err := validateFollowIDs(followerID, followingID); err != nil {
		return nil, err
	}
	if err := s.repo.UnfollowUser(ctx, followerID, followingID); err != nil {
		return nil, err
	}
	return s.FollowStatus(ctx, followerID, followingID)
}

func (s *Service) FollowStatus(ctx context.Context, followerID uint, followingID uint) (*FollowStatus, error) {
	if followingID == 0 {
		return nil, fmt.Errorf("following user id is required")
	}

	following := false
	var err error
	if followerID > 0 && followerID != followingID {
		following, err = s.repo.IsFollowing(ctx, followerID, followingID)
		if err != nil {
			return nil, err
		}
	}
	followerCount, err := s.repo.CountFollowers(ctx, followingID)
	if err != nil {
		return nil, err
	}
	return &FollowStatus{UserID: followingID, Following: following, FollowerCount: followerCount}, nil
}

func (s *Service) CountFollowers(ctx context.Context, userID uint) (int64, error) {
	if userID == 0 {
		return 0, fmt.Errorf("user id is required")
	}
	return s.repo.CountFollowers(ctx, userID)
}

func (s *Service) CountFollowing(ctx context.Context, userID uint) (int64, error) {
	if userID == 0 {
		return 0, fmt.Errorf("user id is required")
	}
	return s.repo.CountFollowing(ctx, userID)
}

func (s *Service) Followers(ctx context.Context, userID uint, limit int) ([]FollowUser, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user id is required")
	}
	return s.repo.ListFollowers(ctx, userID, normalizeFollowLimit(limit))
}

func (s *Service) Following(ctx context.Context, userID uint, limit int) ([]FollowUser, error) {
	if userID == 0 {
		return nil, fmt.Errorf("user id is required")
	}
	return s.repo.ListFollowing(ctx, userID, normalizeFollowLimit(limit))
}

func validateFollowIDs(followerID uint, followingID uint) error {
	if followerID == 0 || followingID == 0 {
		return fmt.Errorf("follower id and following id are required")
	}
	if followerID == followingID {
		return fmt.Errorf("cannot follow yourself")
	}
	return nil
}

func normalizeFollowLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 50
	}
	return limit
}
