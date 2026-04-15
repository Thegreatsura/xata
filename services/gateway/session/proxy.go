package session

import (
	"context"
	"errors"
	"fmt"
	"net"

	"xata/internal/o11y"
	"xata/services/gateway/metrics"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

var ErrIPNotAllowed = errors.New("forbidden")

type Proxy struct {
	tracer   trace.Tracer
	resolver BranchResolver
	dialer   branchDialerFn
	ipFilter IPFilter
}

// IPFilter checks if an IP address is allowed for a branch.
type IPFilter interface {
	IsAllowed(branchID string, clientAddr string) bool
}

func CheckIPAllowed(filter IPFilter, branchID, clientAddr string) error {
	if filter == nil {
		return nil
	}
	if filter.IsAllowed(branchID, clientAddr) {
		return nil
	}
	return ErrIPNotAllowed
}

type branchDialerFn func(ctx context.Context, network string, branch *Branch) (net.Conn, error)

func NewProxy(tracer trace.Tracer, resolver BranchResolver, dialer branchDialerFn, ipFilter IPFilter) *Proxy {
	return &Proxy{
		tracer:   tracer,
		resolver: resolver,
		dialer:   dialer,
		ipFilter: ipFilter,
	}
}

func (p *Proxy) CreateBackendSession(
	ctx context.Context,
	serverName string,
	inboundConn net.Conn,
	startupMessage *pgproto3.StartupMessage,
) (sess Session, err error) {
	ctx, span := p.tracer.Start(ctx, "create_backend_session")
	defer o11y.CloseSpan(span, &err)

	outConn, branchName, err := p.initProxyConn(ctx, serverName, inboundConn, startupMessage)
	if err != nil {
		return nil, err
	}

	span.SetAttributes(metrics.AttrBranchID.String(branchName))
	return New(p.tracer, branchName, inboundConn, outConn), nil
}

func (p *Proxy) CancelSession(ctx context.Context, serverName string, inboundConn net.Conn, req *pgproto3.CancelRequest) (err error) {
	ctx, span := p.tracer.Start(ctx, "cancel_session")
	defer o11y.CloseSpan(span, &err)

	out, branchName, err := p.initProxyConn(ctx, serverName, inboundConn, req)
	if err != nil {
		return err
	}

	span.SetAttributes(metrics.AttrBranchID.String(branchName))
	return out.Close()
}

func (p *Proxy) initProxyConn(
	ctx context.Context,
	serverName string,
	inboundConn net.Conn,
	msg pgproto3.FrontendMessage,
) (net.Conn, string, error) {
	branch, err := p.resolver.Resolve(ctx, serverName)
	if err != nil {
		return nil, "", err
	}

	clientAddr := inboundConn.RemoteAddr().String()
	log.Ctx(ctx).Debug().
		Str("resolved_address", branch.Address).
		Str("branchName", branch.ID).
		Str("client_ip", clientAddr).
		Msg("resolved branch connection")

	if err := CheckIPAllowed(p.ipFilter, branch.ID, clientAddr); err != nil {
		return nil, "", err
	}

	outConn, err := p.dial(ctx, branch)
	if err != nil {
		return nil, branch.ID, fmt.Errorf("unable to connect to the appropriate database: %w", err)
	}

	if err := sendMessage(outConn, msg); err != nil {
		outConn.Close()
		return nil, branch.ID, fmt.Errorf("send startup message [%T]: %w", msg, err)
	}

	return outConn, branch.ID, nil
}

func (p *Proxy) dial(ctx context.Context, branch *Branch) (conn net.Conn, err error) {
	ctx, span := p.tracer.Start(ctx, "dial", trace.WithAttributes(metrics.AttrAddress.String(branch.Address)))
	defer o11y.CloseSpan(span, &err)

	return p.dialer(ctx, "tcp", branch)
}

func sendMessage(conn net.Conn, msg pgproto3.FrontendMessage) error {
	var buffer [256]byte
	bytes, err := msg.Encode(buffer[:0])
	if err != nil {
		return fmt.Errorf("encode message [%T]: %w", msg, err)
	}
	_, err = conn.Write(bytes)
	if err != nil {
		return fmt.Errorf("send message [%T]: %w", msg, err)
	}
	return nil
}
