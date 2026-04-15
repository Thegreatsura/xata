package analytics

import (
	"context"

	"xata/internal/analytics/client"
	"xata/internal/analytics/events"
)

type noopClient struct{}

func NewNoopClient() client.Client {
	return &noopClient{}
}

func (c *noopClient) Track(ctx context.Context, event events.Event) {}

func (c *noopClient) TrackGroup(ctx context.Context, event events.Event) {}

func (c *noopClient) RegisterGroup(ctx context.Context, groupType, groupKey string) error {
	return nil
}

func (c *noopClient) Close(ctx context.Context) error {
	return nil
}
