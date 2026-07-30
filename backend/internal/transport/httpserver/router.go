package httpserver

import (
	"net/http"
	"time"

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
	videoHandler := NewVideoHandler(application.Videos, application.Social, application.VideoDetailCache)
	userHandler := NewUserHandler(application.Users, application.Videos, application.Social)
	likeHandler := NewLikeHandler(application.Social, application.Videos, application.VideoDetailCache)
	commentHandler := NewCommentHandler(application.Social, application.Videos, application.VideoDetailCache)
	followHandler := NewFollowHandler(application.Social, application.Users)
	uploadHandler := NewUploadHandler(application.Storage)

	registerLimit := RateLimit(application.Redis, RateLimitRule{Name: "auth_register", Limit: 5, Window: time.Minute})
	loginLimit := RateLimit(application.Redis, RateLimitRule{Name: "auth_login", Limit: 10, Window: time.Minute})
	publishLimit := RateLimit(application.Redis, RateLimitRule{Name: "video_publish", Limit: 5, Window: time.Minute, UseUserID: true, FallbackToIP: true})
	likeLimit := RateLimit(application.Redis, RateLimitRule{Name: "video_like", Limit: 60, Window: time.Minute, UseUserID: true, FallbackToIP: true})
	commentLimit := RateLimit(application.Redis, RateLimitRule{Name: "video_comment", Limit: 20, Window: time.Minute, UseUserID: true, FallbackToIP: true})
	followLimit := RateLimit(application.Redis, RateLimitRule{Name: "user_follow", Limit: 30, Window: time.Minute, UseUserID: true, FallbackToIP: true})

	api := engine.Group("/api/v1")
	{
		api.GET("/healthz", healthCheck)
		api.POST("/auth/register", registerLimit, authHandler.Register)
		api.POST("/auth/login", loginLimit, authHandler.Login)

		api.GET("/videos", videoHandler.Latest)
		api.GET("/feed/hot", videoHandler.Hot)
		api.GET("/videos/:id", videoHandler.Detail)
		api.GET("/videos/:id/like-status", OptionalAuth(application.Auth), likeHandler.Status)
		api.POST("/videos/:id/like", AuthRequired(application.Auth), likeLimit, likeHandler.Like)
		api.DELETE("/videos/:id/like", AuthRequired(application.Auth), likeLimit, likeHandler.Unlike)
		api.GET("/videos/:id/comments", commentHandler.List)
		api.POST("/videos/:id/comments", AuthRequired(application.Auth), commentLimit, commentHandler.Create)
		api.DELETE("/videos/:id/comments/:commentId", AuthRequired(application.Auth), commentLimit, commentHandler.Delete)
		api.POST("/videos", AuthRequired(application.Auth), publishLimit, videoHandler.Publish)

		api.GET("/users/me", AuthRequired(application.Auth), userHandler.Me)
		api.PATCH("/users/me", AuthRequired(application.Auth), userHandler.UpdateProfile)
		api.PATCH("/users/me/password", AuthRequired(application.Auth), userHandler.ChangePassword)
		api.GET("/users/:id", userHandler.Detail)
		api.GET("/users/:id/stats", userHandler.Stats)
		api.GET("/users/:id/videos", userHandler.Videos)
		api.GET("/users/:id/follow-status", OptionalAuth(application.Auth), followHandler.Status)
		api.POST("/users/:id/follow", AuthRequired(application.Auth), followLimit, followHandler.Follow)
		api.DELETE("/users/:id/follow", AuthRequired(application.Auth), followLimit, followHandler.Unfollow)
		api.GET("/users/:id/followers", followHandler.Followers)
		api.GET("/users/:id/following", followHandler.Following)
		api.GET("/me/liked-videos", AuthRequired(application.Auth), likeHandler.LikedVideos)

		uploads := api.Group("/uploads", AuthRequired(application.Auth))
		{
			uploads.POST("/videos", uploadHandler.Video)
			uploads.POST("/covers", uploadHandler.Cover)
		}
	}

	return engine
}
