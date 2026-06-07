package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamRequest_ForwardsHeadersAndUserAgent(t *testing.T) {
	inbound, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/obj/aws.zip", nil)
	require.NoError(t, err)
	inbound.Header.Set("User-Agent", "Terraform/1.9.0 (+https://www.terraform.io) Atmos/1.2.3")
	inbound.Header.Set("Authorization", "Bearer user-token")
	inbound.Header.Set("X-Custom-Registry-Header", "keep-me")
	inbound.Header.Set("Connection", "keep-alive") // hop-by-hop, must be dropped.

	up := UpstreamRequest{URL: "https://registry.example.com/aws.zip"}
	req, err := buildUpstreamRequest(t.Context(), inbound, up)
	require.NoError(t, err)

	// User-Agent forwarded verbatim (Atmos identity rides inside Terraform's UA).
	assert.Equal(t, "Terraform/1.9.0 (+https://www.terraform.io) Atmos/1.2.3", req.Header.Get("User-Agent"))
	// Credentials and custom headers forwarded.
	assert.Equal(t, "Bearer user-token", req.Header.Get("Authorization"))
	assert.Equal(t, "keep-me", req.Header.Get("X-Custom-Registry-Header"))
	// Hop-by-hop header dropped.
	assert.Empty(t, req.Header.Get("Connection"))
}

func TestBuildUpstreamRequest_TFTokenFallback(t *testing.T) {
	t.Setenv("TF_TOKEN_registry_example_com", "env-token")
	inbound, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/obj/aws.zip", nil)
	require.NoError(t, err)

	up := UpstreamRequest{URL: "https://registry.example.com/aws.zip"}
	req, err := buildUpstreamRequest(t.Context(), inbound, up)
	require.NoError(t, err)
	assert.Equal(t, "Bearer env-token", req.Header.Get("Authorization"))
}

func TestBuildUpstreamRequest_InboundAuthBeatsTFToken(t *testing.T) {
	t.Setenv("TF_TOKEN_registry_example_com", "env-token")
	inbound, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/obj/aws.zip", nil)
	require.NoError(t, err)
	inbound.Header.Set("Authorization", "Bearer inbound-token")

	up := UpstreamRequest{URL: "https://registry.example.com/aws.zip"}
	req, err := buildUpstreamRequest(t.Context(), inbound, up)
	require.NoError(t, err)
	assert.Equal(t, "Bearer inbound-token", req.Header.Get("Authorization"))
}

func TestBuildUpstreamRequest_FallbackUserAgentWhenAbsent(t *testing.T) {
	inbound, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/obj/x", nil)
	require.NoError(t, err)
	inbound.Header.Del("User-Agent")

	req, err := buildUpstreamRequest(t.Context(), inbound, UpstreamRequest{URL: "https://r.example.com/x"})
	require.NoError(t, err)
	assert.Contains(t, req.Header.Get("User-Agent"), "Atmos/")
}
