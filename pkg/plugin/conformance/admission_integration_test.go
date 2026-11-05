package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/policy/admission"
)

func conformanceAdmissionPolicy(t *testing.T, rule string) {
	t.Helper()
	t.Cleanup(ci.SwapRegistryForTest())
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	document := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: corporate}\nspec:\n  policies:\n    plugins:\n      on: plugin\n      action: deny\n      rule: |\n        " + rule + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugins.yaml"), []byte(document), 0o600))
	restore, err := admission.BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(restore)
}

func TestAdmissionConformanceDeniesSourceBeforeReadingPackage(t *testing.T) {
	conformanceAdmissionPolicy(t, `input.registry != 'corporate.example.com'`)

	report, err := Run(t.Context(), filepath.Join(t.TempDir(), "nonexistent"), Options{})

	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Nil(t, report)
}

func TestAdmissionConformanceChecksMetadataBeforeStartingExecutable(t *testing.T) {
	for _, tt := range []struct{ name, rule string }{
		{name: "package name", rule: `input.phase == 'package' && input.name != 'corporate/approved'`},
		// The manifest uses capabilities.commands.about, whereas governance
		// consumes the registered extension ID "command".
		{name: "normalized capability", rule: `input.phase == 'package' && 'command' in input.capabilities`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(conformanceManifest), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte("version: 1\n"), 0o600))
			conformanceAdmissionPolicy(t, tt.rule)

			report, err := Run(t.Context(), directory, Options{})

			require.ErrorIs(t, err, errUtils.ErrPolicyDenied,
				"the package must be rejected before attempting to start its absent executable")
			require.Nil(t, report)
		})
	}
}

// The tested directory is input.path, relative to the project when it lies inside it.
func TestAdmissionConformanceExposesTheTestedDirectoryAsPath(t *testing.T) {
	project := t.TempDir()
	for _, tt := range []struct {
		name, directory string
		denied          bool
	}{
		{name: "below the approved root", directory: filepath.Join(project, "build", "oidc")},
		{name: "other project directory", directory: filepath.Join(project, "scratch", "oidc"), denied: true},
		{name: "outside the project", directory: filepath.Join(t.TempDir(), "oidc"), denied: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.MkdirAll(tt.directory, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(tt.directory, "plugin.yaml"), []byte(conformanceManifest), 0o600))
			conformanceAdmissionPolicy(t, `input.operation == 'test' && input.phase == 'source' && !input.path.startsWith('build/')`)

			_, err := Run(t.Context(), tt.directory, Options{Project: project})

			if tt.denied {
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
				return
			}
			require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
		})
	}
}
