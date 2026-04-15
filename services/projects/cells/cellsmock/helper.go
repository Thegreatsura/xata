package cellsmock

import (
	"testing"

	mock "github.com/stretchr/testify/mock"

	clustersv1 "xata/gen/proto/clusters/v1"
	cells "xata/services/projects/cells"
)

func NewCellsMock(t testing.TB, client clustersv1.ClustersServiceClient) cells.Cells {
	m := NewCells(t)

	m.On("GetCellConnection", mock.Anything, mock.Anything, mock.Anything).Return(&clientMock{client}, nil).Maybe()

	return m
}

type clientMock struct {
	clustersv1.ClustersServiceClient
}

func (c *clientMock) Close() error {
	return nil
}
