package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEffectiveRetryConfig_DefaultWhenUnset guards against JIT source
// provisioning silently running with no retry at all: a source without a
// `retry:` block must get the bounded default rather than a nil config.
func TestEffectiveRetryConfig_DefaultWhenUnset(t *testing.T) {
	for name, spec := range map[string]*schema.VendorComponentSource{
		"nil spec":           nil,
		"spec without retry": {Uri: "github.com/org/repo"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := effectiveRetryConfig(spec)
			require.NotNil(t, cfg)
			require.NotNil(t, cfg.MaxAttempts)
			assert.Equal(t, defaultSourceRetryAttempts, *cfg.MaxAttempts)
			require.NotNil(t, cfg.InitialDelay)
			assert.Equal(t, defaultSourceRetryInitialDelay, *cfg.InitialDelay)
			require.NotNil(t, cfg.MaxDelay)
			assert.Equal(t, defaultSourceRetryMaxDelay, *cfg.MaxDelay)
			assert.Equal(t, schema.BackoffExponential, cfg.BackoffStrategy)
			require.NotNil(t, cfg.Multiplier)
			assert.InDelta(t, defaultSourceRetryMultiplier, *cfg.Multiplier, 0)
			require.NotNil(t, cfg.RandomJitter)
			assert.InDelta(t, defaultSourceRetryJitter, *cfg.RandomJitter, 0)
			// The default must be a policy the retry executor accepts as-is.
			assert.NoError(t, retry.Validate(cfg))
		})
	}
}

// TestEffectiveRetryConfig_ExplicitWins guards against the default
// overriding a source's own policy, including an explicit opt-out
// (max_attempts: 1).
func TestEffectiveRetryConfig_ExplicitWins(t *testing.T) {
	one := 1
	spec := &schema.VendorComponentSource{
		Uri:   "github.com/org/repo",
		Retry: &schema.RetryConfig{MaxAttempts: &one},
	}
	cfg := effectiveRetryConfig(spec)
	assert.Same(t, spec.Retry, cfg)
	assert.Equal(t, 1, *cfg.MaxAttempts)
}
