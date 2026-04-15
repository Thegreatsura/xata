package openebs

import (
	"context"
)

type NoopConnector struct{}

func (nc *NoopConnector) AvailableSpaceBytes(ctx context.Context) (*uint64, error) {
	return nil, nil
}
