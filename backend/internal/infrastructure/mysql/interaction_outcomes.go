package mysql

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"feed-videos-system/backend/internal/domain/social"
)

var ErrOutcomeFingerprintMismatch = errors.New("outcome fingerprint mismatch")

type InteractionOutcome struct {
	OperationID        string    `gorm:"primaryKey;column:operation_id"`
	RequestFingerprint string    `gorm:"column:request_fingerprint"`
	CommandType        string    `gorm:"column:command_type"`
	BusinessKey        string    `gorm:"column:business_key"`
	OpVersion          uint64    `gorm:"column:op_version"`
	VideoID            uint      `gorm:"column:video_id"`
	DBOutcome          string    `gorm:"column:db_outcome"`
	ResultState        string    `gorm:"column:result_state"`
	LikeCount          int64     `gorm:"column:like_count"`
	CommentCount       int64     `gorm:"column:comment_count"`
	StatsVersion       uint64    `gorm:"column:stats_version"`
	HeatDelta          int64     `gorm:"column:heat_delta"`
	HeatMinute         string    `gorm:"column:heat_minute"`
	CreatedAt          time.Time `gorm:"column:created_at"`
}

func (InteractionOutcome) TableName() string {
	return "interaction_outcomes"
}

type VideoStats struct {
	VideoID      uint      `gorm:"primaryKey;column:video_id"`
	LikeCount    int64     `gorm:"column:like_count"`
	CommentCount int64     `gorm:"column:comment_count"`
	StatsVersion uint64    `gorm:"column:stats_version"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (VideoStats) TableName() string {
	return "video_stats"
}

type OutcomeRepository struct {
	db *gorm.DB
}

func NewOutcomeRepository(db *gorm.DB) *OutcomeRepository {
	return &OutcomeRepository{db: db}
}

func (r *OutcomeRepository) GetOutcome(ctx context.Context, operationID string) (*InteractionOutcome, error) {
	var outcome InteractionOutcome
	err := r.db.WithContext(ctx).Where("operation_id = ?", operationID).First(&outcome).Error
	if err != nil {
		return nil, err
	}
	return &outcome, nil
}

func (r *OutcomeRepository) SaveOutcome(ctx context.Context, outcome *InteractionOutcome) error {
	return r.db.WithContext(ctx).Create(outcome).Error
}

func (r *OutcomeRepository) GetVideoStats(ctx context.Context, videoID uint) (*VideoStats, error) {
	var stats VideoStats
	err := r.db.WithContext(ctx).Where("video_id = ?", videoID).First(&stats).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &VideoStats{VideoID: videoID}, nil
		}
		return nil, err
	}
	return &stats, nil
}

func (r *OutcomeRepository) SaveVideoStats(ctx context.Context, stats *VideoStats) error {
	stats.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(stats).Error
}

func (r *OutcomeRepository) ProcessCommentCreate(ctx context.Context, cmd social.Command, now time.Time) (*InteractionOutcome, error) {
	outcome := &InteractionOutcome{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := getOutcomeTx(tx, cmd.OperationID); err == nil {
			if existing.RequestFingerprint != cmd.RequestFingerprint {
				return ErrOutcomeFingerprintMismatch
			}
			outcome = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var existing social.Comment
		err := tx.Where("id = ?", cmd.CommentID).First(&existing).Error
		if err == nil {
			if existing.RequestFingerprint != cmd.RequestFingerprint {
				return ErrOutcomeFingerprintMismatch
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		comment := social.Comment{
			ID:                 cmd.CommentID,
			UserID:             cmd.ActorID,
			VideoID:            cmd.VideoID,
			Content:            cmd.Content,
			OpVersion:          cmd.OpVersion,
			OperationID:        cmd.OperationID,
			RequestFingerprint: cmd.RequestFingerprint,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		createErr := tx.Create(&comment).Error
		if createErr != nil && !isDuplicateEntry(createErr) {
			return createErr
		}

		var stats *VideoStats
		if isDuplicateEntry(createErr) {
			stats, err = getVideoStatsTx(tx, cmd.VideoID)
		} else {
			stats, err = atomicUpdateStatsTx(tx, cmd.VideoID, 0, 1)
		}
		if err != nil {
			return err
		}

		heatDelta := int64(0)
		heatMinute := ""
		if !isDuplicateEntry(createErr) {
			heatDelta = 3
			heatMinute = cmd.OccurredAt.UTC().Format("200601021504")
		}
		*outcome = InteractionOutcome{
			OperationID:        cmd.OperationID,
			RequestFingerprint: cmd.RequestFingerprint,
			CommandType:        string(cmd.Type),
			BusinessKey:        cmd.BusinessKey(),
			OpVersion:          cmd.OpVersion,
			VideoID:            cmd.VideoID,
			DBOutcome:          dbOutcome(createErr),
			ResultState:        map[bool]string{true: "comment_duplicate", false: "comment_created"}[isDuplicateEntry(createErr)],
			LikeCount:          stats.LikeCount,
			CommentCount:       stats.CommentCount,
			StatsVersion:       stats.StatsVersion,
			HeatDelta:          heatDelta,
			HeatMinute:         heatMinute,
			CreatedAt:          now,
		}
		return tx.Create(outcome).Error
	})
	if err != nil {
		return nil, err
	}
	return outcome, nil
}

func (r *OutcomeRepository) ProcessCommentDelete(ctx context.Context, cmd social.Command, now time.Time) (*InteractionOutcome, error) {
	outcome := &InteractionOutcome{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := getOutcomeTx(tx, cmd.OperationID); err == nil {
			if existing.RequestFingerprint != cmd.RequestFingerprint {
				return ErrOutcomeFingerprintMismatch
			}
			outcome = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		result := tx.Model(&social.Comment{}).
			Where("id = ? AND user_id = ? AND deleted_at IS NULL AND op_version < ?", cmd.CommentID, cmd.ActorID, cmd.OpVersion).
			Updates(map[string]any{"deleted_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}

		var stats *VideoStats
		var err error
		if result.RowsAffected > 0 {
			stats, err = atomicUpdateStatsTx(tx, cmd.VideoID, 0, -1)
		} else {
			stats, err = getVideoStatsTx(tx, cmd.VideoID)
		}
		if err != nil {
			return err
		}

		*outcome = InteractionOutcome{
			OperationID:        cmd.OperationID,
			RequestFingerprint: cmd.RequestFingerprint,
			CommandType:        string(cmd.Type),
			BusinessKey:        cmd.BusinessKey(),
			OpVersion:          cmd.OpVersion,
			VideoID:            cmd.VideoID,
			DBOutcome:          dbOutcomeFromRows(result.RowsAffected),
			ResultState:        "comment_deleted",
			LikeCount:          stats.LikeCount,
			CommentCount:       stats.CommentCount,
			StatsVersion:       stats.StatsVersion,
			HeatDelta:          0,
			HeatMinute:         "",
			CreatedAt:          now,
		}
		return tx.Create(outcome).Error
	})
	if err != nil {
		return nil, err
	}
	return outcome, nil
}

func (r *OutcomeRepository) ProcessLike(ctx context.Context, cmd social.Command, now time.Time) (*InteractionOutcome, error) {
	outcome := &InteractionOutcome{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, err := getOutcomeTx(tx, cmd.OperationID); err == nil {
			if existing.RequestFingerprint != cmd.RequestFingerprint {
				return ErrOutcomeFingerprintMismatch
			}
			outcome = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var like social.Like
		err := tx.Clauses().Where("user_id = ? AND video_id = ?", cmd.ActorID, cmd.VideoID).First(&like).Error
		create := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !create {
			return err
		}

		targetLiked := cmd.Type == social.CommandLikeCreate
		changed := false
		if create {
			like = social.Like{UserID: cmd.ActorID, VideoID: cmd.VideoID}
			if targetLiked {
				like.OpVersion = cmd.OpVersion
				like.DeletedAt = nil
			} else {
				like.OpVersion = cmd.OpVersion
				nowCopy := now
				like.DeletedAt = &nowCopy
			}
			if err := tx.Create(&like).Error; err != nil {
				return err
			}
			changed = targetLiked
		} else if cmd.OpVersion > like.OpVersion {
			like.OpVersion = cmd.OpVersion
			if targetLiked {
				if like.DeletedAt != nil {
					changed = true
				}
				like.DeletedAt = nil
			} else {
				if like.DeletedAt == nil {
					changed = true
				}
				nowCopy := now
				like.DeletedAt = &nowCopy
			}
			if err := tx.Save(&like).Error; err != nil {
				return err
			}
		}

		var stats *VideoStats
		if changed {
			likeDelta := int64(-1)
			if targetLiked {
				likeDelta = 1
			}
			stats, err = atomicUpdateStatsTx(tx, cmd.VideoID, likeDelta, 0)
		} else {
			stats, err = getVideoStatsTx(tx, cmd.VideoID)
		}
		if err != nil {
			return err
		}

		heatDelta := int64(0)
		heatMinute := ""
		if changed && targetLiked {
			heatDelta = 5
			heatMinute = cmd.OccurredAt.UTC().Format("200601021504")
		}

		*outcome = InteractionOutcome{
			OperationID:        cmd.OperationID,
			RequestFingerprint: cmd.RequestFingerprint,
			CommandType:        string(cmd.Type),
			BusinessKey:        cmd.BusinessKey(),
			OpVersion:          cmd.OpVersion,
			VideoID:            cmd.VideoID,
			DBOutcome:          dbOutcomeFromBool(changed),
			ResultState:        map[bool]string{true: "like_changed", false: "like_unchanged"}[changed],
			LikeCount:          stats.LikeCount,
			CommentCount:       stats.CommentCount,
			StatsVersion:       stats.StatsVersion,
			HeatDelta:          heatDelta,
			HeatMinute:         heatMinute,
			CreatedAt:          now,
		}
		return tx.Create(outcome).Error
	})
	if err != nil {
		return nil, err
	}
	return outcome, nil
}

func getOutcomeTx(tx *gorm.DB, operationID string) (*InteractionOutcome, error) {
	var outcome InteractionOutcome
	err := tx.Where("operation_id = ?", operationID).First(&outcome).Error
	if err != nil {
		return nil, err
	}
	return &outcome, nil
}

func getVideoStatsTx(tx *gorm.DB, videoID uint) (*VideoStats, error) {
	var stats VideoStats
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).First(&stats).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &VideoStats{VideoID: videoID}, nil
		}
		return nil, err
	}
	return &stats, nil
}

// atomicUpdateStatsTx atomically applies count deltas and returns the updated stats.
// Uses INSERT ... ON DUPLICATE KEY UPDATE to avoid read-modify-write races.
func atomicUpdateStatsTx(tx *gorm.DB, videoID uint, likeDelta, commentDelta int64) (*VideoStats, error) {
	now := time.Now()
	// Ensure row exists
	if err := tx.Exec(`INSERT IGNORE INTO video_stats (video_id, like_count, comment_count, stats_version, updated_at) VALUES (?, 0, 0, 0, ?)`, videoID, now).Error; err != nil {
		return nil, err
	}
	// Atomically apply deltas
	if err := tx.Exec(`UPDATE video_stats SET like_count = like_count + ?, comment_count = comment_count + ?, stats_version = stats_version + 1, updated_at = ? WHERE video_id = ?`, likeDelta, commentDelta, now, videoID).Error; err != nil {
		return nil, err
	}
	// Read back with lock
	var stats VideoStats
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("video_id = ?", videoID).First(&stats).Error; err != nil {
		return nil, err
	}
	return &stats, nil
}

func isDuplicateEntry(err error) bool {
	return err != nil && (contains(err.Error(), "Duplicate entry") || contains(err.Error(), "1062"))
}

func contains(value, substr string) bool {
	return strings.Contains(value, substr)
}

func dbOutcome(err error) string {
	if err == nil {
		return "applied"
	}
	if isDuplicateEntry(err) {
		return "duplicate"
	}
	return "error"
}

func dbOutcomeFromRows(rows int64) string {
	if rows > 0 {
		return "applied"
	}
	return "unchanged"
}

func dbOutcomeFromBool(changed bool) string {
	if changed {
		return "applied"
	}
	return "unchanged"
}
