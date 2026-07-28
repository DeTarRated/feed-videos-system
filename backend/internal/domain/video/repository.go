package video

import "context"

type Repository interface {
	Create(ctx context.Context, v *Video) error
	FindByID(ctx context.Context, id uint) (*Video, error)
	ListLatest(ctx context.Context, limit int) ([]Video, error)
	ListByAuthor(ctx context.Context, authorID uint, limit int) ([]Video, error)
	CountByAuthor(ctx context.Context, authorID uint) (int64, error)
}
