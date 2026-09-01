//go:build linux

package session

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// setTCPUserTimeout bounds how long unacknowledged data may stay outstanding
// on a socket before the kernel gives up and fails the connection.
//
// Without it, a write that is accepted into the send buffer but never
// acknowledged by the peer is retried under tcp_retries2 for roughly 13-30
// minutes, during which the proxy sees no error at all and keeps the session
// open. Setting this turns a silently black-holed backend write into a prompt,
// attributable connection error.
func setTCPUserTimeout(c syscall.RawConn, timeout time.Duration) error {
	ms := int(timeout.Milliseconds())
	var setErr error
	if err := c.Control(func(fd uintptr) {
		setErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_USER_TIMEOUT, ms)
	}); err != nil {
		return err
	}
	return setErr
}

// backendTCPInfo is the kernel's view of a backend connection, read from
// TCP_INFO. It answers a question the bytes copied by the proxy cannot: how
// much of what we wrote was actually acknowledged by the peer.
type backendTCPInfo struct {
	// BytesAcked is cumulative for the connection, so it includes the startup
	// message sent before the session began. It is therefore slightly larger
	// than the session's own byte count, by a fixed amount per connection.
	BytesAcked   uint64
	BytesRetrans uint64
	Unacked      uint32
	NotsentBytes uint32
	TotalRetrans uint32
}

// readTCPInfo reads TCP_INFO from conn. It returns nil when conn is not a
// kernel socket (a pipe in tests, for instance), which is not an error.
func readTCPInfo(conn net.Conn) (*backendTCPInfo, error) {
	sysConn, ok := conn.(syscall.Conn)
	if !ok {
		return nil, nil
	}
	raw, err := sysConn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var info *unix.TCPInfo
	var getErr error
	if err := raw.Control(func(fd uintptr) {
		info, getErr = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	}); err != nil {
		return nil, err
	}
	if getErr != nil {
		return nil, getErr
	}

	return &backendTCPInfo{
		BytesAcked:   info.Bytes_acked,
		BytesRetrans: info.Bytes_retrans,
		Unacked:      info.Unacked,
		NotsentBytes: info.Notsent_bytes,
		TotalRetrans: info.Total_retrans,
	}, nil
}
