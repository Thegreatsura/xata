package api

import (
	"time"
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
}
