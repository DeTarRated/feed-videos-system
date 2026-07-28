package user

import "time"

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"column:password;size:255;not null" json:"-"`
	Nickname     string    `gorm:"size:64;not null" json:"nickname"`
	AvatarURL    string    `gorm:"size:255;not null;default:''" json:"avatarUrl"`
	Bio          string    `gorm:"size:255;not null;default:''" json:"bio"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (User) TableName() string {
	return "users"
}
