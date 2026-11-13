package video

import "time"

type Video struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AuthorID    uint      `gorm:"index;not null" json:"authorId"`
	Title       string    `gorm:"size:128;not null" json:"title"`
	Description string    `gorm:"size:500;not null;default:''" json:"description"`
	VideoURL    string    `gorm:"size:255;not null" json:"videoUrl"`
	CoverURL    string    `gorm:"size:255;not null;default:''" json:"coverUrl"`
	PublishedAt time.Time `json:"publishedAt"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Video) TableName() string {
	return "videos"
}

type HotFeedSnapshotPage struct {
	SnapshotID string `json:"snapshotId"`
	Offset     int    `json:"offset"`
	IDs        []uint `json:"ids"`
	HasMore    bool   `json:"hasMore"`
}
