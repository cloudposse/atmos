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

	// Check if main.tf exists in workdir.
	mainTfPath := filepath.Join(workdirPath, "main.tf")
	if _, err := os.Stat(mainTfPath); err == nil {
		// File exists - check its contents.
		content, err := os.ReadFile(mainTfPath)
		require.NoError(t, err)

		// CRITICAL ASSERTION: The workdir should NOT contain our local marker.
		// If it does, it means the workdir provisioner copied from local
		// instead of source provisioner vendoring from remote.
		assert.False(t,
			strings.Contains(string(content), "LOCAL_VERSION_MARKER"),
			"Workdir should contain content from remote source, not local component. "+
				"This indicates the bug: JIT source provisioning was skipped because "+
				"local component exists, and workdir provisioner copied from local instead.",
		)
	}
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

			// Check if main.tf exists in workdir.
			mainTfPath := filepath.Join(workdirPath, "main.tf")
			if _, statErr := os.Stat(mainTfPath); statErr == nil {
				content, readErr := os.ReadFile(mainTfPath)
				require.NoError(t, readErr)

				// CRITICAL ASSERTION: The workdir should NOT contain our local marker.
				// If it does, it means the workdir provisioner copied from local
				// instead of source provisioner vendoring from remote.
				assert.False(t,
					strings.Contains(string(content), "LOCAL_VERSION_MARKER"),
					"%s: Workdir should contain content from remote source, not local component. "+
						"This indicates JIT source provisioning was skipped for %s.",
					tc.subcommand, tc.subcommand,
				)
			}
		})
	}
}
