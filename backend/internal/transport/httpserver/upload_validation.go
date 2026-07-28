package httpserver

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateUploadFile(kind, filename string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	switch kind {
	case "videos":
		switch ext {
		case ".mp4", ".mov", ".webm", ".mkv", ".m4v":
			return nil
		default:
			return fmt.Errorf("unsupported video file type")
		}
	case "covers":
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif":
			return nil
		default:
			return fmt.Errorf("unsupported cover file type")
		}
	default:
		return fmt.Errorf("unsupported upload type")
	}
}
