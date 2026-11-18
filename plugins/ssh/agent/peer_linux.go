//go:build linux

package agent

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peerUID returns the user id of the process on the other end of c, from SO_PEERCRED.
func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("access socket: %w", err)
	}
	var uid uint32
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e != nil {
			credErr = e
			return
		}
		uid = cred.Uid
	}); err != nil {
		return 0, fmt.Errorf("access socket: %w", err)
	}
	if credErr != nil {
		return 0, fmt.Errorf("read peer credentials: %w", credErr)
	}
	return uid, nil
}
