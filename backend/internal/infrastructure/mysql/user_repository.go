package mysql

import (
	"context"

	"gorm.io/gorm"

	"feed-videos-system/backend/internal/domain/user"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *UserRepository) FindByID(ctx context.Context, id uint) (*user.User, error) {
	var u user.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*user.User, error) {
	var u user.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) UpdateProfile(ctx context.Context, u *user.User) error {
	return r.db.WithContext(ctx).
		Model(&user.User{}).
		Where("id = ?", u.ID).
		Updates(map[string]any{
			"nickname":   u.Nickname,
			"avatar_url": u.AvatarURL,
			"bio":        u.Bio,
		}).Error
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id uint, passwordHash string) error {
	return r.db.WithContext(ctx).
		Model(&user.User{}).
		Where("id = ?", id).
		Update("password", passwordHash).Error
}
