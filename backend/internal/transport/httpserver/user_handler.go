package httpserver

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/user"
	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

type UserHandler struct {
	users  *user.Service
	videos *video.Service
}

func NewUserHandler(users *user.Service, videos *video.Service) *UserHandler {
	return &UserHandler{users: users, videos: videos}
}

type userStatsResponse struct {
	UserID         uint  `json:"userId"`
	WorksCount    int64 `json:"worksCount"`
	LikeCount     int64 `json:"likeCount"`
	FollowerCount int64 `json:"followerCount"`
	FollowingCount int64 `json:"followingCount"`
}

func (h *UserHandler) Stats(c *gin.Context) {
	userID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	if _, err := h.users.Get(c.Request.Context(), userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		response.InternalError(c)
		return
	}

	worksCount, err := h.videos.CountByAuthor(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c)
		return
	}

	response.OK(c, userStatsResponse{
		UserID:         userID,
		WorksCount:    worksCount,
		LikeCount:     0,
		FollowerCount: 0,
		FollowingCount: 0,
	})
}

func (h *UserHandler) Videos(c *gin.Context) {
	userID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if _, err := h.users.Get(c.Request.Context(), userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		response.InternalError(c)
		return
	}

	items, err := h.videos.ByAuthor(c.Request.Context(), userID, limit)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, gin.H{"items": items})
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, errors.New("invalid id"))
		return 0, false
	}
	return uint(id), true
}
