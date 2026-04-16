package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
)

// TestJITSource_WorkdirWithLocalComponent verifies that when:
// - source.uri is configured
// - provision.workdir.enabled: true
// - A LOCAL component already exists at components/terraform/<component>/
// The JIT source provisioning should STILL run and vendor to workdir,
// NOT skip and let workdir provisioner copy from local.
//
// This is a regression test for the bug where JIT source provisioning was
// skipped when a local component existed, even when workdir was enabled.
// The expected behavior is that source + workdir should vendor directly to
// workdir path, ignoring any existing local component.
func TestJITSource_WorkdirWithLocalComponent(t *testing.T) {
	// Use the source-provisioner-workdir fixture which has vpc-remote-workdir
	// configured with source.uri and provision.workdir.enabled: true.
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	// Create a LOCAL component at components/terraform/vpc-remote-workdir/
	// This simulates having previously vendored via vendor.yaml with a different version.
	// The component name matches the one in the stack config that has source + workdir enabled.
	localComponent := filepath.Join("components", "terraform", "vpc-remote-workdir")
	require.NoError(t, os.MkdirAll(localComponent, 0o755))

	// Create a marker file to identify content from LOCAL vs REMOTE source.
	// The local version has a distinct marker that should NOT appear in workdir
	// if source provisioning runs correctly.
	localMarker := "# LOCAL_VERSION_MARKER - This content is from local component and should NOT be used"
	require.NoError(t, os.WriteFile(
		filepath.Join(localComponent, "main.tf"),
		[]byte(localMarker+"\n\nresource \"null_resource\" \"local\" {}\n"),
		0o644,
	))

	t.Cleanup(func() {
		_ = os.RemoveAll(localComponent)
		_ = os.RemoveAll(".workdir")
	})

	// Run terraform plan with --dry-run to trigger the provisioning flow
	// without actually running terraform. The dry-run flag skips terraform
	// execution but still runs through the JIT provisioning logic.
	cmd.RootCmd.SetArgs([]string{
		"terraform", "plan", "vpc-remote-workdir",
		"--stack", "dev",
		"--dry-run",
	})

	// Execute - this may error due to terraform not being configured,
	// but the provisioning should have been attempted.
	_ = cmd.Execute()

	// Check the workdir path where the component should have been provisioned.
	// With source + workdir enabled, the workdir should contain files from REMOTE source,
	// NOT from the local component we created above.
	workdirPath := filepath.Join(".workdir", "terraform", "dev-vpc-remote-workdir")

	// Verify workdir exists.
	info, err := os.Stat(workdirPath)
	require.NoError(t, err, "Workdir should exist at %s", workdirPath)
	require.True(t, info.IsDir(), "Workdir path should be a directory")

	// The remote source (terraform-null-label//exports) contains context.tf, not main.tf.
	// If JIT source provisioning ran correctly, context.tf MUST exist.
	contextTfPath := filepath.Join(workdirPath, "context.tf")
	_, err = os.Stat(contextTfPath)
	require.NoError(t, err, "context.tf should exist in workdir (from remote source)")

	// CRITICAL ASSERTION: The local main.tf with our marker should NOT exist.
	// The remote source doesn't have main.tf, so if it exists, the workdir
	// provisioner incorrectly copied from local instead of using remote source.
	mainTfPath := filepath.Join(workdirPath, "main.tf")
	if _, statErr := os.Stat(mainTfPath); statErr == nil {
		// main.tf exists when it shouldn't - read it to provide better diagnostics.
		content, readErr := os.ReadFile(mainTfPath)
		require.NoError(t, readErr, "Failed to read main.tf for diagnostics")
		assert.False(t, strings.Contains(string(content), "LOCAL_VERSION_MARKER"),
			"main.tf exists and contains LOCAL_VERSION_MARKER. "+
				"This indicates JIT source provisioning was skipped because "+
				"local component exists, and workdir provisioner copied from local instead.",
		)
		t.Fatalf("main.tf should NOT exist in workdir (remote source doesn't have it). " +
			"Its presence indicates the bug: JIT source provisioning was skipped.")
	}
	// main.tf doesn't exist - this is the expected behavior.
}

// TestJITSource_WorkdirWithLocalComponent_SourcePrecedence is an alternative test
// that checks the provisioner output messages to verify source provisioning runs.
func TestJITSource_WorkdirWithLocalComponent_SourcePrecedence(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	// Create a LOCAL component to trigger the bug scenario.
	localComponent := filepath.Join("components", "terraform", "vpc-remote-workdir")
	require.NoError(t, os.MkdirAll(localComponent, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(localComponent, "main.tf"),
		[]byte("# LOCAL VERSION\n"),
		0o644,
	))

	t.Cleanup(func() {
		_ = os.RemoveAll(localComponent)
		_ = os.RemoveAll(".workdir")
	})

	// Verify that source describe still works and recognizes the source config.
	// This confirms the component HAS source configured.
	cmd.RootCmd.SetArgs([]string{
		"terraform", "source", "describe", "vpc-remote-workdir",
		"--stack", "dev",
	})

	err := cmd.Execute()
	require.NoError(t, err, "Source describe should work - component has source configured")

	// The test is a placeholder for verifying source provisioning takes precedence.
	// The actual verification is in the previous test which checks workdir contents.
}

// TestJITSource_WorkdirWithLocalComponent_AllSubcommands verifies that JIT source
// provisioning takes precedence over local components for all terraform subcommands.
//
// When source.uri + provision.workdir.enabled are configured, the JIT source provisioner
// should vendor from remote to workdir, even if a local component exists. This test
// confirms the fix in ExecuteTerraform() works universally for all terraform commands
// that operate on a component with a stack.
func TestJITSource_WorkdirWithLocalComponent_AllSubcommands(t *testing.T) {
	subcommands := []struct {
		name       string
		subcommand string
	}{
		// Core execution commands.
		{"apply", "apply"},
		{"deploy", "deploy"},
		{"destroy", "destroy"},
		{"init", "init"},
		{"workspace", "workspace"},

		// State and resource commands.
		{"console", "console"},
		{"force-unlock", "force-unlock"},
		{"get", "get"},
		{"graph", "graph"},
		{"import", "import"},
		{"output", "output"},
		{"refresh", "refresh"},
		{"show", "show"},
		{"state", "state"},
		{"taint", "taint"},
		{"untaint", "untaint"},

		// Validation and info commands.
		{"metadata", "metadata"},
		{"modules", "modules"},
		{"providers", "providers"},
		{"test", "test"},
		{"validate", "validate"},
	}

	for _, tc := range subcommands {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

			// Create a LOCAL component at components/terraform/vpc-remote-workdir/.
			// This simulates having previously vendored via vendor.yaml with a different version.
			localComponent := filepath.Join("components", "terraform", "vpc-remote-workdir")
			require.NoError(t, os.MkdirAll(localComponent, 0o755))

			// Create a marker file to identify content from LOCAL vs REMOTE source.
			localMarker := "# LOCAL_VERSION_MARKER - This content is from local component and should NOT be used"
			require.NoError(t, os.WriteFile(
				filepath.Join(localComponent, "main.tf"),
				[]byte(localMarker+"\n\nresource \"null_resource\" \"local\" {}\n"),
				0o644,
			))

			t.Cleanup(func() {
				_ = os.RemoveAll(localComponent)
				_ = os.RemoveAll(".workdir")
			})

			// Run terraform <subcommand> with --dry-run to trigger the provisioning flow
			// without actually running terraform.
			cmd.RootCmd.SetArgs([]string{
				"terraform", tc.subcommand, "vpc-remote-workdir",
				"--stack", "dev",
				"--dry-run",
			})

			// Execute - this may error due to terraform not being configured,
			// but the provisioning should have been attempted.
			_ = cmd.Execute()

			// Check the workdir path where the component should have been provisioned.
			workdirPath := filepath.Join(".workdir", "terraform", "dev-vpc-remote-workdir")

			// Verify workdir exists.
			info, err := os.Stat(workdirPath)
			require.NoError(t, err, "Workdir should exist at %s", workdirPath)
			require.True(t, info.IsDir(), "Workdir path should be a directory")

			// The remote source (terraform-null-label//exports) contains context.tf, not main.tf.
			// If JIT source provisioning ran correctly, context.tf MUST exist.
			contextTfPath := filepath.Join(workdirPath, "context.tf")
			_, err = os.Stat(contextTfPath)
			require.NoError(t, err, "%s: context.tf should exist in workdir (from remote source)", tc.subcommand)

			// CRITICAL ASSERTION: The local main.tf with our marker should NOT exist.
			// The remote source doesn't have main.tf, so if it exists, the workdir
			// provisioner incorrectly copied from local instead of using remote source.
			mainTfPath := filepath.Join(workdirPath, "main.tf")
			if _, statErr := os.Stat(mainTfPath); statErr == nil {
				// main.tf exists when it shouldn't - read it to provide better diagnostics.
				content, readErr := os.ReadFile(mainTfPath)
				require.NoError(t, readErr, "%s: Failed to read main.tf for diagnostics", tc.subcommand)
				assert.False(t, strings.Contains(string(content), "LOCAL_VERSION_MARKER"),
					"%s: main.tf exists and contains LOCAL_VERSION_MARKER. "+
						"This indicates JIT source provisioning was skipped.",
					tc.subcommand,
				)
				t.Fatalf("%s: main.tf should NOT exist in workdir (remote source doesn't have it). "+
					"Its presence indicates the bug: JIT source provisioning was skipped.",
					tc.subcommand)
			}
			// main.tf doesn't exist - this is the expected behavior.
		})
	}
}

// TestJITSource_GenerateVarfile verifies that `terraform generate varfile` works
// with JIT-sourced components. This is a regression test for issue #2019.
func TestJITSource_GenerateVarfile(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")
	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	// vpc-remote-workdir has source.uri configured with workdir enabled.
	cmd.RootCmd.SetArgs([]string{
		"terraform", "generate", "varfile", "vpc-remote-workdir",
		"--stack", "dev",
	})

	err := cmd.Execute()
	// Currently fails because generate varfile doesn't run JIT provisioning.
	require.NoError(t, err, "generate varfile should work with JIT-sourced components")

	// Verify workdir was provisioned.
	workdirPath := filepath.Join(".workdir", "terraform", "dev-vpc-remote-workdir")
	info, err := os.Stat(workdirPath)
	require.NoError(t, err, "Workdir should exist at %s", workdirPath)
	require.True(t, info.IsDir(), "Workdir path should be a directory")

	// Verify context.tf exists (from remote source).
	contextTfPath := filepath.Join(workdirPath, "context.tf")
	_, err = os.Stat(contextTfPath)
	require.NoError(t, err, "context.tf should exist in workdir (from remote source)")
}

// TestJITSource_GenerateBackend verifies that `terraform generate backend` works
// with JIT-sourced components. This is a regression test for issue #2019.
// Note: The fixture doesn't have backend configuration, so the command will fail
// with "backend_type is missing". The key assertion is that JIT provisioning
// works (workdir is created) before the backend validation fails.
func TestJITSource_GenerateBackend(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")
	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	cmd.RootCmd.SetArgs([]string{
		"terraform", "generate", "backend", "vpc-remote-workdir",
		"--stack", "dev",
	})

	err := cmd.Execute()
	// The fixture doesn't have backend config, so this will fail with "backend_type is missing".
	// The key test is that JIT provisioning happened (workdir exists) before this error.
	// If JIT didn't work, the error would be "component does not exist".
	if err != nil {
		require.Contains(t, err.Error(), "backend_type",
			"error should be about missing backend_type, not missing component")
	}

	// Verify workdir was provisioned by JIT source.
	workdirPath := filepath.Join(".workdir", "terraform", "dev-vpc-remote-workdir")
	info, statErr := os.Stat(workdirPath)
	require.NoError(t, statErr, "Workdir should exist at %s (JIT provisioning should have run)", workdirPath)
	require.True(t, info.IsDir(), "Workdir path should be a directory")

	// Verify context.tf exists (from remote source).
	contextTfPath := filepath.Join(workdirPath, "context.tf")
	_, statErr = os.Stat(contextTfPath)
	require.NoError(t, statErr, "context.tf should exist in workdir (from remote source)")
}

// TestJITSource_PackerOutput verifies that `packer output` works
// with JIT-sourced components.
func TestJITSource_PackerOutput(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")
	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	// ami-workdir has source.uri configured with workdir enabled.
	cmd.RootCmd.SetArgs([]string{
		"packer", "output", "ami-workdir",
		"--stack", "dev",
	})

	err := cmd.Execute()
	// Note: This will fail even after JIT fix if no manifest exists,
	// but the error should be "manifest not found" not "component not found".
	if err != nil {
		// Accept "manifest not found" as success - JIT provisioning worked.
		require.Contains(t, err.Error(), "manifest",
			"error should be about missing manifest, not missing component")
	}
}

// TestJITSource_HelmfileGenerateVarfile verifies that `helmfile generate varfile`
// works with JIT-sourced components.
func TestJITSource_HelmfileGenerateVarfile(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")
	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	// nginx-workdir has source.uri configured with workdir enabled.
	cmd.RootCmd.SetArgs([]string{
		"helmfile", "generate", "varfile", "nginx-workdir",
		"--stack", "dev",
	})

	err := cmd.Execute()
	require.NoError(t, err, "helmfile generate varfile should work with JIT-sourced components")
}
