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
	// Note: from-plan is defined in apply.go and deploy.go with NoOptDefVal, not here.
	assert.GreaterOrEqual(t, registry.Count(), 17)

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

	// Should include execution flags for controlling terraform behavior.
	// These flags override atmos.yaml settings and are used by various terraform subcommands.
	assert.True(t, registry.Has("auto-generate-backend-file"), "auto-generate-backend-file should be in TerraformFlags")
	assert.True(t, registry.Has("deploy-run-init"), "deploy-run-init should be in TerraformFlags")
	assert.True(t, registry.Has("init-run-reconfigure"), "init-run-reconfigure should be in TerraformFlags")
	assert.True(t, registry.Has("planfile"), "planfile should be in TerraformFlags")
	assert.True(t, registry.Has("skip-planfile"), "skip-planfile should be in TerraformFlags")

	// Check upload-status flag.
	uploadFlag := registry.Get("upload-status")
	require.NotNil(t, uploadFlag)
	boolFlag, ok := uploadFlag.(*flags.BoolFlag)
	require.True(t, ok)
	assert.Equal(t, false, boolFlag.Default)
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

	// Should have all terraform flags.
	assert.GreaterOrEqual(t, registry.Count(), 17)
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("identity"))

	// Verify execution flags are present.
	assert.True(t, registry.Has("auto-generate-backend-file"))
	assert.True(t, registry.Has("deploy-run-init"))
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
	// Create a standard parser with both terraform and affected flags.
	parser := flags.NewStandardParser(
		WithTerraformFlags(),
		WithTerraformAffectedFlags(),
	)

	registry := parser.Registry()

	// Should have all flags from both registries.
	assert.GreaterOrEqual(t, registry.Count(), 24)

	// Should include terraform flags.
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("identity"))

	// Should include affected flags.
	assert.True(t, registry.Has("repo-path"))
	assert.True(t, registry.Has("ref"))
	assert.True(t, registry.Has("include-dependents"))

	// Should include execution flags for controlling terraform behavior.
	assert.True(t, registry.Has("auto-generate-backend-file"))
	assert.True(t, registry.Has("deploy-run-init"))
	assert.True(t, registry.Has("init-run-reconfigure"))
	assert.True(t, registry.Has("planfile"))
	assert.True(t, registry.Has("skip-planfile"))
}

// TestExecutionFlagsProperties verifies that all execution flags have correct properties.
// These flags control terraform execution behavior and override atmos.yaml settings.
func TestExecutionFlagsProperties(t *testing.T) {
	registry := TerraformFlags()

	// Test skip-init flag (bool flag).
	t.Run("skip-init", func(t *testing.T) {
		flag := registry.Get("skip-init")
		require.NotNil(t, flag, "skip-init flag should be registered")
		boolFlag, ok := flag.(*flags.BoolFlag)
		require.True(t, ok, "skip-init should be a BoolFlag")
		assert.Equal(t, false, boolFlag.Default, "skip-init default should be false")
		assert.Equal(t, []string{"ATMOS_SKIP_INIT"}, boolFlag.EnvVars, "skip-init should have ATMOS_SKIP_INIT env var")
	})

	// Test auto-generate-backend-file flag (string flag for true/false override).
	t.Run("auto-generate-backend-file", func(t *testing.T) {
		flag := registry.Get("auto-generate-backend-file")
		require.NotNil(t, flag, "auto-generate-backend-file flag should be registered")
		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "auto-generate-backend-file should be a StringFlag")
		assert.Equal(t, "", strFlag.Default, "auto-generate-backend-file default should be empty")
		assert.Equal(t, []string{"ATMOS_AUTO_GENERATE_BACKEND_FILE"}, strFlag.EnvVars)
	})

	// Test deploy-run-init flag (string flag for true/false override).
	t.Run("deploy-run-init", func(t *testing.T) {
		flag := registry.Get("deploy-run-init")
		require.NotNil(t, flag, "deploy-run-init flag should be registered")
		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "deploy-run-init should be a StringFlag")
		assert.Equal(t, "", strFlag.Default, "deploy-run-init default should be empty")
		assert.Equal(t, []string{"ATMOS_DEPLOY_RUN_INIT"}, strFlag.EnvVars)
	})

	// Test init-run-reconfigure flag (string flag for true/false override).
	t.Run("init-run-reconfigure", func(t *testing.T) {
		flag := registry.Get("init-run-reconfigure")
		require.NotNil(t, flag, "init-run-reconfigure flag should be registered")
		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "init-run-reconfigure should be a StringFlag")
		assert.Equal(t, "", strFlag.Default, "init-run-reconfigure default should be empty")
		assert.Equal(t, []string{"ATMOS_INIT_RUN_RECONFIGURE"}, strFlag.EnvVars)
	})

	// Test init-pass-vars flag (bool flag).
	t.Run("init-pass-vars", func(t *testing.T) {
		flag := registry.Get("init-pass-vars")
		require.NotNil(t, flag, "init-pass-vars flag should be registered")
		boolFlag, ok := flag.(*flags.BoolFlag)
		require.True(t, ok, "init-pass-vars should be a BoolFlag")
		assert.Equal(t, false, boolFlag.Default, "init-pass-vars default should be false")
		assert.Equal(t, []string{"ATMOS_INIT_PASS_VARS"}, boolFlag.EnvVars)
	})

	// Test planfile flag (string flag for path).
	t.Run("planfile", func(t *testing.T) {
		flag := registry.Get("planfile")
		require.NotNil(t, flag, "planfile flag should be registered")
		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "planfile should be a StringFlag")
		assert.Equal(t, "", strFlag.Default, "planfile default should be empty")
		assert.Equal(t, []string{"ATMOS_PLANFILE"}, strFlag.EnvVars)
	})

	// Test skip-planfile flag (string flag for true/false override).
	t.Run("skip-planfile", func(t *testing.T) {
		flag := registry.Get("skip-planfile")
		require.NotNil(t, flag, "skip-planfile flag should be registered")
		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "skip-planfile should be a StringFlag")
		assert.Equal(t, "", strFlag.Default, "skip-planfile default should be empty")
		assert.Equal(t, []string{"ATMOS_SKIP_PLANFILE"}, strFlag.EnvVars)
	})

	// Test append-user-agent flag (string flag).
	t.Run("append-user-agent", func(t *testing.T) {
		flag := registry.Get("append-user-agent")
		require.NotNil(t, flag, "append-user-agent flag should be registered")
		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "append-user-agent should be a StringFlag")
		assert.Equal(t, "", strFlag.Default, "append-user-agent default should be empty")
		assert.Equal(t, []string{"ATMOS_APPEND_USER_AGENT"}, strFlag.EnvVars)
	})

	// Test upload-status flag (bool flag).
	t.Run("upload-status", func(t *testing.T) {
		flag := registry.Get("upload-status")
		require.NotNil(t, flag, "upload-status flag should be registered")
		boolFlag, ok := flag.(*flags.BoolFlag)
		require.True(t, ok, "upload-status should be a BoolFlag")
		assert.Equal(t, false, boolFlag.Default, "upload-status default should be false")
		assert.Equal(t, []string{"ATMOS_UPLOAD_STATUS"}, boolFlag.EnvVars)
	})
}

// TestFlagsCobraRegistration verifies that flags are properly registered on Cobra commands.
// This test ensures the full pipeline from flag definition to CLI availability works.
func TestFlagsCobraRegistration(t *testing.T) {
	t.Run("execution flags are visible on cobra command", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := flags.NewStandardParser(WithTerraformFlags())
		parser.RegisterFlags(cmd)

		// Verify all execution flags are registered and visible.
		executionFlags := []string{
			"skip-init",
			"auto-generate-backend-file",
			"deploy-run-init",
			"init-run-reconfigure",
			"planfile",
			"skip-planfile",
			"upload-status",
			"init-pass-vars",
			"append-user-agent",
		}

		for _, flagName := range executionFlags {
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
	t.Run("execution flags bind to viper", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := flags.NewStandardParser(WithTerraformFlags())
		parser.RegisterFlags(cmd)
		err := parser.BindToViper(v)
		require.NoError(t, err)

		// Set values via viper and verify they can be retrieved.
		v.Set("skip-init", true)
		v.Set("auto-generate-backend-file", "false")
		v.Set("planfile", "/path/to/plan.tfplan")

		assert.True(t, v.GetBool("skip-init"))
		assert.Equal(t, "false", v.GetString("auto-generate-backend-file"))
		assert.Equal(t, "/path/to/plan.tfplan", v.GetString("planfile"))
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

// TestFlagsEnvironmentVariables verifies that environment variables are properly configured.
func TestFlagsEnvironmentVariables(t *testing.T) {
	t.Run("execution flags have correct env var bindings", func(t *testing.T) {
		registry := TerraformFlags()

		envVarTests := []struct {
			flagName string
			envVar   string
		}{
			{"skip-init", "ATMOS_SKIP_INIT"},
			{"auto-generate-backend-file", "ATMOS_AUTO_GENERATE_BACKEND_FILE"},
			{"deploy-run-init", "ATMOS_DEPLOY_RUN_INIT"},
			{"init-run-reconfigure", "ATMOS_INIT_RUN_RECONFIGURE"},
			{"planfile", "ATMOS_PLANFILE"},
			{"skip-planfile", "ATMOS_SKIP_PLANFILE"},
			{"upload-status", "ATMOS_UPLOAD_STATUS"},
			{"init-pass-vars", "ATMOS_INIT_PASS_VARS"},
			{"append-user-agent", "ATMOS_APPEND_USER_AGENT"},
		}

		for _, tc := range envVarTests {
			flag := registry.Get(tc.flagName)
			require.NotNil(t, flag, "%s flag should exist", tc.flagName)

			var envVars []string
			switch f := flag.(type) {
			case *flags.BoolFlag:
				envVars = f.EnvVars
			case *flags.StringFlag:
				envVars = f.EnvVars
			}

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

			var envVars []string
			switch f := flag.(type) {
			case *flags.BoolFlag:
				envVars = f.EnvVars
			case *flags.StringFlag:
				envVars = f.EnvVars
			}

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
