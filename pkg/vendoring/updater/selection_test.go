package updater

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/vendoring"
)

// TestUpdateSelectedComponents_PreservesPartialResultsOnError proves that a later component's
// resolution failure does not discard results already accumulated for earlier components: the
// caller (cmd/vendor's runVendorUpdate, called from vendorUpdateCmd.RunE) explicitly checks
// `report != nil` before checking the error, specifically so it can render partial progress on a
// partial failure. Returning a bare `nil, err` would silently drop that already-applied work.
func TestUpdateSelectedComponents_PreservesPartialResultsOnError(t *testing.T) {
	base := t.TempDir()
	vendorFile := filepath.Join(base, "vendor.yaml")
	require.NoError(t, os.WriteFile(vendorFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: good
      source: github.com/cloudposse/terraform-null-label
      version: 1.0.0
      targets: [components/terraform/good]
`), 0o644))

	params := &SelectionParams{
		VendorFile: vendorFile,
		RunWithProgress: func(doWork func(onProgress func(string, int, int)) (*vendoring.UpdateReport, error)) (*vendoring.UpdateReport, error) {
			// A fake progress wrapper that skips the real (network-touching) doWork and returns a
			// canned success result, so this test exercises UpdateSelectedComponents' own
			// accumulate/return-on-error orchestration in isolation.
			return &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "good"}}}, nil
		},
	}

	report, err := UpdateSelectedComponents(params, []string{"good", "missing"})

	require.Error(t, err, "the second component ('missing') is declared nowhere, so resolution must fail")
	require.NotNil(t, report, "partial results from the first component must be preserved even when a later one fails")
	require.Len(t, report.Results, 1)
	assert.Equal(t, "good", report.Results[0].Component)
}

// TestUpdateSelectedComponents_TagsMismatchErrors proves an explicit --component list combined
// with --tags that matches nothing produces an error rather than a silently empty report --
// vendoring.MatchesComponentTags lets --component and --tags compose, but this is the "you asked
// for something and got literally nothing" case that deserves the same treatment as an unmatched
// --stack/--labels selector elsewhere in this feature.
func TestUpdateSelectedComponents_TagsMismatchErrors(t *testing.T) {
	base := t.TempDir()
	vendorFile := filepath.Join(base, "vendor.yaml")
	require.NoError(t, os.WriteFile(vendorFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: good
      source: github.com/cloudposse/terraform-null-label
      version: 1.0.0
      tags: [networking]
      targets: [components/terraform/good]
`), 0o644))

	params := &SelectionParams{
		VendorFile: vendorFile,
		Tags:       []string{"compute"},
		RunWithProgress: func(doWork func(onProgress func(string, int, int)) (*vendoring.UpdateReport, error)) (*vendoring.UpdateReport, error) {
			return doWork(nil)
		},
	}

	report, err := UpdateSelectedComponents(params, []string{"good"})

	require.Error(t, err)
	assert.Nil(t, report)
}

// TestUpdateSelectedComponents_TagsMatchSucceeds proves a matching --tags filter still updates the
// named component normally (the composed-but-matching case, as opposed to the mismatch case above).
func TestUpdateSelectedComponents_TagsMatchSucceeds(t *testing.T) {
	base := t.TempDir()
	vendorFile := filepath.Join(base, "vendor.yaml")
	require.NoError(t, os.WriteFile(vendorFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: good
      source: github.com/cloudposse/terraform-null-label
      version: 1.0.0
      tags: [networking]
      targets: [components/terraform/good]
`), 0o644))

	params := &SelectionParams{
		VendorFile: vendorFile,
		Tags:       []string{"networking"},
		RunWithProgress: func(doWork func(onProgress func(string, int, int)) (*vendoring.UpdateReport, error)) (*vendoring.UpdateReport, error) {
			return &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "good"}}}, nil
		},
	}

	report, err := UpdateSelectedComponents(params, []string{"good"})

	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "good", report.Results[0].Component)
}
