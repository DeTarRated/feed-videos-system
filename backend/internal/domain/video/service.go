package video

import (
	"context"
	"encoding/json"
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
	GetPage(ctx context.Context, snapshotID string, offset int, limit int) (*HotFeedSnapshotPage, error)
}

type LatestPage struct {
	Items      []Video `json:"items"`
	NextCursor string  `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

type HotPage struct {
	Items       []Video `json:"items"`
	SnapshotID  string  `json:"snapshotId"`
	NextOffset  int     `json:"nextOffset"`
	LastVideoID uint    `json:"lastVideoId"`
	HasMore     bool    `json:"hasMore"`
}

type DetailCache interface {
	GetMany(ctx context.Context, videoIDs []uint) (map[uint][]byte, []uint, error)
	Set(ctx context.Context, videoID uint, value any) error
}

type Service struct {
	repo     Repository
	rank     RankStore
	snapshot SnapshotStore
	details  DetailCache
}

func NewService(repo Repository, rank RankStore, snapshot SnapshotStore, details DetailCache) *Service {
	return &Service{repo: repo, rank: rank, snapshot: snapshot, details: details}
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

func (s *Service) HotPage(ctx context.Context, snapshotID string, offset int, limit int) (*HotPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	if s.rank == nil {
		items, err := s.Latest(ctx, limit)
		if err != nil {
			return nil, err
		}
		return &HotPage{Items: items, HasMore: false}, nil
	}

	if s.snapshot != nil {
		if snapshotID == "" {
			topIDs, err := s.rank.TopIDs(ctx, 200)
			if err != nil {
				return nil, err
			}
			snapshot, err := s.snapshot.Create(ctx, topIDs)
			if err != nil {
				return nil, err
			}
			return s.hotPageFromSnapshot(ctx, snapshot.ID, 0, limit)
		}
		page, err := s.snapshot.GetPage(ctx, snapshotID, offset, limit)
		if err != nil {
			return nil, err
		}
		if page == nil {
			topIDs, err := s.rank.TopIDs(ctx, 200)
			if err != nil {
				return nil, err
			}
			snapshot, err := s.snapshot.Create(ctx, topIDs)
			if err != nil {
				return nil, err
			}
			return s.hotPageFromSnapshot(ctx, snapshot.ID, 0, limit)
		}
		return s.hotPageFromIDSlice(ctx, page.IDs, snapshotID, page.Offset, page.HasMore)
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
	start := offset
	if start >= len(topIDs) {
		return &HotPage{Items: []Video{}, HasMore: false}, nil
	}
	end := start + limit
	if end > len(topIDs) {
		end = len(topIDs)
	}
	return s.hotPageFromIDSlice(ctx, topIDs[start:end], "", end, end < len(topIDs))
}

func (s *Service) hotPageFromSnapshot(ctx context.Context, snapshotID string, offset int, limit int) (*HotPage, error) {
	page, err := s.snapshot.GetPage(ctx, snapshotID, offset, limit)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return &HotPage{Items: []Video{}, SnapshotID: snapshotID, HasMore: false}, nil
	}
	return s.hotPageFromIDSlice(ctx, page.IDs, snapshotID, page.Offset, page.HasMore)
}

func (s *Service) hotPageFromIDSlice(ctx context.Context, ids []uint, snapshotID string, nextOffset int, hasMore bool) (*HotPage, error) {
	if len(ids) == 0 {
		return &HotPage{Items: []Video{}, SnapshotID: snapshotID, NextOffset: nextOffset, HasMore: hasMore}, nil
	}
	items, err := s.loadHotPageItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	var nextLastID uint
	if len(items) > 0 {
		nextLastID = items[len(items)-1].ID
	}
	return &HotPage{Items: items, SnapshotID: snapshotID, NextOffset: nextOffset, LastVideoID: nextLastID, HasMore: hasMore}, nil
}

func (s *Service) loadHotPageItems(ctx context.Context, ids []uint) ([]Video, error) {
	if s.details == nil {
		return s.repo.ListByIDs(ctx, ids)
	}
	hits, missing, err := s.details.GetMany(ctx, ids)
	if err != nil {
		return s.repo.ListByIDs(ctx, ids)
	}
	items := make([]Video, 0, len(ids))
	byID := make(map[uint]Video, len(ids))
	for id, raw := range hits {
		var item Video
		if err := json.Unmarshal(raw, &item); err == nil {
			byID[id] = item
		}
	}
	if len(missing) > 0 {
		loaded, err := s.repo.ListByIDs(ctx, missing)
		if err != nil {
			return nil, err
		}
		for _, item := range loaded {
			_ = s.details.Set(ctx, item.ID, item)
			byID[item.ID] = item
		}
	}
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			items = append(items, item)
		}
	}
	return items, nil
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
