package social

import "time"

type Like struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"index;not null" json:"userId"`
	VideoID   uint       `gorm:"index;not null" json:"videoId"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
}

func (Like) TableName() string {
	return "likes"
}

type LikeStatus struct {
	VideoID   uint  `json:"videoId"`
	Liked     bool  `json:"liked"`
	LikeCount int64 `json:"likeCount"`
}

type Comment struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       uint       `gorm:"index;not null" json:"userId"`
	UserNickname string     `gorm:"column:user_nickname;->" json:"userNickname"`
	VideoID      uint       `gorm:"index;not null" json:"videoId"`
	Content      string     `gorm:"size:500;not null" json:"content"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	DeletedAt    *time.Time `gorm:"index" json:"-"`
}

func (Comment) TableName() string {
	return "comments"
}
