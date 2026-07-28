package video

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type PublishInput struct {
	AuthorID    uint
	Title       string
	Description string
	VideoURL    string
	CoverURL    string
}

func (s *Service) Publish(ctx context.Context, input PublishInput) (*Video, error) {
	title := strings.TrimSpace(input.Title)
	videoURL := strings.TrimSpace(input.VideoURL)
	if input.AuthorID == 0 {
		return nil, fmt.Errorf("author id is required")
	}
	if title == "" || videoURL == "" {
		return nil, fmt.Errorf("title and video url are required")
	}

	v := &Video{
		AuthorID:    input.AuthorID,
		Title:       title,
		Description: strings.TrimSpace(input.Description),
		VideoURL:    videoURL,
		CoverURL:    strings.TrimSpace(input.CoverURL),
		PublishedAt: time.Now(),
	}
	if err := s.repo.Create(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) Detail(ctx context.Context, id uint) (*Video, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) Latest(ctx context.Context, limit int) ([]Video, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.repo.ListLatest(ctx, limit)
}

func (s *Service) ByAuthor(ctx context.Context, authorID uint, limit int) ([]Video, error) {
	if authorID == 0 {
		return nil, fmt.Errorf("author id is required")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.repo.ListByAuthor(ctx, authorID, limit)
}

func (s *Service) CountByAuthor(ctx context.Context, authorID uint) (int64, error) {
	if authorID == 0 {
		return 0, fmt.Errorf("author id is required")
	}
	return s.repo.CountByAuthor(ctx, authorID)
}
