package updater

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
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

// TestUpdateSelectedComponents_TagsPropagateToCheckAndUpdateSource proves p.Tags actually reaches
// vendoring.UpdateResolved/checkAndUpdateSource's own sourceMatchesFilter call, unlike
// TestUpdateSelectedComponents_TagsMatchSucceeds above (whose RunWithProgress returns a canned
// report without ever invoking the real doWork closure, so it can't tell a genuinely-propagated
// Tags value apart from a silently-dropped one). RunWithProgress here invokes doWork for real, and
// the vendor.yaml source deliberately uses a non-Git "oci://" scheme so checkAndUpdateSource's own
// skipReason short-circuits before any network call regardless of the tag outcome -- letting this
// test observe the propagation itself, hermetically:
//   - If Tags ("compute") correctly reaches sourceMatchesFilter and is compared against the
//     source's declared "networking" tag, the mismatch filters the source out entirely (zero
//     results), triggering UpdateSelectedComponents' own "matched nothing" error.
//   - If Tags were silently dropped (never reaching sourceMatchesFilter), the source would match
//     by default, proceed past the filter, hit skipReason's "non-Git source" branch, and return a
//     StatusSkipped result -- success, no error -- the opposite outcome.
func TestUpdateSelectedComponents_TagsPropagateToCheckAndUpdateSource(t *testing.T) {
	base := t.TempDir()
	vendorFile := filepath.Join(base, "vendor.yaml")
	require.NoError(t, os.WriteFile(vendorFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: good
      source: "oci://ghcr.io/cloudposse/mock-good:{{.Version}}"
      version: v0.1.0
      tags: [networking]
      targets: [components/terraform/good]
`), 0o644))

	params := &SelectionParams{
		VendorFile: vendorFile,
		Tags:       []string{"compute"}, // deliberately mismatched against the source's "networking" tag.
		RunWithProgress: func(doWork func(onProgress func(string, int, int)) (*vendoring.UpdateReport, error)) (*vendoring.UpdateReport, error) {
			// Unlike TestUpdateSelectedComponents_TagsMatchSucceeds' canned result, invoke doWork for
			// real: this reaches the actual checkAndUpdateSource call carrying p.Tags. The source's
			// non-Git "oci://" scheme guarantees no network call happens either way.
			return doWork(nil)
		},
	}

	report, err := UpdateSelectedComponents(params, []string{"good"})

	require.Error(t, err, "a mismatched --tags value that actually reached checkAndUpdateSource must filter the only component out")
	assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
	assert.Nil(t, report)
}
