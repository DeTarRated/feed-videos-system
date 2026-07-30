package social

import "time"

type EventType string

const (
	EventLikeRequested       EventType = "like.requested"
	EventLikeCancelRequested EventType = "like.cancel_requested"
	EventCommentCreated      EventType = "comment.created"
	EventCommentDeleted      EventType = "comment.deleted"
)

type Event struct {
	EventID    string    `json:"eventId"`
	Type       EventType `json:"type"`
	ActorID    uint      `json:"actorId"`
	VideoID    uint      `json:"videoId"`
	CommentID  uint      `json:"commentId,omitempty"`
	Delta      int64     `json:"delta"`
	OccurredAt time.Time `json:"occurredAt"`
}
