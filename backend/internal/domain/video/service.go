package video

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type RankStore interface {
	SetScore(ctx context.Context, videoID uint, score float64) error
	TopIDs(ctx context.Context, limit int) ([]uint, error)
}

type Service struct {
	repo Repository
	rank RankStore
}

func NewService(repo Repository, rank RankStore) *Service {
	return &Service{repo: repo, rank: rank}
}

type LatestPage struct {
	Items      []Video `json:"items"`
	NextCursor string  `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
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

func (s *Service) LatestPage(ctx context.Context, cursorValue string, limit int) (*LatestPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	cursor, err := DecodeCursor(cursorValue)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListLatestByCursor(ctx, cursor, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor, err = EncodeCursor(Cursor{PublishedAt: last.PublishedAt, ID: last.ID})
		if err != nil {
			return nil, err
		}
	}

	return &LatestPage{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

func (s *Service) Hot(ctx context.Context, limit int) ([]Video, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if s.rank == nil {
		return s.Latest(ctx, limit)
	}
	ids, err := s.rank.TopIDs(ctx, limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return s.Latest(ctx, limit)
	}
	return s.repo.ListByIDs(ctx, ids)
}

func (s *Service) RefreshHotScore(ctx context.Context, videoID uint, likeCount int64, commentCount int64) error {
	if videoID == 0 {
		return fmt.Errorf("video id is required")
	}
	if s.rank == nil {
		return nil
	}
	item, err := s.repo.FindByID(ctx, videoID)
	if err != nil {
		return err
	}
	return s.rank.SetScore(ctx, videoID, hotScore(*item, likeCount, commentCount))
}

func hotScore(item Video, likeCount int64, commentCount int64) float64 {
	publishScore := math.Log(float64(item.PublishedAt.Unix()) + 1)
	return float64(likeCount*5+commentCount*3) + publishScore
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
