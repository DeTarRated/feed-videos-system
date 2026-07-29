package httpserver

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/social"
	"feed-videos-system/backend/internal/domain/user"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

type FollowHandler struct {
	social *social.Service
	users  *user.Service
}

func NewFollowHandler(socialService *social.Service, users *user.Service) *FollowHandler {
	return &FollowHandler{social: socialService, users: users}
}

func (h *FollowHandler) Follow(c *gin.Context) {
	followerID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	followingID, ok := h.targetUserID(c)
	if !ok {
		return
	}

	status, err := h.social.FollowUser(c.Request.Context(), followerID, followingID)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	response.OK(c, status)
}

func (h *FollowHandler) Unfollow(c *gin.Context) {
	followerID, ok := CurrentUserID(c)
	if !ok {
		response.Unauthorized(c, "login required")
		return
	}
	followingID, ok := h.targetUserID(c)
	if !ok {
		return
	}

	status, err := h.social.UnfollowUser(c.Request.Context(), followerID, followingID)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	response.OK(c, status)
}

func (h *FollowHandler) Status(c *gin.Context) {
	var followerID uint
	if current, ok := CurrentUserID(c); ok {
		followerID = current
	}
	followingID, ok := h.targetUserID(c)
	if !ok {
		return
	}

	status, err := h.social.FollowStatus(c.Request.Context(), followerID, followingID)
	if err != nil {
		response.BadRequest(c, err)
		return
	}
	response.OK(c, status)
}

func (h *FollowHandler) Followers(c *gin.Context) {
	userID, ok := h.targetUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := h.social.Followers(c.Request.Context(), userID, limit)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *FollowHandler) Following(c *gin.Context) {
	userID, ok := h.targetUserID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := h.social.Following(c.Request.Context(), userID, limit)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *FollowHandler) targetUserID(c *gin.Context) (uint, bool) {
	userID, ok := parseUintParam(c, "id")
	if !ok {
		return 0, false
	}
	if _, err := h.users.Get(c.Request.Context(), userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "user not found")
			return 0, false
		}
		response.InternalError(c)
		return 0, false
	}
	return userID, true
}
