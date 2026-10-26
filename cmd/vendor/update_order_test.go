package vendor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/concurrency"
)

func TestUpdatePullPreservesInterleavedReportOrder(t *testing.T) {
	root := concurrentVendorRoot(t)
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	output := setupVendorUICapture(t)
	report := &vendoring.UpdateReport{}
	targets := map[string]string{}
	for _, typ := range []string{"helmfile", "terraform"} {
		source := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(source, "payload.txt"), []byte(typ), 0o600))
		target := writeLocalComponentManifestFixture(t, filepath.Join(root, "components", typ), "example", source)
		targets[typ] = target
		report.Results = append(report.Results, vendoring.SourceUpdateResult{Component: "example", File: filepath.Join(target, "component.yaml"), ComponentType: typ, Status: vendoring.StatusUpdated})
	}
	fallbackSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(fallbackSource, "payload.txt"), []byte("fallback overlay"), 0o600))
	vendorFile := filepath.Join(root, "vendor.yaml")
	require.NoError(t, os.WriteFile(vendorFile, []byte(fmt.Sprintf("apiVersion: atmos/v1\nkind: AtmosVendorConfig\nspec:\n  sources:\n    - component: overlay\n      source: %q\n      targets: [components/terraform/example]\n", filepath.ToSlash(fallbackSource))), 0o600))
	// The fallback is declared between two component types and shares the final
	// component's destination. Its earlier write must not overwrite the final one.
	report.Results = append(report.Results[:1], append([]vendoring.SourceUpdateResult{{Component: "overlay", File: vendorFile, ComponentType: "terraform", Status: vendoring.StatusUpToDate}}, report.Results[1:]...)...)
	command := newVendorPullTestCmd()
	command.Flags().Int(concurrency.Flag, 0, "")
	require.NoError(t, command.Flags().Set(concurrency.Flag, "3"))
	require.NoError(t, runVendorPull(command, nil, report, vendorPullParams{}))
	for typ, target := range targets {
		content, err := os.ReadFile(filepath.Join(target, "payload.txt"))
		require.NoError(t, err)
		assert.Equal(t, typ, string(content), "declaration order decides the final overlapping destination")
	}
	assert.Equal(t, 1, strings.Count(output.String(), "Vendored 3 packages"), "interleaved sources still use one installation batch")
}
