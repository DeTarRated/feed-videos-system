package social

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const CommandSchemaVersion = 1

type CommandType string

const (
	CommandLikeCreate    CommandType = "like.create"
	CommandLikeCancel    CommandType = "like.cancel"
	CommandCommentCreate CommandType = "comment.create"
	CommandCommentDelete CommandType = "comment.delete"
)

type Command struct {
	SchemaVersion      int         `json:"schemaVersion"`
	OperationID        string      `json:"operationId"`
	IdempotencyKey     string      `json:"idempotencyKey"`
	RequestFingerprint string      `json:"requestFingerprint"`
	OpVersion          uint64      `json:"opVersion,string"`
	Type               CommandType `json:"type"`
	ActorID            uint        `json:"actorId"`
	VideoID            uint        `json:"videoId"`
	CommentID          string      `json:"commentId,omitempty"`
	Content            string      `json:"content,omitempty"`
	OccurredAt         time.Time   `json:"occurredAt"`
}

func NewCommand(t CommandType, actorID, videoID uint, opVersion uint64, commentID string, content string, idempotencyKey, fingerprint string) Command {
	return Command{
		SchemaVersion:      CommandSchemaVersion,
		OperationID:        uuid.NewString(),
		IdempotencyKey:     idempotencyKey,
		RequestFingerprint: fingerprint,
		OpVersion:          opVersion,
		Type:               t,
		ActorID:            actorID,
		VideoID:            videoID,
		CommentID:          commentID,
		Content:            content,
		OccurredAt:         time.Now().UTC(),
	}
}

func (c Command) BusinessKey() string {
	switch c.Type {
	case CommandLikeCreate, CommandLikeCancel:
		return fmt.Sprintf("like:%d:%d", c.ActorID, c.VideoID)
	case CommandCommentCreate, CommandCommentDelete:
		return fmt.Sprintf("comment:%s", c.CommentID)
	default:
		return fmt.Sprintf("%s:%s", c.Type, c.OperationID)
	}
}

func MarshalCommand(c Command) ([]byte, error) {
	return json.Marshal(c)
}

func UnmarshalCommand(body []byte) (Command, error) {
	var c Command
	if err := json.Unmarshal(body, &c); err != nil {
		return Command{}, err
	}
	if c.SchemaVersion != CommandSchemaVersion {
		return Command{}, fmt.Errorf("unsupported command schema version %d", c.SchemaVersion)
	}
	return c, nil
}

func RequestFingerprint(operation string, actorID, videoID uint, content string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", operation, actorID, videoID, content)))
	return hex.EncodeToString(sum[:])
}

type CommandError struct {
	Err          error
	NonRetryable bool
}

func (e *CommandError) Error() string {
	return e.Err.Error()
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

func NewNonRetryableCommandError(err error) *CommandError {
	return &CommandError{Err: err, NonRetryable: true}
}

func LikeStatusKey(actorID, videoID uint) string {
	return fmt.Sprintf("like:%d:%d", actorID, videoID)
}

func CommentStatusKey(commentID string) string {
	return fmt.Sprintf("comment:%s", commentID)
}
