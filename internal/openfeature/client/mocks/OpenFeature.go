package mocks

import (
	"context"

	"xata/internal/openfeature/client"

	"github.com/open-feature/go-sdk/openfeature"
)

type Client struct {
	flags map[string]bool
}

// NewClient creates a new basic openfeature client with the given flag
// overrides. The base set of flags is taken from `openfeature.LocalFlags` and
// then any overrides are applied.
func NewClient(flagOverrides map[client.FeatureFlag]bool) *Client {
	flags := map[string]bool{}

	for k, v := range flagOverrides {
		flags[k.Name] = v
	}

	return &Client{flags: flags}
}

func (c *Client) BoolValue(ctx context.Context, key client.FeatureFlag) bool {
	v, ok := c.flags[key.Name]

	if !ok {
		return key.DefaultEnabled
	}
	return v
}

func (c *Client) Track(ctx context.Context, eventName string, details openfeature.TrackingEventDetails) {
	// No-op for testing
}
