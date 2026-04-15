package serverless

import (
	"encoding/binary"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/require"
)

func TestParseStartupPipeline(t *testing.T) {
	startupMsg := &pgproto3.StartupMessage{
		ProtocolVersion: 196608,
		Parameters:      map[string]string{"user": "testuser", "database": "testdb"},
	}
	startupData, err := startupMsg.Encode(nil)
	require.NoError(t, err)

	passwordMsg := &pgproto3.PasswordMessage{Password: "secret"}
	passwordData, err := passwordMsg.Encode(nil)
	require.NoError(t, err)

	tests := map[string]struct {
		data            []byte
		wantStartupLen  int
		wantPipelined   bool
		wantPipelineLen int
		wantErr         bool
	}{
		"startup only": {
			data:           startupData,
			wantStartupLen: len(startupData),
			wantPipelined:  false,
		},
		"startup + password coalesced": {
			data:            append(startupData, passwordData...),
			wantStartupLen:  len(startupData),
			wantPipelined:   true,
			wantPipelineLen: len(passwordData),
		},
		"too short": {
			data:    []byte{0, 0},
			wantErr: true,
		},
		"nil data": {
			data:    nil,
			wantErr: true,
		},
		"length exceeds data": {
			data: func() []byte {
				buf := make([]byte, 8)
				binary.BigEndian.PutUint32(buf[:4], 100)
				binary.BigEndian.PutUint32(buf[4:8], 196608)
				return buf
			}(),
			wantErr: true,
		},
		"oversized startup": {
			data: func() []byte {
				buf := make([]byte, maxStartupPacketLen+100)
				binary.BigEndian.PutUint32(buf[:4], uint32(len(buf)))
				binary.BigEndian.PutUint32(buf[4:8], 196608) // protocol 3.0
				return buf
			}(),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			startup, pipelined, err := parseStartupPipeline(tc.data)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantStartupLen, len(startup))
			if tc.wantPipelined {
				require.NotNil(t, pipelined)
				require.Equal(t, tc.wantPipelineLen, len(pipelined))
			} else {
				require.Nil(t, pipelined)
			}
		})
	}
}

func TestExtractPassword(t *testing.T) {
	tests := map[string]struct {
		msg           []byte
		want          string
		wantRemaining []byte
		wantErr       bool
	}{
		"valid password message": {
			msg: func() []byte {
				msg := &pgproto3.PasswordMessage{Password: "secret"}
				data, _ := msg.Encode(nil)
				return data
			}(),
			want: "secret",
		},
		"empty password": {
			msg: func() []byte {
				msg := &pgproto3.PasswordMessage{Password: ""}
				data, _ := msg.Encode(nil)
				return data
			}(),
			want: "",
		},
		"password with trailing query data": {
			msg: func() []byte {
				pw := &pgproto3.PasswordMessage{Password: "secret"}
				data, _ := pw.Encode(nil)
				query := []byte{byte('P'), 0, 0, 0, 10, 'S', 'E', 'L', 'E', 'C', 'T'}
				return append(data, query...)
			}(),
			want:          "secret",
			wantRemaining: []byte{byte('P'), 0, 0, 0, 10, 'S', 'E', 'L', 'E', 'C', 'T'},
		},
		"too short": {
			msg:     []byte{0, 0, 0},
			wantErr: true,
		},
		"wrong type": {
			msg:     []byte{'Q', 0, 0, 0, 6, 'x', 0},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, remaining, err := extractPassword(tc.msg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantRemaining, remaining)
		})
	}
}

func TestParseStartupParams(t *testing.T) {
	tests := map[string]struct {
		params       map[string]string
		rawData      []byte
		wantUser     string
		wantDatabase string
		wantParams   map[string]string
		wantErr      bool
	}{
		"basic": {
			params:       map[string]string{"user": "testuser", "database": "testdb"},
			wantUser:     "testuser",
			wantDatabase: "testdb",
			wantParams:   map[string]string{},
		},
		"with extra params": {
			params: map[string]string{
				"user":             "testuser",
				"database":         "testdb",
				"application_name": "myapp",
				"client_encoding":  "UTF8",
			},
			wantUser:     "testuser",
			wantDatabase: "testdb",
			wantParams: map[string]string{
				"application_name": "myapp",
				"client_encoding":  "UTF8",
			},
		},
		"no user": {
			params:       map[string]string{"database": "testdb"},
			wantUser:     "",
			wantDatabase: "testdb",
			wantParams:   map[string]string{},
		},
		"too short": {
			rawData: []byte{0, 0, 0, 4},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var data []byte
			if tc.rawData != nil {
				data = tc.rawData
			} else {
				msg := &pgproto3.StartupMessage{
					ProtocolVersion: 196608,
					Parameters:      tc.params,
				}
				var err error
				data, err = msg.Encode(nil)
				require.NoError(t, err)
			}

			user, database, params, err := parseStartupParams(data)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantUser, user)
			require.Equal(t, tc.wantDatabase, database)
			require.Equal(t, tc.wantParams, params)
		})
	}
}
