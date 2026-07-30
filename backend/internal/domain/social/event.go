package social

import "time"

type EventType string

const (
	EventLikeRequested       EventType = "like.requested"
	EventLikeCancelRequested EventType = "like.cancel_requested"
)

type Event struct {
	EventID    string    `json:"eventId"`
	Type       EventType `json:"type"`
	ActorID    uint      `json:"actorId"`
	VideoID    uint      `json:"videoId"`
	Delta      int64     `json:"delta"`
	OccurredAt time.Time `json:"occurredAt"`
}
