package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	tf "github.com/cloudposse/atmos/pkg/terraform"
	"github.com/cloudposse/atmos/tests"
)

func TestExecuteTerraformGeneratePlanfileOld(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	componentPath := filepath.Join(stacksPath, "..", "..", "components", "terraform", "mock")
	component := "component-1"
	stack := "nonprod"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join(componentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.terraform.tfvars.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component))
		assert.NoError(t, err)
	}()

	// Create test command with global flags registered (including 'profile').
	cmd := newTestCommandWithGlobalFlags("terraform generate planfile")
	cmd.Short = "Generate a planfile for a Terraform component"
	cmd.Long = "This command generates a `planfile` for a specified Atmos Terraform component."
	cmd.FParseErrWhitelist = struct{ UnknownFlags bool }{UnknownFlags: false}
	cmd.Run = func(cmd *cobra.Command, args []string) {
		err := ExecuteTerraformGeneratePlanfileOld(cmd, args)
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	// Add command-specific flags.
	cmd.PersistentFlags().StringP("stack", "s", "", "Atmos stack")
	cmd.PersistentFlags().StringP("file", "f", "", "Planfile name")
	cmd.PersistentFlags().String("format", "json", "Output format (json or yaml)")
	cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	cmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function when processing Atmos stack manifests")

	// Execute the command
	cmd.SetArgs([]string{component, "-s", stack, "--format", "json"})
	err := cmd.Execute()
	assert.NoError(t, err, "'atmos terraform generate planfile' command should execute without error")

	// Check that the planfile was generated
	filePath := fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}
}

func TestExecuteTerraformGeneratePlanfile(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	componentPath := filepath.Join(stacksPath, "..", "..", "components", "terraform", "mock")
	component := "component-1"
	stack := "nonprod"
	info := schema.ConfigAndStacksInfo{}

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join(componentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.terraform.tfvars.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/new-planfile.json", componentPath))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/planfiles/new-planfile.yaml", componentPath))
		assert.NoError(t, err)
	}()

	options := PlanfileOptions{
		Component:            component,
		Stack:                stack,
		Format:               "json",
		File:                 "",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
	}

	err := ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath := fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component)
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if statErr != nil {
		t.Errorf("Error checking file: %v", statErr)
	}

	options.Format = "yaml"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component)
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if statErr != nil {
		t.Errorf("Error checking file: %v", statErr)
	}

	options.Format = "json"
	options.File = "new-planfile.json"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/new-planfile.json", componentPath)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	options.Format = "yaml"
	options.File = "planfiles/new-planfile.yaml"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/planfiles/new-planfile.yaml", componentPath)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}
}

func TestExecuteTerraformGeneratePlanfileErrors(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	stacksPath := "../../tests/fixtures/scenarios/terraform-generate-planfile"
	component := "component-1"
	stack := "nonprod"
	info := schema.ConfigAndStacksInfo{}

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	options := PlanfileOptions{
		Component:            component,
		Stack:                stack,
		Format:               "",
		File:                 "",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
	}

	options.Format = "invalid-format"
	err := ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFormat)

	options.Format = "json"
	options.Component = "invalid-component"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)

	options.Component = component
	options.Stack = "invalid-stack"
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)

	options.Format = "json"
	options.Stack = stack
	options.Component = ""
	err = ExecuteTerraformGeneratePlanfile(
		&options,
		&info,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoComponent)
}

// TestValidatePlanfileFormat tests the validatePlanfileFormat function.
func TestValidatePlanfileFormat(t *testing.T) {
	tests := []struct {
		name           string
		format         string
		expectedFormat string
		expectError    bool
	}{
		{
			name:           "Empty string defaults to json",
			format:         "",
			expectedFormat: "json",
			expectError:    false,
		},
		{
			name:           "Valid json format",
			format:         "json",
			expectedFormat: "json",
			expectError:    false,
		},
		{
			name:           "Valid yaml format",
			format:         "yaml",
			expectedFormat: "yaml",
			expectError:    false,
		},
		{
			name:        "Invalid format xml",
			format:      "xml",
			expectError: true,
		},
		{
			name:        "Invalid format toml",
			format:      "toml",
			expectError: true,
		},
		{
			name:        "Invalid format random",
			format:      "random",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := tt.format
			err := validatePlanfileFormat(&format)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFormat)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedFormat, format)
			}
		})
	}
}

// TestPlanfileValidateComponent tests the validateComponent function in terraform_generate_planfile.go.
func TestPlanfileValidateComponent(t *testing.T) {
	tests := []struct {
		name        string
		component   string
		expectError bool
	}{
		{
			name:        "Valid component name",
			component:   "vpc",
			expectError: false,
		},
		{
			name:        "Valid component with hyphen",
			component:   "my-component",
			expectError: false,
		},
		{
			name:        "Valid component with underscore",
			component:   "my_component",
			expectError: false,
		},
		{
			name:        "Empty component name",
			component:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateComponent(tt.component)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrNoComponent)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIsTerraformInputFile pins the staleness-gate predicate. Issue #2498.
func TestIsTerraformInputFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"plain tf", "main.tf", true},
		{"tf.json", "main.tf.json", true},
		{"tfvars", "prod.tfvars", true},
		{"tfvars.json", "prod.auto.tfvars.json", true},
		{"backend tf is also a tf file", "backend.tf", true},
		{"unrelated text file", "README.md", false},
		{"hidden lock file is not an input here", ".terraform.lock.hcl", false},
		{"binary planfile is not an input", "prod-vpc.planfile", false},
		{"JSON planfile is not an input", "prod-vpc.planfile.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isTerraformInputFile(tc.in))
		})
	}
}

// TestPlanfileReuseStaleness covers each gate in issue #2498:
// missing binary, lock-file newer, input newer, all-fresh.
func TestPlanfileReuseStaleness(t *testing.T) {
	t.Parallel()

	t.Run("missing binary reports clear reason", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		reason := planfileReuseStaleness(filepath.Join(dir, "does-not-exist.planfile"), dir)
		assert.Contains(t, reason, "does not exist")
	})

	t.Run("binary path is a directory is rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		nested := filepath.Join(dir, "nested")
		require.NoError(t, os.Mkdir(nested, 0o755))
		reason := planfileReuseStaleness(nested, dir)
		assert.Contains(t, reason, "directory")
	})

	t.Run("lock file newer than binary is stale", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		binPath := filepath.Join(dir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now().Add(-time.Hour))
		lockPath := filepath.Join(dir, ".terraform.lock.hcl")
		writeWithMtime(t, lockPath, []byte("lock"), time.Now())

		reason := planfileReuseStaleness(binPath, dir)
		assert.Equal(t, ".terraform.lock.hcl is newer than planfile", reason)
	})

	t.Run("tf input newer than binary is stale", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		binPath := filepath.Join(dir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now().Add(-time.Hour))
		writeWithMtime(t, filepath.Join(dir, "main.tf"), []byte("resource \"null_resource\" \"x\" {}"), time.Now())

		reason := planfileReuseStaleness(binPath, dir)
		assert.Contains(t, reason, "main.tf")
		assert.Contains(t, reason, "newer than planfile")
	})

	t.Run("tfvars.json newer than binary is stale", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		binPath := filepath.Join(dir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now().Add(-time.Hour))
		writeWithMtime(t, filepath.Join(dir, "stack-comp.terraform.tfvars.json"), []byte("{}"), time.Now())

		reason := planfileReuseStaleness(binPath, dir)
		assert.Contains(t, reason, "newer than planfile")
	})

	t.Run("all-fresh inputs report no reason (reusable)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// Inputs are older than the binary.
		past := time.Now().Add(-time.Hour)
		writeWithMtime(t, filepath.Join(dir, "main.tf"), []byte("// tf"), past)
		writeWithMtime(t, filepath.Join(dir, "stack-comp.terraform.tfvars.json"), []byte("{}"), past)
		writeWithMtime(t, filepath.Join(dir, ".terraform.lock.hcl"), []byte("lock"), past)

		binPath := filepath.Join(dir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now())

		assert.Empty(t, planfileReuseStaleness(binPath, dir))
	})

	t.Run("non-input files and subdirs are ignored", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// Newer non-input file and newer subdir must not trigger staleness.
		future := time.Now().Add(time.Hour)
		writeWithMtime(t, filepath.Join(dir, "README.md"), []byte("# notes"), future)
		require.NoError(t, os.Mkdir(filepath.Join(dir, "modules"), 0o755))
		require.NoError(t, os.Chtimes(filepath.Join(dir, "modules"), future, future))

		binPath := filepath.Join(dir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now())

		assert.Empty(t, planfileReuseStaleness(binPath, dir))
	})

	t.Run("lockfile next to binary trips gate even when componentPath differs (workdir provisioner)", func(t *testing.T) {
		t.Parallel()
		// Simulate the workdir-provisioner case: binary lives in a workdir that
		// is NOT the source componentPath. The lockfile sits next to the binary.
		// If the gate only looked in componentPath, this stale-lock case would
		// be missed silently.
		componentDir := t.TempDir()
		workdir := t.TempDir()

		binPath := filepath.Join(workdir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now().Add(-time.Hour))
		writeWithMtime(t, filepath.Join(workdir, ".terraform.lock.hcl"), []byte("lock"), time.Now())

		reason := planfileReuseStaleness(binPath, componentDir)
		assert.Equal(t, ".terraform.lock.hcl is newer than planfile", reason,
			"lockfile gate must look next to the binary, not just in componentPath")
	})

	t.Run("missing component directory reports read failure", func(t *testing.T) {
		t.Parallel()
		// Place the binary in one tmp dir and point componentPath at a path
		// that does not exist. The binary stat succeeds; ReadDir fails.
		binDir := t.TempDir()
		binPath := filepath.Join(binDir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now())

		missingComponentDir := filepath.Join(t.TempDir(), "does-not-exist")
		reason := planfileReuseStaleness(binPath, missingComponentDir)
		assert.Contains(t, reason, "cannot read component directory")
	})
}

// TestPlanfileReuseGateReason confirms the config-level SkipPlanfile gate
// short-circuits before the on-disk staleness scan. Issue #2498.
func TestPlanfileReuseGateReason(t *testing.T) {
	t.Parallel()

	t.Run("skip_planfile=true short-circuits even with a fresh binary on disk", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		binPath := filepath.Join(dir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now())

		cfg := &schema.AtmosConfiguration{}
		cfg.Components.Terraform.Plan.SkipPlanfile = true

		reason := planfileReuseGateReason(cfg, binPath, dir)
		assert.Equal(t,
			"components.terraform.plan.skip_planfile is true; no binary planfile is ever produced",
			reason)
	})

	t.Run("skip_planfile=false defers to on-disk staleness gates", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		binPath := filepath.Join(dir, "stack-comp.planfile")
		writeWithMtime(t, binPath, []byte("plan"), time.Now())

		cfg := &schema.AtmosConfiguration{}
		assert.Empty(t, planfileReuseGateReason(cfg, binPath, dir),
			"with skip_planfile=false and a fresh binary, gate must permit reuse")
	})
}

// TestResolveSourcePlanFileModes covers the ReusePlanMode branching that does
// not require running terraform. Issue #2498.
func TestResolveSourcePlanFileModes(t *testing.T) {
	t.Parallel()

	t.Run("always mode with missing binary returns ErrReusePlanUnavailable", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		info := &schema.ConfigAndStacksInfo{
			ContextPrefix: "prod",
			Component:     "vpc",
		}
		// Construct an atmosConfig whose canonical planfile path lands in dir.
		atmosConfig := &schema.AtmosConfiguration{}
		// constructTerraformComponentPlanfilePath uses componentPath when no
		// workdir provisioner is set; pass dir as componentPath to make the
		// canonical path match dir/<name>.planfile.
		// We cannot easily set TerraformDirAbsolutePath without affecting other
		// helpers, so this test exercises the explicit-componentPath signature.
		path, cleanup, err := resolveSourcePlanFile(atmosConfig, info, dir, tf.ReusePlanAlways)
		assert.ErrorIs(t, err, errUtils.ErrReusePlanUnavailable)
		assert.Empty(t, path)
		assert.Nil(t, cleanup)
	})

	t.Run("invalid mode returns ErrReusePlanInvalidMode", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		info := &schema.ConfigAndStacksInfo{
			ContextPrefix: "prod",
			Component:     "vpc",
		}
		atmosConfig := &schema.AtmosConfiguration{}
		path, cleanup, err := resolveSourcePlanFile(atmosConfig, info, dir, tf.ReusePlanMode("bogus"))
		assert.ErrorIs(t, err, errUtils.ErrReusePlanInvalidMode)
		assert.Empty(t, path)
		assert.Nil(t, cleanup)
	})

	t.Run("auto returns canonical when binary is fresh", func(t *testing.T) {
		t.Parallel()
		atmosConfig, info, componentPath, expectedBinary := setupReusableBinary(t)

		path, cleanup, err := resolveSourcePlanFile(atmosConfig, info, componentPath, tf.ReusePlanAuto)
		require.NoError(t, err)
		assert.Equal(t, expectedBinary, path,
			"auto with fresh binary should return the canonical path")
		assert.Nil(t, cleanup,
			"reused canonical binary should not have a cleanup func (caller must not delete it)")
	})

	t.Run("always returns canonical when binary is fresh", func(t *testing.T) {
		t.Parallel()
		atmosConfig, info, componentPath, expectedBinary := setupReusableBinary(t)

		path, cleanup, err := resolveSourcePlanFile(atmosConfig, info, componentPath, tf.ReusePlanAlways)
		require.NoError(t, err)
		assert.Equal(t, expectedBinary, path)
		assert.Nil(t, cleanup)
	})
}

// setupReusableBinary builds an atmosConfig/info pair such that
// constructTerraformComponentPlanfilePath resolves to a path under the
// returned componentPath, and writes a fresh binary planfile there.
// The workdir provisioner key short-circuits the workdir resolution so the
// test does not depend on TerraformDirAbsolutePath, GetComponentPath, etc.
func setupReusableBinary(t *testing.T) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) {
	t.Helper()

	componentPath := t.TempDir()

	info := &schema.ConfigAndStacksInfo{
		ContextPrefix: "stack",
		Component:     "comp",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: componentPath,
		},
	}
	atmosConfig := &schema.AtmosConfiguration{}

	binaryPath := filepath.Join(componentPath, "stack-comp.planfile")
	writeWithMtime(t, binaryPath, []byte("plan"), time.Now())

	// Sanity check: the canonical path the resolver computes must match where
	// we wrote the binary. If this drifts, the rest of the test is meaningless.
	canonical := constructTerraformComponentPlanfilePath(atmosConfig, info)
	require.Equal(t, binaryPath, canonical,
		"test fixture must agree with constructTerraformComponentPlanfilePath")

	return atmosConfig, info, componentPath, binaryPath
}

// writeWithMtime writes a file and sets its mtime explicitly so staleness gate
// tests do not depend on filesystem timestamp precision.
func writeWithMtime(t *testing.T, path string, data []byte, mtime time.Time) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o644))
	require.NoError(t, os.Chtimes(path, mtime, mtime))
}
