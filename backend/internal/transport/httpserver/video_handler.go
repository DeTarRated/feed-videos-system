package httpserver

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

type VideoHandler struct {
	videos *video.Service
}

func NewVideoHandler(videos *video.Service) *VideoHandler {
	return &VideoHandler{videos: videos}
}

type publishVideoRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	VideoURL    string `json:"videoUrl" binding:"required"`
	CoverURL    string `json:"coverUrl"`
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
	response.Created(c, item)
}

func (h *VideoHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, errors.New("invalid video id"))
		return
	}

	item, err := h.videos.Detail(c.Request.Context(), uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "video not found")
			return
		}
		response.InternalError(c)
		return
	}
	response.OK(c, item)
}

func (h *VideoHandler) Latest(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := h.videos.Latest(c.Request.Context(), limit)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, gin.H{"items": items})
}
