//go:build linux || darwin

package tcpnet

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// setNotSentLowat bounds the unsent bytes the kernel holds for conn (pieces.go). A
// conn that is not a socket — a relay splice wrapped in something else — is left as
// it is.
func setNotSentLowat(conn net.Conn, bytes int) {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		_ = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_NOTSENT_LOWAT, bytes)
	})
}
