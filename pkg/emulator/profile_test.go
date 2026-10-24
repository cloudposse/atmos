package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpoint_URL_NoPorts(t *testing.T) {
	ep := Endpoint{Host: "localhost"}
	assert.Empty(t, ep.URL("http"))
}

func TestEndpoint_PrimaryHostPort_LowestContainerPort(t *testing.T) {
	ep := Endpoint{Ports: map[int]int{4588: 30002, 4566: 30001}}
	port, ok := ep.PrimaryHostPort()
	require.True(t, ok)
	assert.Equal(t, 30001, port, "primary is the binding for the lowest container port")
}

func TestEndpoint_Authority(t *testing.T) {
	// "localhost" is normalized to the IPv4 loopback literal so published ports
	// are reachable where localhost resolves to IPv6 ::1 (see loopbackHostToIPv4).
	ep := Endpoint{Host: "localhost", Ports: map[int]int{5000: 35000}}
	assert.Equal(t, "127.0.0.1:35000", ep.Authority())

	empty := Endpoint{Host: "localhost"}
	assert.Empty(t, empty.Authority(), "no bound port → empty authority")
}

func TestEndpoint_PreservesNonLoopbackHost(t *testing.T) {
	// A real hostname must pass through untouched — only loopback/wildcard hosts
	// are rewritten.
	ep := Endpoint{Host: "emulator.internal", Ports: map[int]int{5000: 35000}}
	assert.Equal(t, "http://emulator.internal:35000", ep.URL("http"))
	assert.Equal(t, "emulator.internal:35000", ep.Authority())
}

func TestLoopbackHostToIPv4(t *testing.T) {
	for _, in := range []string{"", "localhost", "::1", "[::1]", "::", "[::]", "0.0.0.0"} {
		assert.Equal(t, "127.0.0.1", loopbackHostToIPv4(in), "loopback/wildcard %q → IPv4 loopback", in)
	}
	for _, in := range []string{"example.com", "emulator.internal", "10.0.0.5", "host.docker.internal"} {
		assert.Equal(t, in, loopbackHostToIPv4(in), "non-loopback %q preserved", in)
	}
}
