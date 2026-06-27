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

func TestEndpoint_URL_DefaultsHostToLocalhost(t *testing.T) {
	// No Host set -> URL falls back to "localhost".
	ep := Endpoint{Ports: map[int]int{4566: 54321}}
	assert.Equal(t, "http://localhost:54321", ep.URL("http"))
}

func TestEndpoint_Authority_DefaultsHostToLocalhost(t *testing.T) {
	ep := Endpoint{Ports: map[int]int{5000: 35000}}
	assert.Equal(t, "localhost:35000", ep.Authority())
}
