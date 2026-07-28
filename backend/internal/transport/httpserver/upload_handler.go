package httpserver

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"feed-videos-system/backend/internal/infrastructure/storage"
	"feed-videos-system/backend/internal/transport/httpserver/response"
)

const (
	maxVideoSize = 200 << 20
	maxCoverSize = 10 << 20
)

type UploadHandler struct {
	store *storage.LocalStore
}

func NewUploadHandler(store *storage.LocalStore) *UploadHandler {
	return &UploadHandler{store: store}
}

type uploadResponse struct {
	URL  string `json:"url"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (h *UploadHandler) Video(c *gin.Context) {
	h.upload(c, "videos", maxVideoSize)
}

func (h *UploadHandler) Cover(c *gin.Context) {
	h.upload(c, "covers", maxCoverSize)
}

func (h *UploadHandler) upload(c *gin.Context, kind string, maxSize int64) {
	if err := c.Request.ParseMultipartForm(maxSize); err != nil {
		response.BadRequest(c, fmt.Errorf("invalid multipart form"))
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, fmt.Errorf("file field is required"))
		return
	}
	if file.Size > maxSize {
		response.BadRequest(c, fmt.Errorf("file is too large"))
		return
	}

	if err := validateUploadFile(kind, file.Filename); err != nil {
		response.BadRequest(c, err)
		return
	}

	saved, err := h.store.Save(kind, file)
	if err != nil {
		response.BadRequest(c, err)
		return
	}

	c.JSON(http.StatusCreated, response.Body{Data: uploadResponse{
		URL:  saved.URL,
		Name: saved.Name,
		Size: saved.Size,
	}})
}
