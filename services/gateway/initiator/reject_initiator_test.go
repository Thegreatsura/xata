package initiator

import (
	"context"
	"net"
	"testing"

	"github.com/elastic/go-concert/unison"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/assert"
)

func TestRejectInitiator_InitSession(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	var tg unison.TaskGroup
	var resp pgproto3.BackendMessage
	tg.Go(func(ctx context.Context) error {
		var err error
		frontend := pgproto3.NewFrontend(client, client)
		resp, err = frontend.Receive()
		return err
	})
	tg.Go(func(ctx context.Context) error {
		rejectInitiator := NewRejectInitiator()
		_, err := rejectInitiator.InitSession(context.Background(), "test-session-id", server)
		return err
	})
	assert.NoError(t, tg.Wait())

	errorResponse, ok := resp.(*pgproto3.ErrorResponse)
	assert.True(t, ok)
	assert.Equal(t, "ERROR", errorResponse.Severity)
	assert.Equal(t, "xata PostgreSQL rejected establishment of new SQL connection.", errorResponse.Message)
}
