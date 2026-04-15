package openebs_test

import (
	"context"
	"testing"

	"xata/services/clusters/internal/connectors/openebs"

	"github.com/stretchr/testify/require"
)

func TestNoopConnectorReturnsNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	connector := &openebs.NoopConnector{}

	bytes, err := connector.AvailableSpaceBytes(ctx)

	require.NoError(t, err)
	require.Nil(t, bytes)
}
