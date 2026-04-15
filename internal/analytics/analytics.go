package analytics

import (
	"context"

	"xata/internal/analytics/client"
)

type Client = client.Client

func NewClient(ctx context.Context) (Client, error) {
	return NewNoopClient(), nil
}
