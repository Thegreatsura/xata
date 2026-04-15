package api

import (
	"context"
	"fmt"
	"strconv"

	"go.opentelemetry.io/otel/propagation"

	"xata/internal/o11y"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// SetupGRPC initializes and returns a new GRPC control service
func SetupGRPC(o *o11y.O, cfg Config) *grpc.Server {
	logger := o.Logger()

	s := grpc.NewServer(
		grpc.MaxConcurrentStreams(cfg.MaxConcurrentStreams),
		grpc.ConnectionTimeout(cfg.ConnectionTimeout),
		grpc.ChainUnaryInterceptor(
			o11y.GRPCLoggingUnaryServerInterceptor(&logger, o),
			o11y.GRPCRecoverUnaryServerInterceptor(&logger),
			httpStatusCodeErrInterceptor,
		),
		grpc.ChainStreamInterceptor(
			o11y.GRPCLoggingStreamServerInterceptor(&logger),
			o11y.GRPCRecoverStreamServerInterceptor(&logger),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(o),
			otelgrpc.WithMeterProvider(o),
			otelgrpc.WithPropagators(propagation.TraceContext{})),
		),
	)

	// register health endpoints
	healthpb.RegisterHealthServer(s, &healthServer{})

	// Register reflection service on gRPC server
	reflection.Register(s)

	return s
}

type healthServer struct{}

func (s *healthServer) Check(ctx context.Context, in *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *healthServer) List(context.Context, *healthpb.HealthListRequest) (*healthpb.HealthListResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "List not implemented")
}

func (s *healthServer) Watch(in *healthpb.HealthCheckRequest, srv healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "Watch is not implemented")
}

func httpStatusCodeErrInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	resp, err = handler(ctx, req)
	if err != nil {
		err = wrapStatusError(err)
	}
	return resp, err
}

func wrapStatusError(err error) error {
	se := status.Convert(err)
	httpStatus := GetErrorStatusCode(err)
	if httpStatus == 0 || httpStatus >= 500 {
		return err
	}

	withDetails, _ := status.New(se.Code(), ErrorMessage(err)).WithDetails(&errdetails.ErrorInfo{
		Reason: fmt.Sprintf("%T", err),
		Domain: "xata",
		Metadata: map[string]string{
			"httpStatusCode": strconv.Itoa(httpStatus),
		},
	})
	if withDetails == nil {
		return err
	}

	return withDetails.Err()
}
