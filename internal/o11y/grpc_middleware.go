package o11y

import (
	"context"
	"fmt"
	"runtime"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultStacktraceSize = 4096

// GRPCRecoverUnaryServerInterceptor returns a new unary server interceptor for
// panic recovery.
func GRPCRecoverUnaryServerInterceptor(logger *zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ any, err error) {
		panicked := true

		defer func() {
			if r := recover(); r != nil || panicked {
				err = handleMiddlewareRecover(ctx, logger, r, defaultStacktraceSize)
				err = status.Errorf(codes.Internal, "%v", err)
			}
		}()

		resp, err := handler(ctx, req)
		panicked = false
		return resp, err
	}
}

// GRPCRecoverStreamServerInterceptor returns a new stream server interceptor for panic recovery.
func GRPCRecoverStreamServerInterceptor(logger *zerolog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		panicked := true

		defer func() {
			if r := recover(); r != nil || panicked {
				err = handleMiddlewareRecover(stream.Context(), logger, r, defaultStacktraceSize)
				err = status.Errorf(codes.Internal, "%v", err)
			}
		}()

		err = handler(srv, stream)
		panicked = false
		return err
	}
}

// GRPCUnaryInterceptorLogs returns a unary server interceptor to handle
// logging.
func GRPCUnaryInterceptorLogs(logger *zerolog.Logger) grpc.DialOption {
	return grpc.WithChainUnaryInterceptor(
		GRPCLoggingUnaryClientInterceptor(logger),
	)
}

// GRPCStreamInterceptorLogs returns a stream server interceptor to handle
// logging.
func GRPCStreamInterceptorLogs(logger *zerolog.Logger) grpc.DialOption {
	return grpc.WithChainStreamInterceptor(
		GRPCLoggingStreamClientInterceptor(logger),
	)
}

// GRPCServerStatHandlers returns a list of dial options to setup the handling
// of traces, metrics and propagators.
func GRPCClientStatHandlers(o *O) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(
			otelgrpc.WithTracerProvider(o),
			otelgrpc.WithMeterProvider(o),
			otelgrpc.WithPropagators(propagation.TraceContext{})),
		),
	}
}

// GRPCServerStatHandlers returns a list of server options to setup the handling
// of traces, metrics and propagators.
func GRPCServerStatHandlers(o *O) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(o),
			otelgrpc.WithMeterProvider(o),
			otelgrpc.WithPropagators(propagation.TraceContext{})),
		),
	}
}

func handleMiddlewareRecover(ctx context.Context, logger *zerolog.Logger, r any, stackTraceSize int) error {
	err, ok := r.(error)
	if !ok {
		err = fmt.Errorf("%v", r)
	}

	span := trace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.RecordError(err)
	}

	evt := logger.WithLevel(zerolog.PanicLevel).Err(err)
	if evt.Enabled() && stackTraceSize > 0 {
		stack := make([]byte, stackTraceSize)
		length := runtime.Stack(stack, false)
		evt.Bytes(StackTrace, stack[:length])
	}
	evt.Msg("[PANIC RECOVER]")

	return err
}
