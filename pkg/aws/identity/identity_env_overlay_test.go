package identity

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// fipsFromConfig inspects the resolved aws.Config's ConfigSources for a
// LoadOptions entry and returns its UseFIPSEndpoint setting. SDK v2 stores
// FIPS preference here rather than on aws.Config directly; this helper uses
// only the public config.LoadOptions type, no internal packages.
func fipsFromConfig(cfg aws.Config) aws.FIPSEndpointState {
	for _, src := range cfg.ConfigSources {
		if lo, ok := src.(config.LoadOptions); ok {
			if lo.UseFIPSEndpoint != aws.FIPSEndpointStateUnset {
				return lo.UseFIPSEndpoint
			}
		}
	}
	return aws.FIPSEndpointStateUnset
}

// TestLoadConfigWithAuthAndEnv_RegionFromOverlay confirms the env overlay's
// AWS_REGION reaches the resolved aws.Config when no explicit region and no
// authContext are provided. This is the codepath that makes `!terraform.state`
// honor the target component's `env.AWS_REGION` the way `!terraform.output`'s
// subprocess already does.
func TestLoadConfigWithAuthAndEnv_RegionFromOverlay(t *testing.T) {
	// Isolate from the test runner's process env so AWS_REGION doesn't leak in.
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")

	ctx := context.Background()
	overlay := map[string]string{"AWS_REGION": "eu-west-2"}

	cfg, err := LoadConfigWithAuthAndEnv(ctx, "", "", 15*time.Minute, nil, overlay)
	require.NoError(t, err)
	assert.Equal(t, "eu-west-2", cfg.Region, "overlay AWS_REGION must reach the resolved config")
}

// TestLoadConfigWithAuthAndEnv_DefaultRegionFallback confirms AWS_DEFAULT_REGION
// is the secondary fallback when AWS_REGION is absent from the overlay.
func TestLoadConfigWithAuthAndEnv_DefaultRegionFallback(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")

	ctx := context.Background()
	overlay := map[string]string{"AWS_DEFAULT_REGION": "ap-southeast-1"}

	cfg, err := LoadConfigWithAuthAndEnv(ctx, "", "", 15*time.Minute, nil, overlay)
	require.NoError(t, err)
	assert.Equal(t, "ap-southeast-1", cfg.Region)
}

// TestLoadConfigWithAuthAndEnv_ExplicitRegionWinsOverOverlay confirms an
// explicit region argument (from `backend.s3.region`) wins over the overlay.
func TestLoadConfigWithAuthAndEnv_ExplicitRegionWinsOverOverlay(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")

	ctx := context.Background()
	overlay := map[string]string{"AWS_REGION": "eu-west-2"}

	cfg, err := LoadConfigWithAuthAndEnv(ctx, "us-east-1", "", 15*time.Minute, nil, overlay)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.Region, "backend.s3.region (explicit arg) must win over overlay AWS_REGION")
}

// TestLoadConfigWithAuthAndEnv_AuthContextWinsOverOverlay confirms that when
// Atmos auth is configured for the call, the overlay is ignored — the Atmos
// auth path stays canonical and this fix layers strictly below it.
func TestLoadConfigWithAuthAndEnv_AuthContextWinsOverOverlay(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")

	ctx := context.Background()
	authContext := &schema.AWSAuthContext{Region: "ca-central-1"}
	overlay := map[string]string{"AWS_REGION": "eu-west-2"}

	cfg, err := LoadConfigWithAuthAndEnv(ctx, "", "", 15*time.Minute, authContext, overlay)
	require.NoError(t, err)
	assert.Equal(t, "ca-central-1", cfg.Region, "authContext.Region must override overlay AWS_REGION")
}

// TestLoadConfigWithAuthAndEnv_FIPSEndpointApplied confirms that
// AWS_USE_FIPS_ENDPOINT from the overlay reaches the resolved config when
// truthy ("true" or "1") and is ignored otherwise. FIPS is a global config
// setting in SDK v2, not a per-service option.
func TestLoadConfigWithAuthAndEnv_FIPSEndpointApplied(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_USE_FIPS_ENDPOINT", "")

	ctx := context.Background()

	t.Run("true-enables-fips", func(t *testing.T) {
		overlay := map[string]string{"AWS_USE_FIPS_ENDPOINT": "true", "AWS_REGION": "us-east-1"}
		cfg, err := LoadConfigWithAuthAndEnv(ctx, "", "", 15*time.Minute, nil, overlay)
		require.NoError(t, err)
		assert.Equal(t, aws.FIPSEndpointStateEnabled, fipsFromConfig(cfg),
			"AWS_USE_FIPS_ENDPOINT=true must enable FIPS on the resolved config")
	})

	t.Run("1-also-enables-fips", func(t *testing.T) {
		overlay := map[string]string{"AWS_USE_FIPS_ENDPOINT": "1", "AWS_REGION": "us-east-1"}
		cfg, err := LoadConfigWithAuthAndEnv(ctx, "", "", 15*time.Minute, nil, overlay)
		require.NoError(t, err)
		assert.Equal(t, aws.FIPSEndpointStateEnabled, fipsFromConfig(cfg))
	})

	t.Run("false-leaves-fips-unset", func(t *testing.T) {
		overlay := map[string]string{"AWS_USE_FIPS_ENDPOINT": "false", "AWS_REGION": "us-east-1"}
		cfg, err := LoadConfigWithAuthAndEnv(ctx, "", "", 15*time.Minute, nil, overlay)
		require.NoError(t, err)
		assert.Equal(t, aws.FIPSEndpointStateUnset, fipsFromConfig(cfg),
			"non-truthy values must leave FIPS unset")
	})

	t.Run("auth-context-suppresses-fips-overlay", func(t *testing.T) {
		// When Atmos auth is configured, the overlay branch is skipped entirely.
		// FIPS from the overlay must not bleed through.
		authContext := &schema.AWSAuthContext{Region: "us-east-1"}
		overlay := map[string]string{"AWS_USE_FIPS_ENDPOINT": "true"}
		cfg, err := LoadConfigWithAuthAndEnv(ctx, "", "", 15*time.Minute, authContext, overlay)
		require.NoError(t, err)
		assert.Equal(t, aws.FIPSEndpointStateUnset, fipsFromConfig(cfg),
			"overlay FIPS must be ignored when authContext is set")
	})
}

// TestLoadConfigWithAuth_PreservesExistingBehavior is the backward-compat
// sentinel: calling the original LoadConfigWithAuth (no overlay parameter)
// must produce the same resolved config as LoadConfigWithAuthAndEnv with a
// nil overlay. This guards every existing call site.
func TestLoadConfigWithAuth_PreservesExistingBehavior(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")

	ctx := context.Background()

	legacy, err := LoadConfigWithAuth(ctx, "us-east-2", "", 15*time.Minute, nil)
	require.NoError(t, err)

	overlayed, err := LoadConfigWithAuthAndEnv(ctx, "us-east-2", "", 15*time.Minute, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, legacy.Region, overlayed.Region,
		"nil overlay path is the LoadConfigWithAuth equivalent — must not drift")
}
