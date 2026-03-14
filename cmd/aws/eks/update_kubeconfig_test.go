package eks

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

func TestUpdateKubeconfigCmd_Error(t *testing.T) {
	err := updateKubeconfigCmd.RunE(updateKubeconfigCmd, []string{})
	assert.Error(t, err, "aws eks update-kubeconfig command should return an error when called with no parameters")
}

func TestUpdateKubeconfigCmd_Flags(t *testing.T) {
	// Verify all expected flags are registered.
	flags := updateKubeconfigCmd.Flags()

	tests := []struct {
		name      string
		flagName  string
		shorthand string
	}{
		{name: "stack flag", flagName: "stack", shorthand: "s"},
		{name: "profile flag", flagName: "profile", shorthand: ""},
		{name: "name flag", flagName: "name", shorthand: ""},
		{name: "region flag", flagName: "region", shorthand: ""},
		{name: "kubeconfig flag", flagName: "kubeconfig", shorthand: ""},
		{name: "role-arn flag", flagName: "role-arn", shorthand: ""},
		{name: "dry-run flag", flagName: "dry-run", shorthand: ""},
		{name: "verbose flag", flagName: "verbose", shorthand: ""},
		{name: "alias flag", flagName: "alias", shorthand: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := flags.Lookup(tt.flagName)
			require.NotNil(t, flag, "flag %s should exist", tt.flagName)
			if tt.shorthand != "" {
				assert.Equal(t, tt.shorthand, flag.Shorthand)
			}
		})
	}
}

func TestUpdateKubeconfigCmd_UnexpectedFlags(t *testing.T) {
	// Verify that arbitrary flags do not exist.
	flags := updateKubeconfigCmd.Flags()

	unexpectedFlags := []string{
		"nonexistent-flag",
		"aws-profile",  // We use "profile" not "aws-profile".
		"cluster-name", // We use "name" not "cluster-name".
	}

	for _, flagName := range unexpectedFlags {
		t.Run(flagName, func(t *testing.T) {
			flag := flags.Lookup(flagName)
			assert.Nil(t, flag, "flag %s should not exist", flagName)
		})
	}
}

func TestUpdateKubeconfigCmd_CommandMetadata(t *testing.T) {
	assert.Equal(t, "update-kubeconfig", updateKubeconfigCmd.Use)
	assert.Contains(t, updateKubeconfigCmd.Short, "Update")
	assert.Contains(t, updateKubeconfigCmd.Short, "kubeconfig")
	assert.NotEmpty(t, updateKubeconfigCmd.Long)
}

func TestUpdateKubeconfigCmd_FParseErrWhitelist(t *testing.T) {
	// This command should NOT whitelist unknown flags (strict parsing).
	assert.False(t, updateKubeconfigCmd.FParseErrWhitelist.UnknownFlags)
}

func TestUpdateKubeconfigParser(t *testing.T) {
	// Verify the parser is initialized.
	require.NotNil(t, updateKubeconfigParser, "updateKubeconfigParser should be initialized")
}

// TestUpdateKubeconfigParser_ViperPrefix verifies that the EKS command's flags
// are namespaced under "eks.*" in Viper to prevent key collision with global flags.
// This is the regression test for issue #2076 where AWS_PROFILE env var was
// incorrectly treated as an Atmos configuration profile name.
func TestUpdateKubeconfigParser_ViperPrefix(t *testing.T) {
	// Clear env vars that could interfere (e.g., AWS_REGION set in CI).
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("ATMOS_AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("ATMOS_AWS_REGION", "")
	t.Setenv("ATMOS_STACK", "")

	// Create a fresh Viper instance to avoid polluting the global instance.
	v := viper.New()

	// Create a parser WITH the "eks" prefix (matching production code).
	parser := flags.NewStandardParser(
		flags.WithViperPrefix("eks"),
		flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("region", "", "", "AWS region"),
		flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("region", "ATMOS_AWS_REGION", "AWS_REGION"),
	)

	// Bind to Viper - keys should be namespaced.
	err := parser.BindToViper(v)
	require.NoError(t, err)

	// Verify that the "profile" key at root level is NOT set.
	// Only "eks.profile" should be registered.
	assert.False(t, v.IsSet("profile"), "root 'profile' key should not be set by EKS parser")

	// Verify that the prefixed key exists with the default value.
	assert.Equal(t, "", v.GetString("eks.profile"), "eks.profile should have empty default")
	assert.Equal(t, "", v.GetString("eks.stack"), "eks.stack should have empty default")
	assert.Equal(t, "", v.GetString("eks.region"), "eks.region should have empty default")
}

// TestUpdateKubeconfigParser_AwsProfileDoesNotAffectGlobalProfile verifies that
// setting AWS_PROFILE does not cause the global Viper "profile" key to pick up
// the AWS profile name. This is the core regression test for issue #2076.
func TestUpdateKubeconfigParser_AwsProfileDoesNotAffectGlobalProfile(t *testing.T) {
	v := viper.New()

	// Step 1: Simulate global flag registration (what cmd/root.go init() does).
	// The global "profile" flag is bound to ATMOS_PROFILE.
	globalParser := flags.NewStandardParser(
		flags.WithStringSliceFlag("profile", "p", nil, "Configuration profile"),
		flags.WithEnvVars("profile", "ATMOS_PROFILE"),
	)
	err := globalParser.BindToViper(v)
	require.NoError(t, err)

	// Step 2: Simulate EKS flag registration (what update_kubeconfig.go init() does).
	// WITH the prefix fix, this should not collide with the global "profile" key.
	eksParser := flags.NewStandardParser(
		flags.WithViperPrefix("eks"),
		flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
		flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
	)
	err = eksParser.BindToViper(v)
	require.NoError(t, err)

	// Step 3: Set AWS_PROFILE in the environment.
	t.Setenv("AWS_PROFILE", "my-aws-profile")

	// Step 4: Verify that the global "profile" key does NOT contain the AWS profile.
	// Before the fix, GetStringSlice("profile") would return ["my-aws-profile"]
	// because the EKS init() overwrote the global env binding to include AWS_PROFILE.
	// After the fix, "profile" is still only bound to ATMOS_PROFILE.
	profiles := v.GetStringSlice("profile")
	assert.NotContains(t, profiles, "my-aws-profile",
		"global 'profile' should NOT contain the AWS_PROFILE value; "+
			"AWS_PROFILE should only affect 'eks.profile'")
	assert.Empty(t, profiles,
		"global 'profile' should be empty when only AWS_PROFILE is set")

	// Step 5: Verify that the EKS-scoped key IS affected by AWS_PROFILE.
	assert.True(t, v.IsSet("eks.profile"),
		"'eks.profile' should be set when AWS_PROFILE is set")
	assert.Equal(t, "my-aws-profile", v.GetString("eks.profile"),
		"'eks.profile' should read from AWS_PROFILE")
}

// TestUpdateKubeconfigParser_WithoutPrefix_KeyCollision demonstrates the bug
// that existed before the fix. Without the Viper prefix, the EKS command's
// "profile" flag overwrites the global "profile" key binding, causing AWS_PROFILE
// to be treated as an Atmos configuration profile name.
func TestUpdateKubeconfigParser_WithoutPrefix_KeyCollision(t *testing.T) {
	v := viper.New()

	// Step 1: Global parser binds "profile" → ATMOS_PROFILE.
	globalParser := flags.NewStandardParser(
		flags.WithStringSliceFlag("profile", "p", nil, "Configuration profile"),
		flags.WithEnvVars("profile", "ATMOS_PROFILE"),
	)
	err := globalParser.BindToViper(v)
	require.NoError(t, err)

	// Step 2: EKS parser WITHOUT prefix (the old buggy behavior).
	// This overwrites the global "profile" → ATMOS_PROFILE binding.
	eksParser := flags.NewStandardParser(
		// No WithViperPrefix - this is the bug!
		flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
		flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
	)
	err = eksParser.BindToViper(v)
	require.NoError(t, err)

	// Step 3: Set AWS_PROFILE.
	t.Setenv("AWS_PROFILE", "my-aws-profile")

	// Step 4: The bug - the "profile" Viper key now picks up the AWS_PROFILE value
	// because the EKS parser overwrote the env binding.
	profileValue := v.GetString("profile")
	assert.Equal(t, "my-aws-profile", profileValue,
		"without prefix, global 'profile' key picks up AWS_PROFILE value (the bug)")
}

// TestUpdateKubeconfigParser_BindFlagsToViper verifies that BindFlagsToViper
// with the EKS prefix writes flag values to "eks.*" keys, not root keys.
func TestUpdateKubeconfigParser_BindFlagsToViper(t *testing.T) {
	v := viper.New()

	parser := flags.NewStandardParser(
		flags.WithViperPrefix("eks"),
		flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
	)

	// Create a test command and register flags.
	cmd := &cobra.Command{Use: "test-update-kubeconfig"}
	parser.RegisterFlags(cmd)

	// Simulate setting flags on the command line.
	err := cmd.Flags().Set("profile", "test-profile")
	require.NoError(t, err)
	err = cmd.Flags().Set("stack", "test-stack")
	require.NoError(t, err)

	// Bind flags to Viper.
	err = parser.BindFlagsToViper(cmd, v)
	require.NoError(t, err)

	// Verify values are stored under the prefixed keys.
	assert.Equal(t, "test-profile", v.GetString("eks.profile"),
		"profile value should be stored under eks.profile")
	assert.Equal(t, "test-stack", v.GetString("eks.stack"),
		"stack value should be stored under eks.stack")

	// Verify root keys are NOT affected.
	assert.Empty(t, v.GetString("profile"),
		"root 'profile' key should not be set by EKS parser")
	assert.Empty(t, v.GetString("stack"),
		"root 'stack' key should not be set by EKS parser")
}

// TestUpdateKubeconfigParser_AtmosProfileStillWorks verifies that the fix
// doesn't break ATMOS_PROFILE functionality.
func TestUpdateKubeconfigParser_AtmosProfileStillWorks(t *testing.T) {
	v := viper.New()

	// Global parser.
	globalParser := flags.NewStandardParser(
		flags.WithStringSliceFlag("profile", "p", nil, "Configuration profile"),
		flags.WithEnvVars("profile", "ATMOS_PROFILE"),
	)
	err := globalParser.BindToViper(v)
	require.NoError(t, err)

	// EKS parser with prefix.
	eksParser := flags.NewStandardParser(
		flags.WithViperPrefix("eks"),
		flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
		flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
	)
	err = eksParser.BindToViper(v)
	require.NoError(t, err)

	// Set ATMOS_PROFILE.
	t.Setenv("ATMOS_PROFILE", "dev")

	// Global "profile" should be set by ATMOS_PROFILE.
	assert.True(t, v.IsSet("profile"),
		"global 'profile' key should be set by ATMOS_PROFILE")
}

// TestUpdateKubeconfigParser_BothEnvVarsSet verifies correct behavior when
// both AWS_PROFILE and ATMOS_PROFILE are set simultaneously.
func TestUpdateKubeconfigParser_BothEnvVarsSet(t *testing.T) {
	v := viper.New()

	// Global parser.
	globalParser := flags.NewStandardParser(
		flags.WithStringSliceFlag("profile", "p", nil, "Configuration profile"),
		flags.WithEnvVars("profile", "ATMOS_PROFILE"),
	)
	err := globalParser.BindToViper(v)
	require.NoError(t, err)

	// EKS parser with prefix.
	eksParser := flags.NewStandardParser(
		flags.WithViperPrefix("eks"),
		flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
		flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
	)
	err = eksParser.BindToViper(v)
	require.NoError(t, err)

	// Set both env vars.
	t.Setenv("ATMOS_PROFILE", "atmos-dev")
	t.Setenv("AWS_PROFILE", "aws-production")

	// Global "profile" should read from ATMOS_PROFILE.
	profiles := v.GetStringSlice("profile")
	assert.Contains(t, profiles, "atmos-dev",
		"global 'profile' should read from ATMOS_PROFILE")

	// EKS "eks.profile" should read from AWS_PROFILE (or ATMOS_AWS_PROFILE if set).
	assert.Equal(t, "aws-production", v.GetString("eks.profile"),
		"'eks.profile' should read from AWS_PROFILE")
}
