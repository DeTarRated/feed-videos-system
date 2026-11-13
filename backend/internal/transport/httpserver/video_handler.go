package httpserver

import (
	"context"
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

type VideoHandler struct {
	videos           *video.Service
	social           *social.Service
	videoDetailCache *cache.VideoDetailCache
}

func NewVideoHandler(videos *video.Service, socialService *social.Service, videoDetailCache *cache.VideoDetailCache) *VideoHandler {
	return &VideoHandler{videos: videos, social: socialService, videoDetailCache: videoDetailCache}
}

type publishVideoRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	VideoURL    string `json:"videoUrl" binding:"required"`
	CoverURL    string `json:"coverUrl"`
}

type videoDetailResponse struct {
	*video.Video
	CommentCount int64 `json:"commentCount"`
}

type hotPageResponse struct {
	Items       []video.Video `json:"items"`
	SnapshotID  string        `json:"snapshotId"`
	NextOffset  int           `json:"nextOffset"`
	LastVideoID uint          `json:"lastVideoId"`
	HasMore     bool          `json:"hasMore"`
}

func (h *VideoHandler) Publish(c *gin.Context) {
	var req publishVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}

	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}

	item, err := h.videos.Publish(c.Request.Context(), video.PublishInput{
		AuthorID:    userID,
		Title:       req.Title,
		Description: req.Description,
		VideoURL:    req.VideoURL,
		CoverURL:    req.CoverURL,
	})
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	if err := h.videos.RefreshHotScore(c.Request.Context(), item.ID, 0, 0); err != nil {
		log.Printf("refresh hot score: %v", err)
	}
	response.Created(c, item)
}

func (h *VideoHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, errors.New("invalid video id"))
		return
	}

	videoID := uint(id)
	if empty, err := h.videoDetailCache.IsEmpty(c.Request.Context(), videoID); err == nil && empty {
		response.NotFound(c, "video not found")
		return
	}
	cached := videoDetailResponse{Video: &video.Video{}}
	if hit, err := h.videoDetailCache.Get(c.Request.Context(), videoID, &cached); err == nil && hit {
		response.OK(c, cached)
		return
	} else if err != nil {
		log.Printf("read video detail cache: %v", err)
	}

	value, err := h.videoDetailCache.Do(c.Request.Context(), cache.VideoDetailKey(videoID), func(ctx context.Context) (any, error) {
		item, err := h.videos.Detail(ctx, videoID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_ = h.videoDetailCache.SetEmpty(ctx, videoID)
			}
			return nil, err
		}
		commentCount, err := h.social.CountVideoComments(ctx, videoID)
		if err != nil {
			return nil, err
		}
		detail := videoDetailResponse{Video: item, CommentCount: commentCount}
		if err := h.videoDetailCache.Set(ctx, videoID, detail); err != nil {
			log.Printf("write video detail cache: %v", err)
		}
		return detail, nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "video not found")
			return
		}
		response.InternalError(c)
		return
	}
	response.OK(c, value)
}

func (h *VideoHandler) Latest(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, err := h.videos.LatestPage(c.Request.Context(), c.Query("cursor"), limit)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	response.OK(c, page)
}

func (h *VideoHandler) Hot(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	page, err := h.videos.HotPage(c.Request.Context(), c.Query("snapshotId"), offset, limit)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, hotPageResponse{Items: page.Items, SnapshotID: page.SnapshotID, NextOffset: page.NextOffset, LastVideoID: page.LastVideoID, HasMore: page.HasMore})
}
