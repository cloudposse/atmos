package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

func TestTerraformFlags(t *testing.T) {
	registry := TerraformFlags()

	// Should have common flags (stack, dry-run) + Terraform-specific flags including identity.
	// Note: Backend execution flags (auto-generate-backend-file, init-run-reconfigure)
	// are in BackendExecutionFlags() and registered per-command.
	assert.GreaterOrEqual(t, registry.Count(), 12)

	// Should include common flags.
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("dry-run"))

	// Should include identity flag for terraform commands.
	assert.True(t, registry.Has("identity"), "identity should be in TerraformFlags for terraform commands")

	// Should include Terraform-specific flags.
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("skip-init"))
	// Note: from-plan is not in shared TerraformFlags - it's defined in apply.go/deploy.go.
	assert.True(t, registry.Has("init-pass-vars"))
	assert.True(t, registry.Has("append-user-agent"))
	assert.True(t, registry.Has("process-templates"))
	assert.True(t, registry.Has("process-functions"))
	assert.True(t, registry.Has("skip"))
	assert.True(t, registry.Has("query"))
	assert.True(t, registry.Has("components"))

	// Check upload-status flag.
	uploadFlag := registry.Get("upload-status")
	require.NotNil(t, uploadFlag)
	boolFlag, ok := uploadFlag.(*flags.BoolFlag)
	require.True(t, ok)
	assert.Equal(t, false, boolFlag.Default)
}

func TestBackendExecutionFlags(t *testing.T) {
	registry := BackendExecutionFlags()

	// Should have 2 backend execution flags.
	assert.Equal(t, 2, registry.Count())

	// Should include backend execution flags.
	assert.True(t, registry.Has("auto-generate-backend-file"), "auto-generate-backend-file should be in BackendExecutionFlags")
	assert.True(t, registry.Has("init-run-reconfigure"), "init-run-reconfigure should be in BackendExecutionFlags")

	// Check auto-generate-backend-file flag.
	autoGenFlag := registry.Get("auto-generate-backend-file")
	require.NotNil(t, autoGenFlag)
	strFlag, ok := autoGenFlag.(*flags.StringFlag)
	require.True(t, ok)
	assert.Equal(t, "", strFlag.Default)
	assert.Equal(t, []string{"ATMOS_AUTO_GENERATE_BACKEND_FILE"}, strFlag.EnvVars)

	// Check init-run-reconfigure flag.
	initReconfFlag := registry.Get("init-run-reconfigure")
	require.NotNil(t, initReconfFlag)
	strFlag, ok = initReconfFlag.(*flags.StringFlag)
	require.True(t, ok)
	assert.Equal(t, "", strFlag.Default)
	assert.Equal(t, []string{"ATMOS_INIT_RUN_RECONFIGURE"}, strFlag.EnvVars)
}

func TestTerraformAffectedFlags(t *testing.T) {
	registry := TerraformAffectedFlags()

	// Should have 7 affected flags.
	assert.Equal(t, 7, registry.Count())

	// Should include all affected flags.
	assert.True(t, registry.Has("repo-path"))
	assert.True(t, registry.Has("ref"))
	assert.True(t, registry.Has("sha"))
	assert.True(t, registry.Has("ssh-key"))
	assert.True(t, registry.Has("ssh-key-password"))
	assert.True(t, registry.Has("include-dependents"))
	assert.True(t, registry.Has("clone-target-ref"))

	// Check repo-path flag.
	repoPathFlag := registry.Get("repo-path")
	require.NotNil(t, repoPathFlag)
	strFlag, ok := repoPathFlag.(*flags.StringFlag)
	require.True(t, ok)
	assert.Equal(t, "", strFlag.Default)
	assert.Equal(t, []string{"ATMOS_REPO_PATH"}, strFlag.EnvVars)
}

func TestWithTerraformFlags(t *testing.T) {
	// Create a standard parser with terraform flags.
	parser := flags.NewStandardParser(
		WithTerraformFlags(),
	)

	registry := parser.Registry()

	// Should have all terraform flags (common + terraform-specific).
	assert.GreaterOrEqual(t, registry.Count(), 12)
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("identity"))
}

func TestWithBackendExecutionFlags(t *testing.T) {
	// Create a standard parser with backend execution flags.
	parser := flags.NewStandardParser(
		WithBackendExecutionFlags(),
	)

	registry := parser.Registry()

	// Should have backend execution flags.
	assert.Equal(t, 2, registry.Count())
	assert.True(t, registry.Has("auto-generate-backend-file"))
	assert.True(t, registry.Has("init-run-reconfigure"))
}

func TestWithTerraformAffectedFlags(t *testing.T) {
	// Create a standard parser with affected flags.
	parser := flags.NewStandardParser(
		WithTerraformAffectedFlags(),
	)

	registry := parser.Registry()

	// Should have all affected flags.
	assert.Equal(t, 7, registry.Count())
	assert.True(t, registry.Has("repo-path"))
	assert.True(t, registry.Has("ref"))
	assert.True(t, registry.Has("include-dependents"))
}

func TestCombinedTerraformFlags(t *testing.T) {
	// Create a standard parser with terraform, affected, and backend execution flags.
	parser := flags.NewStandardParser(
		WithTerraformFlags(),
		WithTerraformAffectedFlags(),
		WithBackendExecutionFlags(),
	)

	registry := parser.Registry()

	// Should have all flags from all registries.
	assert.GreaterOrEqual(t, registry.Count(), 21)

	// Should include terraform flags.
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("identity"))

	// Should include affected flags.
	assert.True(t, registry.Has("repo-path"))
	assert.True(t, registry.Has("ref"))
	assert.True(t, registry.Has("include-dependents"))

	// Should include backend execution flags.
	assert.True(t, registry.Has("auto-generate-backend-file"))
	assert.True(t, registry.Has("init-run-reconfigure"))
}

// TestExecutionFlagsProperties verifies that shared execution flags have correct properties.
// These flags are in TerraformFlags() and shared across all terraform commands.
func TestExecutionFlagsProperties(t *testing.T) {
	registry := TerraformFlags()

	tests := []struct {
		name         string
		flagName     string
		flagType     string // "bool" or "string"
		defaultValue any
		envVars      []string
	}{
		{
			name:         "skip-init is a bool flag with correct defaults",
			flagName:     "skip-init",
			flagType:     "bool",
			defaultValue: false,
			envVars:      []string{"ATMOS_SKIP_INIT"},
		},
		{
			name:         "init-pass-vars is a bool flag with correct defaults",
			flagName:     "init-pass-vars",
			flagType:     "bool",
			defaultValue: false,
			envVars:      []string{"ATMOS_INIT_PASS_VARS"},
		},
		{
			name:         "append-user-agent is a string flag",
			flagName:     "append-user-agent",
			flagType:     "string",
			defaultValue: "",
			envVars:      []string{"ATMOS_APPEND_USER_AGENT"},
		},
		{
			name:         "upload-status is a bool flag with correct defaults",
			flagName:     "upload-status",
			flagType:     "bool",
			defaultValue: false,
			envVars:      []string{"ATMOS_UPLOAD_STATUS"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flag := registry.Get(tc.flagName)
			require.NotNil(t, flag, "%s flag should be registered", tc.flagName)

			switch tc.flagType {
			case "bool":
				boolFlag, ok := flag.(*flags.BoolFlag)
				require.True(t, ok, "%s should be a BoolFlag", tc.flagName)
				assert.Equal(t, tc.defaultValue, boolFlag.Default, "%s default mismatch", tc.flagName)
				assert.Equal(t, tc.envVars, boolFlag.EnvVars, "%s env vars mismatch", tc.flagName)
			case "string":
				strFlag, ok := flag.(*flags.StringFlag)
				require.True(t, ok, "%s should be a StringFlag", tc.flagName)
				assert.Equal(t, tc.defaultValue, strFlag.Default, "%s default mismatch", tc.flagName)
				assert.Equal(t, tc.envVars, strFlag.EnvVars, "%s env vars mismatch", tc.flagName)
			default:
				t.Fatalf("unknown flag type: %s", tc.flagType)
			}
		})
	}
}

// TestFlagsCobraRegistration verifies that flags are properly registered on Cobra commands.
// This test ensures the full pipeline from flag definition to CLI availability works.
func TestFlagsCobraRegistration(t *testing.T) {
	t.Run("shared terraform flags are visible on cobra command", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := flags.NewStandardParser(WithTerraformFlags())
		parser.RegisterFlags(cmd)

		// Verify shared execution flags are registered and visible.
		sharedFlags := []string{
			"skip-init",
			"upload-status",
			"init-pass-vars",
			"append-user-agent",
		}

		for _, flagName := range sharedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "%s flag should be registered on cobra command", flagName)
		}
	})

	t.Run("backend execution flags are visible on cobra command", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := flags.NewStandardParser(WithBackendExecutionFlags())
		parser.RegisterFlags(cmd)

		// Verify backend execution flags are registered.
		backendFlags := []string{
			"auto-generate-backend-file",
			"init-run-reconfigure",
		}

		for _, flagName := range backendFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "%s flag should be registered on cobra command", flagName)
		}
	})

	t.Run("affected flags are visible on cobra command", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := flags.NewStandardParser(WithTerraformAffectedFlags())
		parser.RegisterFlags(cmd)

		// Verify all affected flags are registered.
		affectedFlags := []string{
			"repo-path",
			"ref",
			"sha",
			"ssh-key",
			"ssh-key-password",
			"include-dependents",
			"clone-target-ref",
		}

		for _, flagName := range affectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "%s flag should be registered on cobra command", flagName)
		}
	})
}

// TestFlagsViperBinding verifies that flags are properly bound to Viper for value retrieval.
func TestFlagsViperBinding(t *testing.T) {
	t.Run("shared terraform flags bind to viper", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := flags.NewStandardParser(WithTerraformFlags())
		parser.RegisterFlags(cmd)
		err := parser.BindToViper(v)
		require.NoError(t, err)

		// Set values via viper and verify they can be retrieved.
		v.Set("skip-init", true)
		v.Set("append-user-agent", "atmos/test")

		assert.True(t, v.GetBool("skip-init"))
		assert.Equal(t, "atmos/test", v.GetString("append-user-agent"))
	})

	t.Run("backend execution flags bind to viper", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := flags.NewStandardParser(WithBackendExecutionFlags())
		parser.RegisterFlags(cmd)
		err := parser.BindToViper(v)
		require.NoError(t, err)

		// Set values via viper and verify they can be retrieved.
		v.Set("auto-generate-backend-file", "false")
		v.Set("init-run-reconfigure", "true")

		assert.Equal(t, "false", v.GetString("auto-generate-backend-file"))
		assert.Equal(t, "true", v.GetString("init-run-reconfigure"))
	})

	t.Run("affected flags bind to viper", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := flags.NewStandardParser(WithTerraformAffectedFlags())
		parser.RegisterFlags(cmd)
		err := parser.BindToViper(v)
		require.NoError(t, err)

		// Set values via viper and verify they can be retrieved.
		v.Set("repo-path", "/path/to/repo")
		v.Set("ref", "main")
		v.Set("include-dependents", true)

		assert.Equal(t, "/path/to/repo", v.GetString("repo-path"))
		assert.Equal(t, "main", v.GetString("ref"))
		assert.True(t, v.GetBool("include-dependents"))
	})
}

// getEnvVarsFromFlag extracts EnvVars from a flag regardless of its type.
func getEnvVarsFromFlag(flag flags.Flag) []string {
	switch f := flag.(type) {
	case *flags.BoolFlag:
		return f.EnvVars
	case *flags.StringFlag:
		return f.EnvVars
	default:
		return nil
	}
}

// TestFlagsEnvironmentVariables verifies that environment variables are properly configured.
func TestFlagsEnvironmentVariables(t *testing.T) {
	t.Run("shared terraform flags have correct env var bindings", func(t *testing.T) {
		registry := TerraformFlags()

		envVarTests := []struct {
			flagName string
			envVar   string
		}{
			{"skip-init", "ATMOS_SKIP_INIT"},
			{"upload-status", "ATMOS_UPLOAD_STATUS"},
			{"init-pass-vars", "ATMOS_INIT_PASS_VARS"},
			{"append-user-agent", "ATMOS_APPEND_USER_AGENT"},
		}

		for _, tc := range envVarTests {
			flag := registry.Get(tc.flagName)
			require.NotNil(t, flag, "%s flag should exist", tc.flagName)

			envVars := getEnvVarsFromFlag(flag)
			assert.Contains(t, envVars, tc.envVar,
				"%s flag should have %s env var", tc.flagName, tc.envVar)
		}
	})

	t.Run("backend execution flags have correct env var bindings", func(t *testing.T) {
		registry := BackendExecutionFlags()

		envVarTests := []struct {
			flagName string
			envVar   string
		}{
			{"auto-generate-backend-file", "ATMOS_AUTO_GENERATE_BACKEND_FILE"},
			{"init-run-reconfigure", "ATMOS_INIT_RUN_RECONFIGURE"},
		}

		for _, tc := range envVarTests {
			flag := registry.Get(tc.flagName)
			require.NotNil(t, flag, "%s flag should exist", tc.flagName)

			envVars := getEnvVarsFromFlag(flag)
			assert.Contains(t, envVars, tc.envVar,
				"%s flag should have %s env var", tc.flagName, tc.envVar)
		}
	})

	t.Run("affected flags have correct env var bindings", func(t *testing.T) {
		registry := TerraformAffectedFlags()

		envVarTests := []struct {
			flagName string
			envVar   string
		}{
			{"repo-path", "ATMOS_REPO_PATH"},
			{"ref", "ATMOS_REF"},
			{"sha", "ATMOS_SHA"},
			{"ssh-key", "ATMOS_SSH_KEY"},
			{"ssh-key-password", "ATMOS_SSH_KEY_PASSWORD"},
			{"include-dependents", "ATMOS_INCLUDE_DEPENDENTS"},
			{"clone-target-ref", "ATMOS_CLONE_TARGET_REF"},
		}

		for _, tc := range envVarTests {
			flag := registry.Get(tc.flagName)
			require.NotNil(t, flag, "%s flag should exist", tc.flagName)

			envVars := getEnvVarsFromFlag(flag)
			assert.Contains(t, envVars, tc.envVar,
				"%s flag should have %s env var", tc.flagName, tc.envVar)
		}
	})
}

// TestIdentityFlagConfiguration verifies the identity flag has correct NoOptDefVal for interactive selection.
func TestIdentityFlagConfiguration(t *testing.T) {
	registry := TerraformFlags()
	flag := registry.Get("identity")
	require.NotNil(t, flag)

	strFlag, ok := flag.(*flags.StringFlag)
	require.True(t, ok)

	assert.Equal(t, "", strFlag.Default, "identity default should be empty")
	assert.Equal(t, "__SELECT__", strFlag.NoOptDefVal, "identity NoOptDefVal should be __SELECT__ for interactive selection")
	assert.Equal(t, "i", strFlag.Shorthand, "identity should have -i shorthand")
	assert.Contains(t, strFlag.EnvVars, "ATMOS_IDENTITY")
	assert.Contains(t, strFlag.EnvVars, "IDENTITY")
}
