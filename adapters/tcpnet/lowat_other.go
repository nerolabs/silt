//go:build !linux && !darwin

package tcpnet

import "net"

// setNotSentLowat is a no-op where the option is not available; pieces still
// interleave above a larger kernel queue (pieces.go).
func setNotSentLowat(net.Conn, int) {}
