package httpserver

import (
	"errors"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
	h.deleteVideoDetailCache(c, videoID)
	h.refreshHotScore(c, videoID)
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

	comment, err := h.social.FindComment(c.Request.Context(), uint(commentID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "comment not found")
			return
		}
		response.InternalError(c)
		return
	}

	if err := h.social.DeleteComment(c.Request.Context(), uint(commentID), userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "comment not found")
			return
		}
		response.BadRequest(c, err)
		return
	}
	h.deleteVideoDetailCache(c, comment.VideoID)
	h.refreshHotScore(c, comment.VideoID)
	response.OK(c, gin.H{"deleted": true})
}

func (h *CommentHandler) deleteVideoDetailCache(c *gin.Context, videoID uint) {
	if err := h.videoDetailCache.Delete(c.Request.Context(), videoID); err != nil {
		log.Printf("delete video detail cache: %v", err)
	}
}

func (h *CommentHandler) refreshHotScore(c *gin.Context, videoID uint) {
	likeCount, err := h.social.Status(c.Request.Context(), 0, videoID)
	if err != nil {
		log.Printf("count video likes for hot score: %v", err)
		return
	}
	commentCount, err := h.social.CountVideoComments(c.Request.Context(), videoID)
	if err != nil {
		log.Printf("count video comments for hot score: %v", err)
		return
	}
	if err := h.videos.RefreshHotScore(c.Request.Context(), videoID, likeCount.LikeCount, commentCount); err != nil {
		log.Printf("refresh hot score: %v", err)
	}
}
