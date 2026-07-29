package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"feed-videos-system/backend/internal/app"
)

func NewRouter(application *app.App) http.Handler {
	engine := gin.New()
	engine.Use(CORS())
	engine.Use(gin.Recovery())
	engine.Use(RequestLogger())
	engine.NoRoute(NoRoute)

	engine.GET("/healthz", healthCheck)
	engine.Static("/media", application.Config.StorageDir)

	authHandler := NewAuthHandler(application.Users, application.Auth)
	videoHandler := NewVideoHandler(application.Videos, application.Social)
	userHandler := NewUserHandler(application.Users, application.Videos, application.Social)
	likeHandler := NewLikeHandler(application.Social)
	commentHandler := NewCommentHandler(application.Social)
	followHandler := NewFollowHandler(application.Social, application.Users)
	uploadHandler := NewUploadHandler(application.Storage)

	api := engine.Group("/api/v1")
	{
		api.GET("/healthz", healthCheck)
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)

		api.GET("/videos", videoHandler.Latest)
		api.GET("/videos/:id", videoHandler.Detail)
		api.GET("/videos/:id/like-status", OptionalAuth(application.Auth), likeHandler.Status)
		api.POST("/videos/:id/like", AuthRequired(application.Auth), likeHandler.Like)
		api.DELETE("/videos/:id/like", AuthRequired(application.Auth), likeHandler.Unlike)
		api.GET("/videos/:id/comments", commentHandler.List)
		api.POST("/videos/:id/comments", AuthRequired(application.Auth), commentHandler.Create)
		api.DELETE("/videos/:id/comments/:commentId", AuthRequired(application.Auth), commentHandler.Delete)
		api.POST("/videos", AuthRequired(application.Auth), videoHandler.Publish)

		api.GET("/users/:id/stats", userHandler.Stats)
		api.GET("/users/:id/videos", userHandler.Videos)
		api.GET("/users/:id/follow-status", OptionalAuth(application.Auth), followHandler.Status)
		api.POST("/users/:id/follow", AuthRequired(application.Auth), followHandler.Follow)
		api.DELETE("/users/:id/follow", AuthRequired(application.Auth), followHandler.Unfollow)
		api.GET("/users/:id/followers", followHandler.Followers)
		api.GET("/users/:id/following", followHandler.Following)
		api.GET("/users/me/liked-videos", AuthRequired(application.Auth), likeHandler.LikedVideos)

		uploads := api.Group("/uploads", AuthRequired(application.Auth))
		{
			uploads.POST("/videos", uploadHandler.Video)
			uploads.POST("/covers", uploadHandler.Cover)
		}
	}

	return engine
}
