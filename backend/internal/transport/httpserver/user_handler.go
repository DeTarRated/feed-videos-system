package httpserver

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/social"
	"feed-videos-system/backend/internal/domain/user"
	"feed-videos-system/backend/internal/domain/video"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

type UserHandler struct {
	users  *user.Service
	videos *video.Service
	social *social.Service
}

func NewUserHandler(users *user.Service, videos *video.Service, socialService *social.Service) *UserHandler {
	return &UserHandler{users: users, videos: videos, social: socialService}
}

type userStatsResponse struct {
	UserID         uint  `json:"userId"`
	WorksCount     int64 `json:"worksCount"`
	LikeCount      int64 `json:"likeCount"`
	FollowerCount  int64 `json:"followerCount"`
	FollowingCount int64 `json:"followingCount"`
}

type updateProfileRequest struct {
	Nickname  string `json:"nickname" binding:"required"`
	AvatarURL string `json:"avatarUrl"`
	Bio       string `json:"bio"`
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

func (h *UserHandler) Me(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	h.getUser(c, userID)
}

func (h *UserHandler) Detail(c *gin.Context) {
	userID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	h.getUser(c, userID)
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}
	u, err := h.users.UpdateProfile(c.Request.Context(), userID, user.UpdateProfileInput{
		Nickname:  req.Nickname,
		AvatarURL: req.AvatarURL,
		Bio:       req.Bio,
	})
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	response.OK(c, u)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}
	if err := h.users.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) {
			response.Unauthorized(c, "invalid old password")
			return
		}
		response.BadRequest(c, err)
		return
	}
	response.OK(c, gin.H{"updated": true})
}

func (h *UserHandler) getUser(c *gin.Context, userID uint) {
	u, err := h.users.Get(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "user not found")
			return
		}
		response.InternalError(c)
		return
	}
	response.OK(c, u)
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

	likeCount, err := h.social.CountAuthorReceivedLikes(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c)
		return
	}
	followerCount, err := h.social.CountFollowers(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c)
		return
	}
	followingCount, err := h.social.CountFollowing(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c)
		return
	}

	response.OK(c, userStatsResponse{
		UserID:         userID,
		WorksCount:     worksCount,
		LikeCount:      likeCount,
		FollowerCount:  followerCount,
		FollowingCount: followingCount,
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
