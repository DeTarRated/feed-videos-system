package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type RegisterInput struct {
	Username string
	Password string
	Nickname string
}

type UpdateProfileInput struct {
	Nickname  string
	AvatarURL string
	Bio       string
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*User, error) {
	username := strings.TrimSpace(input.Username)
	password := strings.TrimSpace(input.Password)
	nickname := strings.TrimSpace(input.Nickname)
	if nickname == "" {
		nickname = username
	}
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("password must be at least 6 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &User{
		Username:     username,
		PasswordHash: string(hash),
		Nickname:     nickname,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (*User, error) {
	u, err := s.repo.FindByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func (s *Service) Get(ctx context.Context, id uint) (*User, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) UpdateProfile(ctx context.Context, id uint, input UpdateProfileInput) (*User, error) {
	if id == 0 {
		return nil, fmt.Errorf("user id is required")
	}
	nickname := strings.TrimSpace(input.Nickname)
	avatarURL := strings.TrimSpace(input.AvatarURL)
	bio := strings.TrimSpace(input.Bio)
	if nickname == "" {
		return nil, fmt.Errorf("nickname is required")
	}
	if len([]rune(nickname)) > 64 {
		return nil, fmt.Errorf("nickname is too long")
	}
	if len([]rune(avatarURL)) > 255 {
		return nil, fmt.Errorf("avatar url is too long")
	}
	if len([]rune(bio)) > 255 {
		return nil, fmt.Errorf("bio is too long")
	}

	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	u.Nickname = nickname
	u.AvatarURL = avatarURL
	u.Bio = bio
	if err := s.repo.UpdateProfile(ctx, u); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *Service) ChangePassword(ctx context.Context, id uint, oldPassword string, newPassword string) error {
	oldPassword = strings.TrimSpace(oldPassword)
	newPassword = strings.TrimSpace(newPassword)
	if id == 0 {
		return fmt.Errorf("user id is required")
	}
	if oldPassword == "" || newPassword == "" {
		return fmt.Errorf("old password and new password are required")
	}
	if len(newPassword) < 6 {
		return fmt.Errorf("new password must be at least 6 characters")
	}

	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.UpdatePassword(ctx, id, string(hash))
}
