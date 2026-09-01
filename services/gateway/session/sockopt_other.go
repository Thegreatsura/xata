//go:build !linux

package session

import (
	"net"
	"syscall"
	"time"
)

// setTCPUserTimeout is a no-op outside Linux: TCP_USER_TIMEOUT is a
// Linux-specific socket option. The gateway runs on Linux in every deployed
// environment, so this exists to keep local builds and tests working.
func setTCPUserTimeout(_ syscall.RawConn, _ time.Duration) error { return nil }

// backendTCPInfo mirrors the Linux definition so callers compile everywhere.
type backendTCPInfo struct {
	BytesAcked   uint64
	BytesRetrans uint64
	Unacked      uint32
	NotsentBytes uint32
	TotalRetrans uint32
}

// readTCPInfo is a no-op outside Linux: TCP_INFO is Linux-specific. The
// gateway runs on Linux in every deployed environment, so this exists to keep
// local builds and tests working.
func readTCPInfo(_ net.Conn) (*backendTCPInfo, error) { return nil, nil }
