package social

import (
	"context"
	"fmt"

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
