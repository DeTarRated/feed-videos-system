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

type LikeHandler struct {
	social           *social.Service
	videos           *video.Service
	videoDetailCache *cache.VideoDetailCache
}

func NewLikeHandler(socialService *social.Service, videos *video.Service, videoDetailCache *cache.VideoDetailCache) *LikeHandler {
	return &LikeHandler{social: socialService, videos: videos, videoDetailCache: videoDetailCache}
}

func (h *LikeHandler) Like(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	videoID, ok := parseVideoID(c)
	if !ok {
		return
	}

	status, err := h.social.LikeVideo(c.Request.Context(), userID, videoID, c.GetHeader("Idempotency-Key"))
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
	response.Accepted(c, status)
}

func (h *LikeHandler) Unlike(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	videoID, ok := parseVideoID(c)
	if !ok {
		return
	}

	status, err := h.social.UnlikeVideo(c.Request.Context(), userID, videoID, c.GetHeader("Idempotency-Key"))
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
	response.Accepted(c, status)
}

func (h *LikeHandler) Status(c *gin.Context) {
	var userID uint
	if current, ok := CurrentUserID(c); ok {
		userID = current
	}
	videoID, ok := parseVideoID(c)
	if !ok {
		return
	}

	status, err := h.social.Status(c.Request.Context(), userID, videoID)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	response.OK(c, status)
}

func (h *LikeHandler) LikedVideos(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := h.social.LikedVideos(c.Request.Context(), userID, limit)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, gin.H{"items": items})
}

func parseVideoID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, errors.New("invalid video id"))
		return 0, false
	}
	return uint(id), true
}
