//go:build linux

package tools

import (
	"fmt"
	"net"
	"syscall"
)

// checkPeerCred verifies that the connecting process's UID is in allowedUIDs.
// Uses SO_PEERCRED (Linux-specific) to obtain the peer's credentials.
// If allowedUIDs is empty, the check is skipped (all UIDs are accepted).
func checkPeerCred(conn net.Conn, allowedUIDs []uint32) error {
	if len(allowedUIDs) == 0 {
		return nil
	}

	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("peercred: connection is not a unix socket")
	}

	raw, err := uc.SyscallConn()
	if err != nil {
		return fmt.Errorf("peercred: SyscallConn: %w", err)
	}

	var cred *syscall.Ucred
	var credErr error
	err = raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return fmt.Errorf("peercred: raw.Control: %w", err)
	}
	if credErr != nil {
		return fmt.Errorf("peercred: GetsockoptUcred: %w", credErr)
	}

	for _, uid := range allowedUIDs {
		if cred.Uid == uid {
			return nil
		}
	}

	return fmt.Errorf("peercred: connection from uid %d (pid %d) not in allowed UIDs %v — rejected",
		cred.Uid, cred.Pid, allowedUIDs)
}
