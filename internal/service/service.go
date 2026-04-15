package service

import (
	"context"

	"xata/internal/o11y"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
)

type Service interface {
	// Name should return the name of the service
	Name() string

	// ReadConfig is used to read the configuration for the service from the environment
	// It will always be run before Setup // Init
	// It should return an error if the configuration is invalid
	ReadConfig(ctx context.Context) error

	// Setup is run after ReadConfig, and is used to setup the service
	// for instance, executing migrations
	// This step may be run multiple times, so it should be idempotent
	// It may also be run from a different process than the one that runs the service
	Setup(ctx context.Context) error

	// Init is run after ReadConfig and before register handlers
	// It is used to initialize the service
	Init(ctx context.Context) error

	// Close is run when the service is shutting down
	// It should clean up any resources used by the service
	Close(ctx context.Context) error
}

// HTTPService should be implemented by services that offer an HTTP API
type HTTPService interface {
	RegisterHTTPHandlers(o *o11y.O, group *echo.Group) error
}

// GRPCService should be implemented by services that offer a gRPC API
type GRPCService interface {
	RegisterGRPCHandlers(o *o11y.O, server *grpc.Server)
}

// RunnerService should be implemented by services that need to run generic tasks
type RunnerService interface {
	Run(ctx context.Context, o *o11y.O) error
}
