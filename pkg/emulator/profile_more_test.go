package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpoint_HostPort(t *testing.T) {
	ep := Endpoint{Ports: map[int]int{4566: 54321}}

	port, ok := ep.HostPort(4566)
	require.True(t, ok)
	assert.Equal(t, 54321, port)

	_, ok = ep.HostPort(9999)
	assert.False(t, ok, "unbound container port has no host port")
}

func TestEndpoint_PrimaryHostPort_Empty(t *testing.T) {
	ep := Endpoint{}
	_, ok := ep.PrimaryHostPort()
	assert.False(t, ok)
}

func TestEndpoint_NetworkIP(t *testing.T) {
	ep := Endpoint{NetworkIPs: map[string]string{
		"z-net": "",
		"a-net": "172.20.0.2",
	}}

	ip, ok := ep.NetworkIP()
	require.True(t, ok)
	assert.Equal(t, "172.20.0.2", ip)
}

func TestEndpoint_URL_DefaultsHostToLoopbackIPv4(t *testing.T) {
	// No Host set -> URL falls back to the IPv4 loopback literal (see
	// loopbackHostToIPv4): "localhost" can resolve to IPv6 ::1, which hangs
	// against an IPv4-only published port on Linux.
	ep := Endpoint{Ports: map[int]int{4566: 54321}}
	assert.Equal(t, "http://127.0.0.1:54321", ep.URL("http"))
}

func TestEndpoint_Authority_DefaultsHostToLoopbackIPv4(t *testing.T) {
	ep := Endpoint{Ports: map[int]int{5000: 35000}}
	assert.Equal(t, "127.0.0.1:35000", ep.Authority())
}
