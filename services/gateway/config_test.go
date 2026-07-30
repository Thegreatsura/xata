package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	tests := map[string]struct {
		config  Config
		wantErr string
	}{
		"valid": {
			config: Config{ListenAddress: ":7654", DrainingTime: 3500 * time.Second},
		},
		"zero draining time is valid": {
			config: Config{ListenAddress: ":7654"},
		},
		"missing listen address": {
			config:  Config{DrainingTime: 3500 * time.Second},
			wantErr: "listen address is required",
		},
		"negative draining time": {
			config:  Config{ListenAddress: ":7654", DrainingTime: -1 * time.Second},
			wantErr: "draining time must not be negative",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.config.Validate()
			if tt.wantErr == "" {
				require.NoError(t, got)
				return
			}
			require.ErrorContains(t, got, tt.wantErr)
		})
	}
}
