package client

import (
	"context"

	"xata/internal/analytics/events"
)

// Client defines the interface for analytics operations
type Client interface {
	Track(ctx context.Context, event events.Event)
	TrackGroup(ctx context.Context, event events.Event)
	RegisterGroup(ctx context.Context, groupType, groupKey string) error
	Close(ctx context.Context) error
}
