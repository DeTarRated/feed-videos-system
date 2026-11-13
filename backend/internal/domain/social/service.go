package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/infrastructure/cache"
)

var ErrCommentNotReady = errors.New("comment_not_ready")

type CommandPublisher interface {
	PublishCommandWithConfirm(ctx context.Context, routingKey string, body []byte, timeout time.Duration) error
}

type Service struct {
	repo              Repository
	commandPublisher  CommandPublisher
	idempotency       *cache.IdempotencyStore
	versioner         *cache.VersionSequencer
	confirmTimeout    time.Duration
	likeRoutingKey    string
	commentRoutingKey string
	likeStateCache    *cache.LikeStateCache
	commentCache      *cache.CommentCache
	sfGroup           singleflight.Group
}

func NewService(
	repo Repository,
	commandPublisher CommandPublisher,
	idempotency *cache.IdempotencyStore,
	versioner *cache.VersionSequencer,
	confirmTimeout time.Duration,
	likeRoutingKey string,
	commentRoutingKey string,
	likeStateCache *cache.LikeStateCache,
	commentCache *cache.CommentCache,
) *Service {
	return &Service{
		repo:              repo,
		commandPublisher:  commandPublisher,
		idempotency:       idempotency,
		versioner:         versioner,
		confirmTimeout:    confirmTimeout,
		likeRoutingKey:    likeRoutingKey,
		commentRoutingKey: commentRoutingKey,
		likeStateCache:    likeStateCache,
		commentCache:      commentCache,
	}
}

func (s *Service) LikeVideo(ctx context.Context, userID uint, videoID uint, idempotencyKey string) (*LikeStatus, error) {
	if userID == 0 || videoID == 0 {
		return nil, fmt.Errorf("user id and video id are required")
	}
	cmd, state, err := s.buildOrReuseCommand(ctx, CommandLikeCreate, userID, videoID, idempotencyKey, "", LikeStatusKey(userID, videoID))
	if err != nil {
		return nil, err
	}
	if state == commandStateReused {
		return &LikeStatus{VideoID: videoID, Liked: true, Status: "pending", OperationID: cmd.OperationID, OpVersion: formatVersion(cmd.OpVersion)}, nil
	}
	if err := s.publishCommandAndConfirm(ctx, s.likeRoutingKey, mustMarshalCommand(cmd)); err != nil {
		return nil, err
	}
	return &LikeStatus{VideoID: videoID, Liked: true, Status: "pending", OperationID: cmd.OperationID, OpVersion: formatVersion(cmd.OpVersion)}, nil
}

func (s *Service) UnlikeVideo(ctx context.Context, userID uint, videoID uint, idempotencyKey string) (*LikeStatus, error) {
	if userID == 0 || videoID == 0 {
		return nil, fmt.Errorf("user id and video id are required")
	}
	cmd, state, err := s.buildOrReuseCommand(ctx, CommandLikeCancel, userID, videoID, idempotencyKey, "", LikeStatusKey(userID, videoID))
	if err != nil {
		return nil, err
	}
	if state == commandStateReused {
		return &LikeStatus{VideoID: videoID, Liked: false, Status: "pending", OperationID: cmd.OperationID, OpVersion: formatVersion(cmd.OpVersion)}, nil
	}
	if err := s.publishCommandAndConfirm(ctx, s.likeRoutingKey, mustMarshalCommand(cmd)); err != nil {
		return nil, err
	}
	return &LikeStatus{VideoID: videoID, Liked: false, Status: "pending", OperationID: cmd.OperationID, OpVersion: formatVersion(cmd.OpVersion)}, nil
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

func (s *Service) CreateComment(ctx context.Context, userID uint, videoID uint, content string, idempotencyKey string) (*PendingComment, error) {
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

	cmd, state, err := s.buildOrReuseCommand(ctx, CommandCommentCreate, userID, videoID, idempotencyKey, content, CommentStatusKey(uuid.NewString()))
	if err != nil {
		return nil, err
	}
	if state == commandStateReused {
		return pendingCommentFromCommand(cmd), nil
	}
	if err := s.publishCommandAndConfirm(ctx, s.commentRoutingKey, mustMarshalCommand(cmd)); err != nil {
		return nil, err
	}
	return pendingCommentFromCommand(cmd), nil
}

func (s *Service) ListComments(ctx context.Context, videoID uint, limit int) ([]Comment, error) {
	if videoID == 0 {
		return nil, fmt.Errorf("video id is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	if limit <= 20 && s.commentCache != nil {
		key := cache.CommentFirstPageKeyWithLimit(videoID, limit)
		value, err := s.commentCache.Do(ctx, key, func(ctx context.Context) (any, error) {
			if data, err := s.commentCache.GetFirstPageWithLimit(ctx, videoID, limit); err == nil && len(data) > 0 {
				var cached []Comment
				if json.Unmarshal(data, &cached) == nil {
					return cached, nil
				}
			}
			items, err := s.repo.ListComments(ctx, videoID, limit)
			if err != nil {
				return nil, err
			}
			if len(items) > 0 {
				if data, err := json.Marshal(items); err == nil {
					_ = s.commentCache.SetFirstPageWithLimit(ctx, videoID, limit, data)
				}
			}
			return items, nil
		})
		if err != nil {
			return nil, err
		}
		return value.([]Comment), nil
	}

	return s.repo.ListComments(ctx, videoID, limit)
}

func (s *Service) FindComment(ctx context.Context, commentID string) (*Comment, error) {
	if commentID == "" {
		return nil, fmt.Errorf("comment id is required")
	}

	// Try detail cache first.
	if s.commentCache != nil {
		if data, err := s.commentCache.GetDetail(ctx, commentID); err == nil {
			var cached Comment
			if json.Unmarshal(data, &cached) == nil {
				return &cached, nil
			}
		}
	}

	return s.repo.FindComment(ctx, commentID)
}

func (s *Service) DeleteComment(ctx context.Context, commentID string, userID uint, idempotencyKey string) (*PendingCommentDelete, error) {
	if commentID == "" || userID == 0 {
		return nil, fmt.Errorf("comment id and user id are required")
	}

	comment, err := s.FindComment(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotReady
		}
		return nil, err
	}
	if comment.UserID != userID {
		return nil, fmt.Errorf("permission denied")
	}

	cmd, state, err := s.buildOrReuseCommand(ctx, CommandCommentDelete, userID, comment.VideoID, idempotencyKey, "", CommentStatusKey(commentID))
	if err != nil {
		return nil, err
	}
	if state == commandStateReused {
		return &PendingCommentDelete{CommentID: commentID, Status: "pending", OperationID: cmd.OperationID, OpVersion: formatVersion(cmd.OpVersion)}, nil
	}
	if err := s.publishCommandAndConfirm(ctx, s.commentRoutingKey, mustMarshalCommand(cmd)); err != nil {
		return nil, err
	}
	return &PendingCommentDelete{CommentID: commentID, Status: "pending", OperationID: cmd.OperationID, OpVersion: formatVersion(cmd.OpVersion)}, nil
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

type commandReuseState int

const (
	commandStateCreated commandReuseState = iota
	commandStateReused
)

func (s *Service) buildOrReuseCommand(ctx context.Context, commandType CommandType, userID uint, videoID uint, idempotencyKey string, content string, versionKey string) (Command, commandReuseState, error) {
	commentID := ""
	if commandType == CommandCommentCreate {
		commentID = uuid.NewString()
	}
	fingerprint := RequestFingerprint(string(commandType), userID, videoID, content)
	idemKey := cache.IdempotencyKey(userID, string(commandType), idempotencyKey)
	if record, err := s.idempotency.Get(ctx, idemKey); err == nil && record != nil {
		if record.RequestFingerprint != fingerprint {
			return Command{}, 0, cache.ErrIdempotencyConflict
		}
		var stored Command
		if err := json.Unmarshal(record.Command, &stored); err != nil {
			return Command{}, 0, err
		}
		if err := s.publishCommandAndConfirm(ctx, s.routingKeyFor(commandType), []byte(record.Body)); err != nil {
			return Command{}, 0, err
		}
		return stored, commandStateReused, nil
	}

	opVersion, err := s.acquireCommandVersion(ctx, versionKey)
	if err != nil {
		return Command{}, 0, err
	}
	cmd := NewCommand(commandType, userID, videoID, opVersion, commentID, content, idempotencyKey, fingerprint)
	body, err := MarshalCommand(cmd)
	if err != nil {
		return Command{}, 0, err
	}
	if err := s.idempotency.Acquire(ctx, idemKey, fingerprint, s.routingKeyFor(commandType), body, body); err != nil {
		if record, getErr := s.idempotency.Get(ctx, idemKey); getErr == nil && record != nil && record.RequestFingerprint == fingerprint {
			var stored Command
			if unmarshalErr := json.Unmarshal(record.Command, &stored); unmarshalErr == nil {
				if publishErr := s.publishCommandAndConfirm(ctx, s.routingKeyFor(commandType), []byte(record.Body)); publishErr == nil {
					return stored, commandStateReused, nil
				}
			}
		}
		return Command{}, 0, err
	}
	return cmd, commandStateCreated, nil
}

func (s *Service) routingKeyFor(commandType CommandType) string {
	if commandType == CommandLikeCreate || commandType == CommandLikeCancel {
		return s.likeRoutingKey
	}
	return s.commentRoutingKey
}

func (s *Service) acquireCommandVersion(ctx context.Context, businessKey string) (uint64, error) {
	if s.versioner == nil {
		return 0, fmt.Errorf("version sequencer unavailable")
	}
	return s.versioner.Next(ctx, businessKey)
}

func (s *Service) publishCommandAndConfirm(ctx context.Context, routingKey string, body []byte) error {
	if s.commandPublisher == nil {
		return fmt.Errorf("command publisher unavailable")
	}
	return s.commandPublisher.PublishCommandWithConfirm(ctx, routingKey, body, s.confirmTimeout)
}

type PendingComment struct {
	ID          string    `json:"id"`
	VideoID     uint      `json:"videoId"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	OperationID string    `json:"operationId"`
	OpVersion   string    `json:"opVersion"`
	CreatedAt   time.Time `json:"createdAt"`
}

type PendingCommentDelete struct {
	CommentID   string `json:"commentId"`
	Status      string `json:"status"`
	OperationID string `json:"operationId"`
	OpVersion   string `json:"opVersion"`
}

func pendingCommentFromCommand(cmd Command) *PendingComment {
	return &PendingComment{
		ID:          cmd.CommentID,
		VideoID:     cmd.VideoID,
		Content:     cmd.Content,
		Status:      "pending",
		OperationID: cmd.OperationID,
		OpVersion:   formatVersion(cmd.OpVersion),
		CreatedAt:   cmd.OccurredAt,
	}
}

func mustMarshalCommand(cmd Command) []byte {
	body, err := MarshalCommand(cmd)
	if err != nil {
		panic(err)
	}
	return body
}

func formatVersion(v uint64) string {
	return fmt.Sprintf("%d", v)
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
