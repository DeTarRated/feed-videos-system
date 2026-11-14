package social

import "time"

type Like struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"index;not null" json:"userId"`
	VideoID   uint       `gorm:"index;not null" json:"videoId"`
	OpVersion uint64     `gorm:"column:op_version" json:"-"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
}

func (Like) TableName() string {
	return "likes"
}

type LikeStatus struct {
	VideoID     uint   `json:"videoId"`
	Liked       bool   `json:"liked"`
	LikeCount   int64  `json:"likeCount"`
	Status      string `json:"status,omitempty"`
	OperationID string `json:"operationId,omitempty"`
	OpVersion   string `json:"opVersion,omitempty"`
}

type Comment struct {
	ID                 string     `gorm:"primaryKey;size:36;column:id" json:"id"`
	UserID             uint       `gorm:"index;not null" json:"userId"`
	UserNickname       string     `gorm:"column:user_nickname;->" json:"userNickname"`
	VideoID            uint       `gorm:"index;not null" json:"videoId"`
	Content            string     `gorm:"size:500;not null" json:"content"`
	OpVersion          uint64     `gorm:"column:op_version" json:"opVersion,string"`
	OperationID        string     `gorm:"column:operation_id" json:"-"`
	RequestFingerprint string     `gorm:"column:request_fingerprint" json:"-"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	DeletedAt          *time.Time `gorm:"index" json:"-"`
}

func (Comment) TableName() string {
	return "comments"
}

type Follow struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	FollowerID  uint       `gorm:"index;not null" json:"followerId"`
	FollowingID uint       `gorm:"index;not null" json:"followingId"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `gorm:"index" json:"-"`
}

func (Follow) TableName() string {
	return "follows"
}

type FollowStatus struct {
	UserID        uint  `json:"userId"`
	Following     bool  `json:"following"`
	FollowerCount int64 `json:"followerCount"`
}

type FollowUser struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
	Bio       string `json:"bio"`
}
