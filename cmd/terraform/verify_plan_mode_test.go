package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newVerifyPlanCmd returns a command exposing a bool --verify-plan flag, the
// tri-state flag resolveVerifyPlanMode reads.
func newVerifyPlanCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().Bool("verify-plan", false, "")
	return cmd
}

func TestResolveVerifyPlanMode(t *testing.T) {
	t.Run("flag unset and env unset defers to default", func(t *testing.T) {
		// A bound bool flag registers a viper default, so the unset case MUST stay
		// empty (not off) — otherwise verification would be disabled by default.
		assert.Equal(t, schema.PlanfileVerifyMode(""), resolveVerifyPlanMode(newVerifyPlanCmd()))
	})

	t.Run("--verify-plan forces fail", func(t *testing.T) {
		cmd := newVerifyPlanCmd()
		require.NoError(t, cmd.Flags().Set("verify-plan", "true"))
		assert.Equal(t, schema.PlanfileVerifyFail, resolveVerifyPlanMode(cmd))
	})

	t.Run("--verify-plan=false forces off", func(t *testing.T) {
		cmd := newVerifyPlanCmd()
		require.NoError(t, cmd.Flags().Set("verify-plan", "false"))
		assert.Equal(t, schema.PlanfileVerifyOff, resolveVerifyPlanMode(cmd))
	})

	t.Run("env var forces fail when flag unchanged", func(t *testing.T) {
		t.Setenv("ATMOS_TERRAFORM_VERIFY_PLAN", "true")
		assert.Equal(t, schema.PlanfileVerifyFail, resolveVerifyPlanMode(newVerifyPlanCmd()))
	})

	t.Run("env var false forces off when flag unchanged", func(t *testing.T) {
		t.Setenv("ATMOS_TERRAFORM_VERIFY_PLAN", "false")
		assert.Equal(t, schema.PlanfileVerifyOff, resolveVerifyPlanMode(newVerifyPlanCmd()))
	})

	t.Run("CLI flag wins over env var", func(t *testing.T) {
		t.Setenv("ATMOS_TERRAFORM_VERIFY_PLAN", "true")
		cmd := newVerifyPlanCmd()
		require.NoError(t, cmd.Flags().Set("verify-plan", "false"))
		assert.Equal(t, schema.PlanfileVerifyOff, resolveVerifyPlanMode(cmd))
	})

	t.Run("nil command falls back to env var", func(t *testing.T) {
		t.Setenv("ATMOS_TERRAFORM_VERIFY_PLAN", "true")
		assert.Equal(t, schema.PlanfileVerifyFail, resolveVerifyPlanMode(nil))
	})

	t.Run("unparseable env var is ignored", func(t *testing.T) {
		t.Setenv("ATMOS_TERRAFORM_VERIFY_PLAN", "notabool")
		assert.Equal(t, schema.PlanfileVerifyMode(""), resolveVerifyPlanMode(newVerifyPlanCmd()))
	})
}

// writeTempAtmosProject creates a minimal, self-contained atmos project with
// planfile storage configured (so verifyStoredPlanForDeploy runs past its storage
// gate); the verify argument, when non-empty, sets
// components.terraform.planfiles.verify. It returns the deploy info that resolves
// to the created component.
func writeTempAtmosProject(t *testing.T, verify string) *schema.ConfigAndStacksInfo {
	t.Helper()

	planfiles := "    planfiles:\n      priority:\n        - github\n"
	if verify != "" {
		planfiles += "      verify: " + verify + "\n"
	}
	return writeTempAtmosProjectWithPlanfiles(t, planfiles)
}

// writeTempAtmosProjectWithPlanfiles creates a minimal, self-contained atmos
// project (atmos.yaml + stacks/ + a terraform component dir) in a temp dir and
// chdirs into it so cfg.InitCliConfig succeeds. The planfilesYAML argument is spliced verbatim
// under components.terraform (empty means no planfiles section, i.e. planfile
// storage unconfigured). It returns the deploy info that resolves to the created
// component.
func writeTempAtmosProjectWithPlanfiles(t *testing.T, planfilesYAML string) *schema.ConfigAndStacksInfo {
	t.Helper()
	tmpDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "components", "terraform", "mycomponent"), 0o755))

	atmosYAML := "base_path: \".\"\n" +
		"stacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*.yaml\"\n  excluded_paths: []\n" +
		"components:\n  terraform:\n    base_path: components/terraform\n" + planfilesYAML
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	// A parseable stack file so InitCliConfig's import glob succeeds (an empty
	// stacks dir makes it error, which would short-circuit verifyStoredPlanForDeploy).
	stackYAML := "vars:\n  stage: test-stack\ncomponents:\n  terraform:\n    mycomponent:\n      vars:\n        foo: bar\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "stacks", "test-stack.yaml"), []byte(stackYAML), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	info := &schema.ConfigAndStacksInfo{
		Stack:            "test-stack",
		ComponentFromArg: "mycomponent",
		Component:        "mycomponent",
		FinalComponent:   "mycomponent",
		ContextPrefix:    "test-stack",
		// Off so the verdict is deterministic regardless of whether the test binary
		// itself runs inside CI (ResolveVerifyMode/IsPlanRequired short-circuit to off).
		VerifyPlanMode: schema.PlanfileVerifyOff,
	}
	return info
}

func TestVerifyStoredPlanForDeploy(t *testing.T) {
	t.Run("non-deploy subcommand is a no-op", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "mycomponent", Stack: "prod"}
		assert.NoError(t, verifyStoredPlanForDeploy("apply", info))
	})

	t.Run("deploy with config error defers to main path", func(t *testing.T) {
		// A bare dir with no atmos project makes InitCliConfig fail; the verify
		// step swallows the error and lets the normal execution path surface it.
		t.Chdir(t.TempDir())
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "mycomponent", Stack: "test-stack"}
		assert.NoError(t, verifyStoredPlanForDeploy("deploy", info))
	})

	t.Run("deploy with invalid verify mode errors", func(t *testing.T) {
		info := writeTempAtmosProject(t, "bogus")
		err := verifyStoredPlanForDeploy("deploy", info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})

	t.Run("deploy without planfile storage is a silent no-op", func(t *testing.T) {
		// Regression guard: a deploy with no planfile storage configured must not
		// run verification or emit a "no stored planfile" warning (the storage gate).
		info := writeTempAtmosProjectWithPlanfiles(t, "") // no planfiles section.
		info.VerifyPlanMode = ""                          // flag unset.
		assert.NoError(t, verifyStoredPlanForDeploy("deploy", info))
	})

	t.Run("explicit --verify-plan without planfile storage errors", func(t *testing.T) {
		// An explicit verification request that cannot be honored must fail loudly,
		// not silently apply an unverified fresh plan.
		info := writeTempAtmosProjectWithPlanfiles(t, "") // no planfiles section.
		info.VerifyPlanMode = schema.PlanfileVerifyFail   // --verify-plan(=true).
		err := verifyStoredPlanForDeploy("deploy", info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileStorageNotConfigured)
	})

	t.Run("env-var verify-plan without planfile storage errors", func(t *testing.T) {
		// ATMOS_TERRAFORM_VERIFY_PLAN=true resolves to the same explicit request as
		// the CLI flag and must error the same way without storage.
		info := writeTempAtmosProjectWithPlanfiles(t, "") // no planfiles section.
		t.Setenv("ATMOS_TERRAFORM_VERIFY_PLAN", "true")
		info.VerifyPlanMode = resolveVerifyPlanMode(newVerifyPlanCmd())
		err := verifyStoredPlanForDeploy("deploy", info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileStorageNotConfigured)
	})

	t.Run("explicit --verify-plan=false without planfile storage is a no-op", func(t *testing.T) {
		info := writeTempAtmosProjectWithPlanfiles(t, "") // no planfiles section.
		info.VerifyPlanMode = schema.PlanfileVerifyOff    // --verify-plan=false.
		assert.NoError(t, verifyStoredPlanForDeploy("deploy", info))
	})

	t.Run("config-set verify without planfile storage warns but proceeds", func(t *testing.T) {
		// verify set in atmos.yaml with no storage is a misconfiguration, but
		// pre-existing configs may carry it: warn (in logs), do not block.
		info := writeTempAtmosProjectWithPlanfiles(t, "    planfiles:\n      verify: fail\n")
		info.VerifyPlanMode = "" // flag unset.
		assert.NoError(t, verifyStoredPlanForDeploy("deploy", info))
	})

	t.Run("explicit --verify-plan with storage but no stored plan requires it", func(t *testing.T) {
		// Regression guard: with storage configured the explicit flag keeps its
		// existing behavior — a missing stored plan is required and blocks.
		info := writeTempAtmosProject(t, "")
		info.VerifyPlanMode = schema.PlanfileVerifyFail // --verify-plan(=true).
		err := verifyStoredPlanForDeploy("deploy", info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStoredPlanfileMissing)
	})

	t.Run("deploy with no stored planfile does not block (verify off)", func(t *testing.T) {
		info := writeTempAtmosProject(t, "")
		// Storage configured but no stored plan exists and verify is off: the missing
		// plan must not be required.
		assert.NoError(t, verifyStoredPlanForDeploy("deploy", info))
	})

	t.Run("deploy with a stored planfile and verify off skips verification", func(t *testing.T) {
		info := writeTempAtmosProject(t, "")

		// Place a stored planfile where verifyStoredPlanForDeploy looks for it.
		atmosConfig, err := cfg.InitCliConfig(*info, true)
		require.NoError(t, err)
		canonical := e.ConstructTerraformComponentPlanfilePath(&atmosConfig, info)
		stored := filepath.Join(filepath.Dir(canonical), planfile.StoredPlanPrefix+planfile.PlanFilename)
		require.NoError(t, os.MkdirAll(filepath.Dir(stored), 0o755))
		require.NoError(t, os.WriteFile(stored, []byte("stored plan"), 0o644))

		// verify off: a stored plan exists but the drift check is skipped (no terraform).
		assert.NoError(t, verifyStoredPlanForDeploy("deploy", info))
	})
}
