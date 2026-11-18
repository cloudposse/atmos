//go:build darwin || linux

package agent

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerUIDReadsTheRealPeer(t *testing.T) {
	sock := filepath.Join(shortDir(t), "p.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	client, err := net.Dial("unix", sock)
	require.NoError(t, err)
	defer client.Close()
	server := <-accepted
	defer server.Close()

	uid, err := peerUID(server.(*net.UnixConn))
	require.NoError(t, err)
	assert.Equal(t, uint32(os.Getuid()), uid)
}
