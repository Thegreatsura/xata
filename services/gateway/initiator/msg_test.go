package initiator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSLYesResponse_Encode(t *testing.T) {
	response := &SSLYesResponse{}
	result, err := response.Encode([]byte{})
	require.NoError(t, err)
	require.Equal(t, []byte{'S'}, result)
}

func TestSSLYesResponse_Decode(t *testing.T) {
	tests := map[string]struct {
		input   []byte
		wantErr bool
	}{
		"non-empty data": {
			input:   []byte{1},
			wantErr: true,
		},
		"empty data": {
			input:   []byte{},
			wantErr: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			response := &SSLYesResponse{}
			err := response.Decode(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNoResponse_Encode(t *testing.T) {
	response := &NoResponse{}
	result, err := response.Encode([]byte{})
	require.NoError(t, err)
	require.Equal(t, []byte{'N'}, result)
}

func TestNoResponse_Decode(t *testing.T) {
	tests := map[string]struct {
		input   []byte
		wantErr bool
	}{
		"non-empty data": {
			input:   []byte{1},
			wantErr: true,
		},
		"empty data": {
			input:   []byte{},
			wantErr: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			response := &NoResponse{}
			err := response.Decode(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
