package httpserver

import (
	"errors"
	"log"
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

	status, err := h.social.LikeVideo(c.Request.Context(), userID, videoID)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	h.deleteVideoDetailCache(c, videoID)
	h.refreshHotScore(c, videoID, status.LikeCount)
	response.OK(c, status)
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

	status, err := h.social.UnlikeVideo(c.Request.Context(), userID, videoID)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	h.deleteVideoDetailCache(c, videoID)
	h.refreshHotScore(c, videoID, status.LikeCount)
	response.OK(c, status)
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

func (h *LikeHandler) deleteVideoDetailCache(c *gin.Context, videoID uint) {
	if err := h.videoDetailCache.Delete(c.Request.Context(), videoID); err != nil {
		log.Printf("delete video detail cache: %v", err)
	}
}

func (h *LikeHandler) refreshHotScore(c *gin.Context, videoID uint, likeCount int64) {
	commentCount, err := h.social.CountVideoComments(c.Request.Context(), videoID)
	if err != nil {
		log.Printf("count video comments for hot score: %v", err)
		return
	}
	if err := h.videos.RefreshHotScore(c.Request.Context(), videoID, likeCount, commentCount); err != nil {
		log.Printf("refresh hot score: %v", err)
	}
}
