package client

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestParseConfig_CarriesOptional verifies the per-server `optional` flag is threaded from the
// schema into the parsed config (issue #3430, proposal #2).
func TestParseConfig_CarriesOptional(t *testing.T) {
	cfg, err := ParseConfig("central-logs", schema.MCPServerConfig{
		Command:  "atmos",
		Identity: "platform",
		Optional: true,
	})
	assert.NoError(t, err)
	assert.True(t, cfg.Optional)

	cfg, err = ParseConfig("required-server", schema.MCPServerConfig{Command: "atmos"})
	assert.NoError(t, err)
	assert.False(t, cfg.Optional, "optional defaults to false")
}

// TestShouldSkipOptionalServer verifies an optional server is skipped only when it fails because
// its identity cannot be resolved; every other case remains an error.
func TestShouldSkipOptionalServer(t *testing.T) {
	authUnavailable := fmt.Errorf("%w: server %q", errUtils.ErrMCPServerAuthUnavailable, "central-logs")
	identityNotFound := fmt.Errorf("%w: identity %q", errUtils.ErrIdentityNotFound, "central-logging")
	otherErr := fmt.Errorf("some other failure")

	tests := []struct {
		name string
		cfg  *ParsedConfig
		err  error
		want bool
	}{
		{"optional server with auth unavailable is skipped", &ParsedConfig{Optional: true}, authUnavailable, true},
		{"optional server with identity-not-found is skipped", &ParsedConfig{Optional: true}, identityNotFound, true},
		{"optional server with other error is not skipped", &ParsedConfig{Optional: true}, otherErr, false},
		{"required server with auth unavailable is not skipped", &ParsedConfig{Optional: false}, authUnavailable, false},
		{"required server with identity-not-found is not skipped", &ParsedConfig{Optional: false}, identityNotFound, false},
		{"nil config is not skipped", nil, authUnavailable, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldSkipOptionalServer(tt.cfg, tt.err))
		})
	}
}
