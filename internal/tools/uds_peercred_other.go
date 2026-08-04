//go:build !linux

package tools

import (
	"fmt"
	"net"
)

// checkPeerCred fails closed on non-Linux platforms when allowedUIDs are
// configured, since SO_PEERCRED is not available to enforce the check.
func checkPeerCred(conn net.Conn, allowedUIDs []uint32) error {
	if len(allowedUIDs) > 0 {
		return fmt.Errorf("peercred: SO_PEERCRED not available on this platform — cannot enforce UID check")
	}
	return nil
}
