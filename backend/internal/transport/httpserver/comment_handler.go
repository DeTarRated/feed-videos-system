package httpserver

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"feed-videos-system/backend/internal/domain/social"
	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/infrastructure/cache"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

type CommentHandler struct {
	social           *social.Service
	videos           *video.Service
	videoDetailCache *cache.VideoDetailCache
}

func NewCommentHandler(socialService *social.Service, videos *video.Service, videoDetailCache *cache.VideoDetailCache) *CommentHandler {
	return &CommentHandler{social: socialService, videos: videos, videoDetailCache: videoDetailCache}
}

type createCommentRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *CommentHandler) List(c *gin.Context) {
	videoID, ok := parseVideoID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := h.social.ListComments(c.Request.Context(), videoID, limit)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *CommentHandler) Create(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	videoID, ok := parseVideoID(c)
	if !ok {
		return
	}

	var req createCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}
	comment, err := h.social.CreateComment(c.Request.Context(), userID, videoID, req.Content, c.GetHeader("Idempotency-Key"))
	if err != nil {
		switch {
		case errors.Is(err, cache.ErrIdempotencyConflict):
			response.Conflict(c, "idempotency key conflicts with previous request")
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), isPublishUnavailableError(err):
			response.ServiceUnavailable(c, "interaction command is temporarily unavailable")
		default:
			response.BadRequest(c, err)
		}
		return
	}
	// Side effects (cache invalidation, hot score, comment count) are handled
	// asynchronously by the Worker consuming the CommentCreated event.
	response.Accepted(c, comment)
}

func (h *CommentHandler) Delete(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	commentID := c.Param("commentId")
	if commentID == "" {
		response.BadRequest(c, errors.New("invalid comment id"))
		return
	}

	result, err := h.social.DeleteComment(c.Request.Context(), commentID, userID, c.GetHeader("Idempotency-Key"))
	if err != nil {
		switch {
		case errors.Is(err, social.ErrCommentNotReady):
			response.Conflict(c, "comment_not_ready")
		case errors.Is(err, cache.ErrIdempotencyConflict):
			response.Conflict(c, "idempotency key conflicts with previous request")
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), isPublishUnavailableError(err):
			response.ServiceUnavailable(c, "interaction command is temporarily unavailable")
		default:
			response.BadRequest(c, err)
		}
		return
	}
	// Side effects (cache invalidation, hot score, comment count) are handled
	// asynchronously by the Worker consuming the CommentDeleted event.
	response.Accepted(c, result)
}
