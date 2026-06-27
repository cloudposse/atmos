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
	ep := Endpoint{Host: "localhost", Ports: map[int]int{5000: 35000}}
	assert.Equal(t, "localhost:35000", ep.Authority())

	empty := Endpoint{Host: "localhost"}
	assert.Empty(t, empty.Authority(), "no bound port → empty authority")
}
