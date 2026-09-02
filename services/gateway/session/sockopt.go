package session

import (
	"net"
	"syscall"
	"time"
)

// SetConnTCPUserTimeout sets TCP_USER_TIMEOUT on conn's underlying socket.
//
// It is the inbound counterpart to the backend dialer's Control hook: the
// gateway accepts client connections rather than dialing them, so the option
// has to be applied after Accept. conn must expose the raw socket via
// syscall.Conn; callers unwrap any PROXY protocol or TLS wrapper first. A
// conn that is not a socket (a pipe in tests) and a non-positive timeout are
// both no-ops. TCP_USER_TIMEOUT is Linux-only; elsewhere setTCPUserTimeout
// does nothing.
func SetConnTCPUserTimeout(conn net.Conn, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	sysConn, ok := conn.(syscall.Conn)
	if !ok {
		return nil
	}
	raw, err := sysConn.SyscallConn()
	if err != nil {
		return err
	}
	return setTCPUserTimeout(raw, timeout)
}
