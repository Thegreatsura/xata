package kubernetes

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/client-go/rest"
)

type MockConfigProvider struct {
	buildConfigError   bool
	inClusterConfigErr bool
}

func (mcp *MockConfigProvider) BuildConfigFromFlags() (*rest.Config, error) {
	if mcp.buildConfigError {
		return nil, errors.New("build config from flags error")
	}
	return &rest.Config{}, nil
}

func (mcp *MockConfigProvider) InClusterConfig() (*rest.Config, error) {
	if mcp.inClusterConfigErr {
		return nil, errors.New("in-cluster config error")
	}
	return &rest.Config{}, nil
}

// should build config from flags if kube config is not empty
func Test_GetRestConfig(t *testing.T) {
	tests := []struct {
		name               string
		mockConfigProvider *MockConfigProvider
		wantError          error
	}{
		{
			name:               "InClusterConfig is used and works",
			mockConfigProvider: &MockConfigProvider{},
			wantError:          nil,
		},
		{
			name: "InClusterConfig fails, using BuildConfigFromFlags works",
			mockConfigProvider: &MockConfigProvider{
				inClusterConfigErr: true,
				buildConfigError:   false,
			},
			wantError: nil,
		},
		{
			name: "InClusterConfig fails, using BuildConfigFromFlags also fails",
			mockConfigProvider: &MockConfigProvider{
				buildConfigError:   true,
				inClusterConfigErr: true,
			},
			wantError: fmt.Errorf("new err"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetRestConfig(tt.mockConfigProvider)
			if tt.wantError != nil {
				assert.Error(t, err)
				// assert.Equal(t, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
