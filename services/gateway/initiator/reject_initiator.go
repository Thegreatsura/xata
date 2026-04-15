package initiator

import (
	"context"
	"net"

	"xata/services/gateway/session"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgproto3"
)

type RejectInitiator struct{}

func NewRejectInitiator() *RejectInitiator {
	return &RejectInitiator{}
}

func (r *RejectInitiator) InitSession(ctx context.Context, sessionID string, conn net.Conn) (session.Session, error) {
	// Just send error response. We ignore any startup message from the client.
	inbound := pgproto3.NewBackend(conn, conn)
	inbound.Send(&pgproto3.ErrorResponse{
		Severity: "ERROR",
		Code:     pgerrcode.SQLServerRejectedEstablishmentOfSQLConnection,
		Message:  "xata PostgreSQL rejected establishment of new SQL connection.",
	})
	return session.NoopSession(), inbound.Flush()
}
