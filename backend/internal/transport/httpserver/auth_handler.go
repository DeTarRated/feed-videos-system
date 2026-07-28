package httpserver

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/auth"
	"feed-videos-system/backend/internal/domain/user"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

type AuthHandler struct {
	users *user.Service
	auth  *auth.Service
}

func NewAuthHandler(users *user.Service, authService *auth.Service) *AuthHandler {
	return &AuthHandler{users: users, auth: authService}
}

type registerRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type authResponse struct {
	Token string     `json:"token"`
	User  *user.User `json:"user"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}

	u, err := h.users.Register(c.Request.Context(), user.RegisterInput{
		Username: req.Username,
		Password: req.Password,
		Nickname: req.Nickname,
	})
	if err != nil {
		response.BadRequest(c, err)
		return
	}

	token, err := h.auth.Generate(u.ID)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.Created(c, authResponse{Token: token, User: u})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}

	u, err := h.users.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) || errors.Is(err, gorm.ErrRecordNotFound) {
			response.Unauthorized(c, "invalid username or password")
			return
		}
		response.InternalError(c)
		return
	}

	token, err := h.auth.Generate(u.ID)
	if err != nil {
		response.InternalError(c)
		return
	}
	response.OK(c, authResponse{Token: token, User: u})
}
