package client

import (
	"context"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/rs/zerolog/log"
)

type FeatureFlag struct {
	Name           string
	DefaultEnabled bool
}

type OpenFeatureClient interface {
	BoolValue(ctx context.Context, key FeatureFlag) bool
	Track(ctx context.Context, eventName string, details openfeature.TrackingEventDetails)
}

type Client struct {
	client *openfeature.Client
}

// NewClient creates a new OpenFeature client with the specified name and provider.
func NewClient(clientName string, provider openfeature.FeatureProvider) (*Client, error) {
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		return nil, err
	}
	return &Client{
		client: openfeature.NewClient(clientName),
	}, nil
}

// BoolValue evaluates a boolean feature flag using the OpenFeature client.
func (c *Client) BoolValue(ctx context.Context, key FeatureFlag) bool {
	v, err := c.client.BooleanValue(ctx, key.Name, key.DefaultEnabled, openfeature.TransactionContext(ctx))
	if err != nil {
		log.Ctx(ctx).Err(err).Msgf("evaluating feature flag %s", key.Name)
	}

	return v
}

// Track sends an analytics event using the current evaluation context.
func (c *Client) Track(ctx context.Context, eventName string, details openfeature.TrackingEventDetails) {
	c.client.Track(ctx, eventName, openfeature.TransactionContext(ctx), details)
}

// NewTrackingEventDetails creates a new TrackingEventDetails with the given value.
func NewTrackingEventDetails(value float64) openfeature.TrackingEventDetails {
	return openfeature.NewTrackingEventDetails(value)
}

// NewTrackingEvent creates a new TrackingEventDetails with zero value.
func NewTrackingEvent() openfeature.TrackingEventDetails {
	return openfeature.NewTrackingEventDetails(0)
}
