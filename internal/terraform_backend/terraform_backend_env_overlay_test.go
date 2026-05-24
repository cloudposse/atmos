package terraform_backend_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tb "github.com/cloudposse/atmos/internal/terraform_backend"
)

// TestExtractComponentEnvOverlay verifies the env-overlay extraction used by
// in-process backend readers so `!terraform.state` matches the subprocess env
// overlay `!terraform.output` already applies via SetupEnvironment.
func TestExtractComponentEnvOverlay(t *testing.T) {
	t.Run("nil-when-no-env-section", func(t *testing.T) {
		sections := map[string]any{
			"backend": map[string]any{"type": "s3"},
		}
		got := tb.ExtractComponentEnvOverlay(&sections, tb.ComponentEnvKeysAWS)
		assert.Nil(t, got, "components without an env section produce nil overlay (backward compat hinge)")
	})

	t.Run("nil-when-env-section-empty", func(t *testing.T) {
		sections := map[string]any{
			"env": map[string]any{},
		}
		got := tb.ExtractComponentEnvOverlay(&sections, tb.ComponentEnvKeysAWS)
		assert.Nil(t, got, "empty env section produces nil overlay")
	})

	t.Run("nil-when-no-whitelisted-keys-present", func(t *testing.T) {
		sections := map[string]any{
			"env": map[string]any{
				"SERVICE_DOMAIN": "example.dev",
				"NAMESPACE":      "dev",
				"TF_DATA_DIR":    ".terraform/dev",
			},
		}
		got := tb.ExtractComponentEnvOverlay(&sections, tb.ComponentEnvKeysAWS)
		assert.Nil(t, got, "env without any whitelisted key produces nil — non-whitelisted keys are not exposed to in-process clients")
	})

	t.Run("extracts-aws-profile", func(t *testing.T) {
		sections := map[string]any{
			"env": map[string]any{
				"AWS_PROFILE":    "prod-identity",
				"SERVICE_DOMAIN": "example.com",
			},
		}
		got := tb.ExtractComponentEnvOverlay(&sections, tb.ComponentEnvKeysAWS)
		assert.Equal(t, map[string]string{"AWS_PROFILE": "prod-identity"}, got)
	})

	t.Run("extracts-multiple-whitelisted-keys", func(t *testing.T) {
		sections := map[string]any{
			"env": map[string]any{
				"AWS_PROFILE":                 "prod-identity",
				"AWS_REGION":                  "us-east-1",
				"AWS_CONFIG_FILE":             "/aws/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/aws/creds",
				"SERVICE_DOMAIN":              "example.com",
			},
		}
		got := tb.ExtractComponentEnvOverlay(&sections, tb.ComponentEnvKeysAWS)
		assert.Equal(t, map[string]string{
			"AWS_PROFILE":                 "prod-identity",
			"AWS_REGION":                  "us-east-1",
			"AWS_CONFIG_FILE":             "/aws/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/aws/creds",
		}, got, "non-whitelisted keys filtered out")
	})

	t.Run("coerces-non-string-values-to-string", func(t *testing.T) {
		// Atmos env values can arrive as bool/int when set from go-templating output;
		// the subprocess SetupEnvironment uses fmt.Sprintf to coerce them.
		sections := map[string]any{
			"env": map[string]any{
				"AWS_USE_FIPS_ENDPOINT": true,
				"AWS_REGION":            "us-west-2",
			},
		}
		got := tb.ExtractComponentEnvOverlay(&sections, tb.ComponentEnvKeysAWS)
		assert.Equal(t, map[string]string{
			"AWS_USE_FIPS_ENDPOINT": "true",
			"AWS_REGION":            "us-west-2",
		}, got)
	})

	t.Run("repro-cross-namespace-profile-divergence", func(t *testing.T) {
		// The failure mode this fix addresses: two components in different
		// namespaces resolve to distinct overlays, so the in-process S3 client
		// uses each namespace's AWS profile rather than the calling shell's.
		nsA := map[string]any{
			"env": map[string]any{"AWS_PROFILE": "dev-identity"},
		}
		nsB := map[string]any{
			"env": map[string]any{"AWS_PROFILE": "prod-identity"},
		}
		gotA := tb.ExtractComponentEnvOverlay(&nsA, tb.ComponentEnvKeysAWS)
		gotB := tb.ExtractComponentEnvOverlay(&nsB, tb.ComponentEnvKeysAWS)
		assert.Equal(t, "dev-identity", gotA["AWS_PROFILE"])
		assert.Equal(t, "prod-identity", gotB["AWS_PROFILE"])
		assert.NotEqual(t, gotA["AWS_PROFILE"], gotB["AWS_PROFILE"],
			"distinct namespaces must yield distinct overlays — this is what makes the cache key unique")
	})

	t.Run("nil-overlay-preserves-existing-behavior", func(t *testing.T) {
		// Sentinel for backward compatibility: orgs that don't set whitelisted
		// env keys in any component must continue to use the default AWS
		// credential chain exactly as before.
		sections := map[string]any{
			"backend": map[string]any{"s3": map[string]any{"bucket": "x"}},
			"vars":    map[string]any{"namespace": "anything"},
		}
		assert.Nil(t, tb.ExtractComponentEnvOverlay(&sections, tb.ComponentEnvKeysAWS))
	})
}

// TestComponentEnvKeysAWS_StablePublicSurface guards against accidental
// changes to the whitelist that would either expose non-credential env vars
// or drop a credential one. Add new entries deliberately; this test will fail
// loudly if the list drifts.
func TestComponentEnvKeysAWS_StablePublicSurface(t *testing.T) {
	expected := []string{
		"AWS_PROFILE",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
		"AWS_CONFIG_FILE",
		"AWS_SHARED_CREDENTIALS_FILE",
		"AWS_ENDPOINT_URL_S3",
		"AWS_ENDPOINT_URL_STS",
		"AWS_USE_FIPS_ENDPOINT",
		"AWS_STS_REGIONAL_ENDPOINTS",
	}
	assert.Equal(t, expected, tb.ComponentEnvKeysAWS,
		"whitelist changes require deliberate review — see the issue tracker for rationale before editing")
}
