package api

import (
	"fmt"
	"time"

	"xata/internal/envcfg"
	grpcauth "xata/internal/grpc/auth"
)

// Config common to all api services
type Config struct {
	// ShutdownTimeout configures for how long the server shall wait for active handlers before we forcefully continue the shutdown.
	// Setting the timeout to 0 disable the timeout (infinite wait).
	ShutdownTimeout time.Duration `env:"XATA_SHUTDOWN_TIMEOUT" env-description:"Configure API server shutdown timeout" env-default:"30s"`

	// MaxConcurrentStreams for the GRPC server
	MaxConcurrentStreams uint32 `env:"XATA_GRPC_MAX_STREAMS" env-default:"10" env-description:"Max concurrent streams accessing the GRPC service"`

	// MaxRequestSize for the API servers (e.g. 30M)
	MaxRequestSize string `env:"XATA_MAX_REQUEST_SIZE" env-default:"30M" env-description:"Max request size for the API servers"`

	// ConnectionTimeout for the GRPC server
	ConnectionTimeout time.Duration `env:"XATA_GRPC_CONNECTION_TIMEOUT" env-default:"600s" env-description:"GRPC server connection timeout"`

	// GRPCAuthMode controls verification of bearer tokens on incoming gRPC
	// calls: off (default), log (verify and log failures, allow all) or
	// enforce (reject calls without a valid token).
	GRPCAuthMode string `env:"XATA_GRPC_AUTH_MODE" env-default:"off" env-description:"gRPC server token verification: off, log or enforce"`

	// GRPCAuthTrustedKeys is a PEM bundle of Ed25519 public keys or X.509
	// certificates (e.g. issued by cert-manager) whose signatures are
	// accepted on incoming tokens.
	GRPCAuthTrustedKeys envcfg.PublicEDDSAKeys `env:"XATA_GRPC_AUTH_TRUSTED_KEYS" env-description:"PEM bundle of Ed25519 public keys or X.509 certificates trusted to sign gRPC auth tokens"`

	// GRPCAuthAudience is the aud claim incoming tokens must carry.
	GRPCAuthAudience string `env:"XATA_GRPC_AUTH_AUDIENCE" env-description:"audience incoming gRPC auth tokens must be issued for"`

	// GRPCAuthLeeway is the clock skew tolerated on token time claims.
	GRPCAuthLeeway time.Duration `env:"XATA_GRPC_AUTH_LEEWAY" env-default:"60s" env-description:"clock skew tolerated when validating gRPC auth tokens"`
}

// AfterRead validates the gRPC auth settings once the environment is read.
func (c *Config) AfterRead() error {
	mode, err := grpcauth.ParseMode(c.GRPCAuthMode)
	if err != nil {
		return fmt.Errorf("XATA_GRPC_AUTH_MODE: %w", err)
	}
	if mode == grpcauth.ModeOff {
		return nil
	}
	if len(c.GRPCAuthTrustedKeys) == 0 {
		return fmt.Errorf("XATA_GRPC_AUTH_TRUSTED_KEYS is required when XATA_GRPC_AUTH_MODE is %q", mode)
	}
	if c.GRPCAuthAudience == "" {
		return fmt.Errorf("XATA_GRPC_AUTH_AUDIENCE is required when XATA_GRPC_AUTH_MODE is %q", mode)
	}
	return nil
}

// GRPCAuth builds the interceptor configuration from the validated settings.
func (c *Config) GRPCAuth() (grpcauth.Config, error) {
	mode, err := grpcauth.ParseMode(c.GRPCAuthMode)
	if err != nil {
		return grpcauth.Config{}, err
	}
	cfg := grpcauth.Config{Mode: mode}
	if mode != grpcauth.ModeOff {
		cfg.Verifier = grpcauth.NewVerifier(c.GRPCAuthTrustedKeys, c.GRPCAuthAudience, c.GRPCAuthLeeway)
	}
	return cfg, nil
}
