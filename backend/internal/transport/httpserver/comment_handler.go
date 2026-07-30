package httpserver

import (
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
	comment, err := h.social.CreateComment(c.Request.Context(), userID, videoID, req.Content)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	// Side effects (cache invalidation, hot score, comment count) are handled
	// asynchronously by the Worker consuming the CommentCreated event.
	response.Created(c, comment)
}

func (h *CommentHandler) Delete(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	commentID, err := strconv.ParseUint(c.Param("commentId"), 10, 64)
	if err != nil || commentID == 0 {
		response.BadRequest(c, errors.New("invalid comment id"))
		return
	}

	if err := h.social.DeleteComment(c.Request.Context(), uint(commentID), userID); err != nil {
		response.BadRequest(c, err)
		return
	}
	// Side effects (cache invalidation, hot score, comment count) are handled
	// asynchronously by the Worker consuming the CommentDeleted event.
	response.OK(c, gin.H{"deleted": true})
}
