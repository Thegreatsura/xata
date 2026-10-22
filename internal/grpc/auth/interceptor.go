package grpcauth

import (
	"context"
	"fmt"
	"strings"

	"xata/internal/o11y"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Mode controls what the interceptor does with calls that carry no valid
// token.
type Mode string

const (
	// ModeOff skips token verification entirely.
	ModeOff Mode = "off"
	// ModeLog verifies tokens and logs failures, but lets every call through.
	// Use it to confirm all callers send valid tokens before enforcing.
	ModeLog Mode = "log"
	// ModeEnforce rejects calls without a valid token with codes.Unauthenticated.
	ModeEnforce Mode = "enforce"
)

// ParseMode parses a Mode from its configuration value.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(s)); m {
	case "":
		return ModeOff, nil
	case ModeOff, ModeLog, ModeEnforce:
		return m, nil
	default:
		return "", fmt.Errorf("unknown auth mode %q, want off, log or enforce", s)
	}
}

// TLSMode controls whether the interceptor requires calls to arrive over TLS.
// The listener itself may accept both TLS and plaintext connections; this
// mode decides what plaintext callers may do.
type TLSMode string

const (
	// TLSModeOff does not look at the transport.
	TLSModeOff TLSMode = "off"
	// TLSModeOptional accepts TLS and plaintext alike.
	TLSModeOptional TLSMode = "optional"
	// TLSModeRequire rejects plaintext calls to non-exempt methods.
	TLSModeRequire TLSMode = "require"
)

// ParseTLSMode parses a TLSMode from its configuration value.
func ParseTLSMode(s string) (TLSMode, error) {
	switch m := TLSMode(strings.ToLower(s)); m {
	case TLSModeOff, TLSModeOptional, TLSModeRequire:
		return m, nil
	default:
		return "", fmt.Errorf("unknown tls mode %q, want off, optional or require", s)
	}
}

// exemptPrefixes lists services that never need a token or TLS: health, so
// kubelet probes (plaintext, no credentials) keep working, and reflection,
// which only exposes the schema already published in proto/.
var exemptPrefixes = []string{
	"/grpc.health.v1.Health/",
	"/grpc.reflection.v1.ServerReflection/",
	"/grpc.reflection.v1alpha.ServerReflection/",
}

// DefaultExempt reports whether fullMethod is exempt from authentication.
func DefaultExempt(fullMethod string) bool {
	for _, p := range exemptPrefixes {
		if strings.HasPrefix(fullMethod, p) {
			return true
		}
	}
	return false
}

// Config for the Interceptor.
type Config struct {
	Mode Mode
	// Verifier validates tokens. Required unless Mode is ModeOff.
	Verifier *Verifier
	TLSMode  TLSMode
	// Exempt reports methods that skip every check. Defaults to DefaultExempt.
	Exempt func(fullMethod string) bool
}

// Interceptor verifies bearer tokens on incoming calls.
type Interceptor struct {
	cfg      Config
	logger   zerolog.Logger
	requests metric.Int64Counter
}

// New builds an Interceptor.
func New(o *o11y.O, cfg Config) (*Interceptor, error) {
	if cfg.Mode == "" {
		cfg.Mode = ModeOff
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = TLSModeOff
	}
	if cfg.Mode != ModeOff && cfg.Verifier == nil {
		return nil, fmt.Errorf("auth mode %q requires a verifier", cfg.Mode)
	}
	if cfg.Exempt == nil {
		cfg.Exempt = DefaultExempt
	}

	requests, err := o.Meter("xata.grpc.auth").Int64Counter("xata.grpc.server.auth.requests",
		metric.WithDescription("incoming gRPC calls by authentication result"))
	if err != nil {
		return nil, fmt.Errorf("create auth requests counter: %w", err)
	}

	return &Interceptor{cfg: cfg, logger: o.Logger(), requests: requests}, nil
}

// Unary returns the unary server interceptor.
func (i *Interceptor) Unary() grpc.UnaryServerInterceptor {
	return auth.UnaryServerInterceptor(i.check)
}

// Stream returns the stream server interceptor.
func (i *Interceptor) Stream() grpc.StreamServerInterceptor {
	return auth.StreamServerInterceptor(i.check)
}

const (
	transportTLS       = "tls"
	transportPlaintext = "plaintext"

	resultOK                = "ok"
	resultMissing           = "missing"
	resultInvalid           = "invalid"
	resultSkipped           = "skipped"
	resultPlaintextRejected = "plaintext_rejected"
)

// check is an auth.AuthFunc: it applies the TLS and token policy to one call
// and returns a context carrying the verified caller.
func (i *Interceptor) check(ctx context.Context) (context.Context, error) {
	fullMethod, _ := grpc.Method(ctx)
	if i.cfg.Exempt(fullMethod) {
		return ctx, nil
	}

	transport, peerAddr := transportOf(ctx)
	if i.cfg.TLSMode == TLSModeRequire && transport != transportTLS {
		i.record(ctx, resultPlaintextRejected, "", transport)
		i.logger.Warn().Str("grpc.method", fullMethod).Str("peer", peerAddr).
			Msg("rejected plaintext gRPC call: transport security required")
		return ctx, status.Error(codes.Unauthenticated, "transport security required")
	}

	if i.cfg.Mode == ModeOff {
		i.record(ctx, resultSkipped, "", transport)
		return ctx, nil
	}

	result, issuer, reason := i.verify(ctx)
	i.record(ctx, result, issuer, transport)
	if result == resultOK {
		return ContextWithCaller(ctx, issuer), nil
	}

	evt := i.logger.Warn().Str("grpc.method", fullMethod).Str("peer", peerAddr).
		Str("auth.result", result).Str("auth.mode", string(i.cfg.Mode))
	if reason != nil {
		evt = evt.Err(reason)
	}
	evt.Msg("unauthenticated gRPC call")

	if i.cfg.Mode == ModeEnforce {
		return ctx, status.Error(codes.Unauthenticated, "invalid or missing token")
	}
	return ctx, nil
}

func (i *Interceptor) verify(ctx context.Context) (result, issuer string, reason error) {
	raw, err := auth.AuthFromMD(ctx, "bearer")
	if err != nil {
		return resultMissing, "", nil
	}
	claims, err := i.cfg.Verifier.Verify(raw)
	if err != nil {
		return resultInvalid, "", err
	}
	return resultOK, claims.Issuer, nil
}

func (i *Interceptor) record(ctx context.Context, result, issuer, transport string) {
	i.requests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("result", result),
		attribute.String("issuer", issuer),
		attribute.String("transport", transport),
		attribute.String("mode", string(i.cfg.Mode)),
	))
}

// transportOf reports whether the peer connected over TLS, and its address.
func transportOf(ctx context.Context) (transport, addr string) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return transportPlaintext, ""
	}
	if p.Addr != nil {
		addr = p.Addr.String()
	}
	if _, isTLS := p.AuthInfo.(credentials.TLSInfo); isTLS {
		return transportTLS, addr
	}
	return transportPlaintext, addr
}

type callerKey struct{}

// ContextWithCaller records the verified calling service on the context.
func ContextWithCaller(ctx context.Context, issuer string) context.Context {
	return context.WithValue(ctx, callerKey{}, issuer)
}

// CallerFromContext returns the verified calling service, if any.
func CallerFromContext(ctx context.Context) (string, bool) {
	issuer, ok := ctx.Value(callerKey{}).(string)
	return issuer, ok && issuer != ""
}
