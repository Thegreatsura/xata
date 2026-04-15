package openfeature

import (
	"context"

	"xata/internal/openfeature/client"
)

type Client = client.OpenFeatureClient

type FeatureFlag = client.FeatureFlag

func NewClient(ctx context.Context, clientName string) (Client, error) {
	return NewNoopClient(clientName)
}
