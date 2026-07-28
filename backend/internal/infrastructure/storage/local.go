package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LocalStore struct {
	BaseDir string
}

type SavedFile struct {
	Path string
	URL  string
	Name string
	Size int64
}

func NewLocalStore(baseDir string) *LocalStore {
	return &LocalStore{BaseDir: baseDir}
}

func (s *LocalStore) EnsureDir() error {
	if s.BaseDir == "" {
		return fmt.Errorf("base dir is empty")
	}
	return os.MkdirAll(s.BaseDir, 0o755)
}

func (s *LocalStore) Resolve(parts ...string) string {
	items := append([]string{s.BaseDir}, parts...)
	return filepath.Clean(filepath.Join(items...))
}

func (s *LocalStore) Save(kind string, file *multipart.FileHeader) (*SavedFile, error) {
	if err := validateKind(kind); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, fmt.Errorf("file is required")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	relDir := filepath.Join(kind, time.Now().Format("20060102"))
	absDir := s.Resolve(relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, err
	}

	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	absPath := filepath.Join(absDir, name)
	dst, err := os.Create(absPath)
	if err != nil {
		return nil, err
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		return nil, err
	}

	urlPath := "/media/" + strings.ReplaceAll(filepath.ToSlash(filepath.Join(relDir, name)), " ", "%20")
	return &SavedFile{
		Path: absPath,
		URL:  urlPath,
		Name: name,
		Size: written,
	}, nil
}

func validateKind(kind string) error {
	switch kind {
	case "videos", "covers", "avatars":
		return nil
	default:
		return fmt.Errorf("unsupported storage kind")
	}
}
