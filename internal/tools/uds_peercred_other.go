//go:build !linux

package tools

import (
	"log/slog"
	"net"
)

// checkPeerCred is a no-op on non-Linux platforms where SO_PEERCRED is not
// available. It logs a warning when allowedUIDs are configured but cannot
// be enforced.
func checkPeerCred(conn net.Conn, allowedUIDs []uint32) error {
	if len(allowedUIDs) > 0 {
		slog.Warn("peercred: SO_PEERCRED not available on this platform — UID check skipped")
	}
	return nil
}
