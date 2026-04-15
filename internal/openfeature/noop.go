package openfeature

import (
	ofclient "xata/internal/openfeature/client"

	"github.com/open-feature/go-sdk/openfeature"
)

func NewNoopClient(clientName string) (*ofclient.Client, error) {
	return ofclient.NewClient(clientName, openfeature.NoopProvider{})
}
