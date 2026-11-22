package source

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

const fetchHint = "Verify source URI is accessible and credentials are valid"

// TestAutoProvisionSourceShippedBackendWithGeneration drives automatic provisioning, the path
// `terraform plan` takes, for a module that ships backend.tf.json while backend generation is on.
// It succeeds, and the module's backend file is left for Atmos to generate.
func TestAutoProvisionSourceShippedBackendWithGeneration(t *testing.T) {
	stubShippedBackendWarning(t, new([]string))
	sourceDir := t.TempDir()
	refreshWrite(t, sourceDir, "main.tf", "# module")
	refreshWrite(t, sourceDir, generatedBackendFile, `{"terraform":{"backend":{"s3":{}}}}`)
	component := map[string]any{"component": "mod", "atmos_stack": "dev", "source": map[string]any{"uri": sourceDir}}
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")
	config.Components.Terraform.AutoGenerateBackendFile = true
	require.NoError(t, AutoProvisionSource(t.Context(), config, "terraform", component, nil, provisioner.OutputWriters{}))
	target, err := ResolveTarget(config, "terraform", "mod", component)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(target.Dir, "main.tf"))
	assert.NoFileExists(t, filepath.Join(target.Dir, generatedBackendFile))
}

// TestAutoProvisionSourceLocalConflictHasNoFetchHint verifies that a source rejected for a local
// ownership conflict does not tell the user to check the source URI or credentials.
func TestAutoProvisionSourceLocalConflictHasNoFetchHint(t *testing.T) {
	sourceDir := t.TempDir()
	refreshWrite(t, sourceDir, "main.tf", "# module")
	refreshWrite(t, sourceDir, "terraform.tfstate", `{"version":4}`)
	component := map[string]any{"component": "mod", "atmos_stack": "dev", "source": map[string]any{"uri": sourceDir}}
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	config.Components.Terraform.BasePath = filepath.Join("components", "terraform")
	err := AutoProvisionSource(t.Context(), config, "terraform", component, nil, provisioner.OutputWriters{})
	require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
	assert.NotContains(t, errUtils.AllHints(err), fetchHint)
}

// TestProvisionFailureHintOnlyForFetchFailures checks the outer hint against each class of cause.
func TestProvisionFailureHintOnlyForFetchFailures(t *testing.T) {
	cases := []struct {
		name  string
		cause error
		hint  bool
	}{
		{name: "copy conflict", cause: errUtils.Build(errUtils.ErrSourceCopyFailed).WithExplanation("conflict").Err()},
		{name: "wrapped copy conflict", cause: errors.Join(errors.New("context"), errUtils.ErrSourceCopyFailed)},
		{name: "unsafe target", cause: errUtils.ErrUnsafeVendorTarget},
		{name: "invalid spec", cause: errUtils.ErrSourceInvalidSpec},
		{name: "busy target", cause: errUtils.ErrCacheLocked},
		{name: "temporary directory", cause: errUtils.ErrCreateTempDir},
		{name: "local file system", cause: &os.PathError{Op: "open", Path: "x", Err: os.ErrPermission}},
		{name: "registry resolution", cause: errors.New("GET https://registry.example/v2/: UNAUTHORIZED"), hint: true},
		{name: "access denied", cause: errUtils.ErrSourceAccessDenied, hint: true},
		{name: "not found", cause: errUtils.ErrSourceNotFound, hint: true},
		{name: "download", cause: errUtils.ErrSourceDownload, hint: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := provisionFailure(tc.cause, "Failed", map[string]string{"component": "mod"}, fetchHint)
			require.ErrorIs(t, err, errUtils.ErrSourceProvision)
			if tc.hint {
				assert.Contains(t, errUtils.AllHints(err), fetchHint)
			} else {
				assert.NotContains(t, errUtils.AllHints(err), fetchHint)
			}
		})
	}
}
