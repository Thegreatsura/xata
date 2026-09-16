package client

import (
	"context"
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/rs/zerolog/log"

	"xata/internal/envcfg"
)

type FeatureFlag struct {
	Name           string
	DefaultEnabled bool
}

type OpenFeatureClient interface {
	BoolValue(ctx context.Context, key FeatureFlag) bool
	Track(ctx context.Context, eventName string, details openfeature.TrackingEventDetails)
}

type config struct {
	Overrides map[string]bool `env:"XATA_FEATURE_FLAGS" env-description:"feature flag values that take precedence over the provider, as comma separated name:value pairs"`
}

type Client struct {
	client    *openfeature.Client
	overrides map[string]bool
}

// NewClient creates a new OpenFeature client with the specified name and provider.
func NewClient(clientName string, provider openfeature.FeatureProvider) (*Client, error) {
	var cfg config
	if err := envcfg.Read(&cfg); err != nil {
		return nil, fmt.Errorf("read feature flags config: %w", err)
	}
	if len(cfg.Overrides) > 0 {
		log.Warn().Any("flags", cfg.Overrides).Msg("feature flag overrides active")
	}

	if err := openfeature.SetProviderAndWait(provider); err != nil {
		return nil, err
	}
	return &Client{
		client:    openfeature.NewClient(clientName),
		overrides: cfg.Overrides,
	}, nil
}

// BoolValue evaluates a boolean feature flag using the OpenFeature client,
// unless XATA_FEATURE_FLAGS sets it.
func (c *Client) BoolValue(ctx context.Context, key FeatureFlag) bool {
	if v, ok := c.overrides[key.Name]; ok {
		return v
	}

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
