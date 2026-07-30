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

type HotFeedSnapshot struct {
	ID      string    `json:"id"`
	IDs     []uint    `json:"ids"`
	Created time.Time `json:"createdAt"`
}

type SnapshotStore interface {
	Create(ctx context.Context, ids []uint) (*HotFeedSnapshot, error)
	Get(ctx context.Context, snapshotID string) (*HotFeedSnapshot, error)
}

type LatestPage struct {
	Items      []Video `json:"items"`
	NextCursor string  `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

type HotPage struct {
	Items       []Video `json:"items"`
	SnapshotID  string  `json:"snapshotId"`
	LastVideoID uint    `json:"lastVideoId"`
	HasMore     bool    `json:"hasMore"`
}

type Service struct {
	repo     Repository
	rank     RankStore
	snapshot SnapshotStore
}

func NewService(repo Repository, rank RankStore, snapshot SnapshotStore) *Service {
	return &Service{repo: repo, rank: rank, snapshot: snapshot}
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
	page, err := s.HotPage(ctx, "", 0, limit)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Service) HotPage(ctx context.Context, snapshotID string, lastVideoID uint, limit int) (*HotPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if s.rank == nil {
		items, err := s.Latest(ctx, limit)
		if err != nil {
			return nil, err
		}
		return &HotPage{Items: items, HasMore: false}, nil
	}

	topIDs, err := s.rank.TopIDs(ctx, 200)
	if err != nil {
		return nil, err
	}
	if len(topIDs) == 0 {
		items, err := s.Latest(ctx, limit)
		if err != nil {
			return nil, err
		}
		return &HotPage{Items: items, HasMore: false}, nil
	}

	var snapshot *HotFeedSnapshot
	if s.snapshot != nil {
		if snapshotID == "" {
			snapshot, err = s.snapshot.Create(ctx, topIDs)
			if err != nil {
				return nil, err
			}
		} else {
			snapshot, err = s.snapshot.Get(ctx, snapshotID)
			if err != nil {
				return nil, err
			}
			if snapshot == nil {
				snapshot, err = s.snapshot.Create(ctx, topIDs)
				if err != nil {
					return nil, err
				}
				lastVideoID = 0
			}
		}
	} else {
		snapshot = &HotFeedSnapshot{ID: "", IDs: topIDs, Created: time.Now().UTC()}
	}

	start := 0
	if lastVideoID > 0 {
		for i, id := range snapshot.IDs {
			if id == lastVideoID {
				start = i + 1
				break
			}
		}
	}
	if start > len(snapshot.IDs) {
		start = len(snapshot.IDs)
	}
	end := start + limit
	if end > len(snapshot.IDs) {
		end = len(snapshot.IDs)
	}
	if start >= len(snapshot.IDs) {
		return &HotPage{Items: []Video{}, SnapshotID: snapshot.ID, LastVideoID: 0, HasMore: false}, nil
	}

	ids := snapshot.IDs[start:end]
	items, err := s.repo.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	hasMore := end < len(snapshot.IDs)
	var nextLastID uint
	if len(items) > 0 {
		nextLastID = items[len(items)-1].ID
	}
	return &HotPage{Items: items, SnapshotID: snapshot.ID, LastVideoID: nextLastID, HasMore: hasMore}, nil
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
