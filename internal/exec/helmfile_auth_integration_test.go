package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/helmfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHelmAwsProfilePatternNotSetShouldNotCauseError tests that when helm_aws_profile_pattern
// is not set (empty string), it should not cause an error and should fall back to ambient credentials.
func TestHelmAwsProfilePatternNotSetShouldNotCauseError(t *testing.T) {
	// Simulate the scenario from the bug report:
	// User comments out helm_aws_profile_pattern from config, so it's empty string
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				HelmAwsProfilePattern: "", // Empty - user commented it out
			},
		},
	}

	// Simulate a helmfile command without --identity flag
	identity := ""

	// Create auth input
	authInput := helmfile.AuthInput{
		Identity:       identity,
		ProfilePattern: atmosConfig.Components.Helmfile.HelmAwsProfilePattern,
	}

	context := &helmfile.Context{
		Namespace:   "core",
		Tenant:      "",
		Environment: "",
		Stage:       "tooling",
		Region:      "us-gov-west-1",
	}

	// Call ResolveAWSAuth
	authResult, err := helmfile.ResolveAWSAuth(authInput, context)

	// Should NOT return an error
	require.NoError(t, err, "ResolveAWSAuth should not error when helm_aws_profile_pattern is empty")

	// Should use ambient credentials
	assert.Equal(t, "ambient", authResult.Source)
	assert.False(t, authResult.UseIdentityAuth)
	assert.Empty(t, authResult.Profile)
	assert.False(t, authResult.IsDeprecated)
}

// TestHelmAwsProfilePatternSetInConfigIsUsedAndWarns tests that when helm_aws_profile_pattern
// IS set in the config, it should be used and should emit a deprecation warning.
func TestHelmAwsProfilePatternSetInConfigIsUsedAndWarns(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				HelmAwsProfilePattern: "{namespace}--gbl-{stage}-helm", // Set in config
			},
		},
	}

	identity := "" // No --identity flag

	authInput := helmfile.AuthInput{
		Identity:       identity,
		ProfilePattern: atmosConfig.Components.Helmfile.HelmAwsProfilePattern,
	}

	context := &helmfile.Context{
		Namespace:   "core",
		Tenant:      "",
		Environment: "",
		Stage:       "tooling",
		Region:      "us-gov-west-1",
	}

	authResult, err := helmfile.ResolveAWSAuth(authInput, context)

	require.NoError(t, err)

	// Should use the profile pattern
	assert.Equal(t, "pattern", authResult.Source)
	assert.False(t, authResult.UseIdentityAuth)
	assert.Equal(t, "core--gbl-tooling-helm", authResult.Profile)
	assert.True(t, authResult.IsDeprecated, "Should be marked as deprecated")
}

// TestIdentityFlagOverridesProfilePattern tests that --identity flag takes precedence
// over helm_aws_profile_pattern.
func TestIdentityFlagOverridesProfilePattern(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				HelmAwsProfilePattern: "{namespace}--gbl-{stage}-helm", // Set in config
			},
		},
	}

	identity := "my-identity" // User provides --identity

	authInput := helmfile.AuthInput{
		Identity:       identity,
		ProfilePattern: atmosConfig.Components.Helmfile.HelmAwsProfilePattern,
	}

	context := &helmfile.Context{
		Namespace:   "core",
		Tenant:      "",
		Environment: "",
		Stage:       "tooling",
		Region:      "us-gov-west-1",
	}

	authResult, err := helmfile.ResolveAWSAuth(authInput, context)

	require.NoError(t, err)

	// Should use identity, not the profile pattern
	assert.Equal(t, "identity", authResult.Source)
	assert.True(t, authResult.UseIdentityAuth)
	assert.Empty(t, authResult.Profile)
	assert.False(t, authResult.IsDeprecated)
}

// TestIdentityFalseWithProfilePatternFallsBackToPattern tests that --identity=false
// explicitly opts out of identity auth and falls back to profile pattern if set.
func TestIdentityFalseWithProfilePatternFallsBackToPattern(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				HelmAwsProfilePattern: "{namespace}--gbl-{stage}-helm",
			},
		},
	}

	identity := config.IdentityFlagDisabledValue // User provides --identity=false

	authInput := helmfile.AuthInput{
		Identity:       identity,
		ProfilePattern: atmosConfig.Components.Helmfile.HelmAwsProfilePattern,
	}

	context := &helmfile.Context{
		Namespace:   "core",
		Tenant:      "",
		Environment: "",
		Stage:       "tooling",
		Region:      "us-gov-west-1",
	}

	authResult, err := helmfile.ResolveAWSAuth(authInput, context)

	require.NoError(t, err)

	// Should fall back to profile pattern
	assert.Equal(t, "pattern", authResult.Source)
	assert.False(t, authResult.UseIdentityAuth)
	assert.Equal(t, "core--gbl-tooling-helm", authResult.Profile)
	assert.True(t, authResult.IsDeprecated)
}

// TestIdentityFalseWithoutProfilePatternUsesAmbient tests that --identity=false
// without a profile pattern falls back to ambient credentials.
func TestIdentityFalseWithoutProfilePatternUsesAmbient(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				HelmAwsProfilePattern: "", // Not set
			},
		},
	}

	identity := config.IdentityFlagDisabledValue // User provides --identity=false

	authInput := helmfile.AuthInput{
		Identity:       identity,
		ProfilePattern: atmosConfig.Components.Helmfile.HelmAwsProfilePattern,
	}

	context := &helmfile.Context{
		Namespace:   "core",
		Tenant:      "",
		Environment: "",
		Stage:       "tooling",
		Region:      "us-gov-west-1",
	}

	authResult, err := helmfile.ResolveAWSAuth(authInput, context)

	require.NoError(t, err)

	// Should use ambient credentials
	assert.Equal(t, "ambient", authResult.Source)
	assert.False(t, authResult.UseIdentityAuth)
	assert.Empty(t, authResult.Profile)
	assert.False(t, authResult.IsDeprecated)
}
