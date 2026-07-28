package httpserver

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		fmt.Printf("method=%s path=%s status=%d duration=%s\n", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(started))
	}
}
