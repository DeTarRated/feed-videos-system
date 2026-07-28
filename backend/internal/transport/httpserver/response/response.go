package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Body struct {
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Data: data})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Data: data})
}

func BadRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, Body{Error: err.Error()})
}

func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Body{Error: message})
}

func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Body{Error: message})
}

func InternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, Body{Error: "internal server error"})
}
