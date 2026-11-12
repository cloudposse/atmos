//go:build !darwin && !linux && !windows

package agent

import (
	"errors"
	"net"
)

// errNoPeerCredentials means this platform has no peer credential check, so every connection is
// refused rather than served unchecked.
var errNoPeerCredentials = errors.New("peer credentials are not available on this platform")

func peerUID(*net.UnixConn) (uint32, error) { return 0, errNoPeerCredentials }
