package social

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/infrastructure/cache"
)

type EventPublisher interface {
	PublishLikeEvent(ctx context.Context, event Event) error
}

type Service struct {
	repo           Repository
	publisher      EventPublisher
	likeStateCache *cache.LikeStateCache
	sfGroup        singleflight.Group
}

func NewService(repo Repository, publisher EventPublisher, likeStateCache *cache.LikeStateCache) *Service {
	return &Service{repo: repo, publisher: publisher, likeStateCache: likeStateCache}
}

func (s *Service) LikeVideo(ctx context.Context, userID uint, videoID uint) (*LikeStatus, error) {
	if userID == 0 || videoID == 0 {
		return nil, fmt.Errorf("user id and video id are required")
	}
	if err := s.publish(ctx, Event{
		EventID:    buildEventID("like", userID, videoID),
		Type:       EventLikeRequested,
		ActorID:    userID,
		VideoID:    videoID,
		Delta:      1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	return &LikeStatus{VideoID: videoID, Liked: true}, nil
}

func (s *Service) UnlikeVideo(ctx context.Context, userID uint, videoID uint) (*LikeStatus, error) {
	if userID == 0 || videoID == 0 {
		return nil, fmt.Errorf("user id and video id are required")
	}
	if err := s.publish(ctx, Event{
		EventID:    buildEventID("unlike", userID, videoID),
		Type:       EventLikeCancelRequested,
		ActorID:    userID,
		VideoID:    videoID,
		Delta:      -1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		return nil, err
	}
	return &LikeStatus{VideoID: videoID, Liked: false}, nil
}

func (s *Service) Status(ctx context.Context, userID uint, videoID uint) (*LikeStatus, error) {
	if videoID == 0 {
		return nil, fmt.Errorf("video id is required")
	}

	// Redis first: try cache for both values.
	if liked, count, err := s.statusFromCache(ctx, userID, videoID); err == nil {
		return &LikeStatus{VideoID: videoID, Liked: liked, LikeCount: count}, nil
	}

	// Redis failed — fallback to MySQL, guarded by singleflight to prevent thundering herd.
	key := fmt.Sprintf("db:like:status:%d:%d", userID, videoID)
	v, err, _ := s.sfGroup.Do(key, func() (interface{}, error) {
		return s.statusFromDB(ctx, userID, videoID)
	})
	if err != nil {
		return nil, err
	}
	return v.(*LikeStatus), nil
}

// statusFromCache reads the like status and count from Redis.
// Returns a non-nil error if either call fails, so the caller falls back to MySQL.
func (s *Service) statusFromCache(ctx context.Context, userID uint, videoID uint) (bool, int64, error) {
	if s.likeStateCache == nil {
		return false, 0, fmt.Errorf("cache unavailable")
	}
	var liked bool
	var err error
	if userID > 0 {
		liked, err = s.likeStateCache.IsLiked(ctx, videoID, userID)
		if err != nil {
			return false, 0, err
		}
	}
	count, err := s.likeStateCache.GetLikeCount(ctx, videoID)
	if err != nil {
		return false, 0, err
	}
	return liked, count, nil
}

// statusFromDB reads the like status and count from MySQL.
// Original Status() body, kept as the fallback path.
func (s *Service) statusFromDB(ctx context.Context, userID uint, videoID uint) (*LikeStatus, error) {
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
	created, err := s.repo.FindComment(ctx, comment.ID)
	if err != nil {
		return nil, err
	}
	return created, nil
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

func (s *Service) FindComment(ctx context.Context, commentID uint) (*Comment, error) {
	if commentID == 0 {
		return nil, fmt.Errorf("comment id is required")
	}
	return s.repo.FindComment(ctx, commentID)
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

func buildEventID(prefix string, userID, videoID uint) string {
	return fmt.Sprintf("%s:%d:%d:%d", prefix, userID, videoID, time.Now().UnixNano())
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

func (s *Service) publish(ctx context.Context, event Event) error {
	if s.publisher == nil {
		return nil
	}
	return s.publisher.PublishLikeEvent(ctx, event)
}
