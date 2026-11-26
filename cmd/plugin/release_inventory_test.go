package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
	pluginrelease "github.com/cloudposse/atmos/pkg/plugin/release"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
	"github.com/cloudposse/atmos/pkg/ui"
)

// After a partial failure, the released packages still reach the catalog and the output,
// while the command still fails (#3763). Fail-fast, an all-failed run, and cancellation
// keep the old behavior: no catalog change and no success output.
func TestCompleteInventoryReleaseAfterPackageFailures(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	firstOnly := []catalog.ReleaseSelection{{Name: "first", Version: "1.0.0", Repository: "oci://registry.example.com/first"}}
	firstAndLast := append(slices.Clone(firstOnly), catalog.ReleaseSelection{Name: "last", Version: "3.0.0", Repository: "oci://registry.example.com/last"})
	tests := []struct {
		name string
		// Input state.
		ctx       context.Context
		runErr    error
		failFast  bool
		succeeded int
		catalog   bool
		released  []catalog.ReleaseSelection
		reconcile error
		present   error
		// Expected behavior.
		wantReconciled []catalog.ReleaseSelection
		wantReconcile  bool
		wantPresented  bool
		wantErr        []error
	}{
		{name: "all released with catalog", ctx: t.Context(), succeeded: 2, catalog: true, released: firstAndLast, wantReconciled: firstAndLast, wantReconcile: true, wantPresented: true},
		{name: "all released without catalog", ctx: t.Context(), succeeded: 2, wantPresented: true},
		{name: "partial failure reconciles and reports the released packages", ctx: t.Context(), runErr: errUtils.ErrPluginInventoryRelease, succeeded: 2, catalog: true, released: firstAndLast, wantReconciled: firstAndLast, wantReconcile: true, wantPresented: true, wantErr: []error{errUtils.ErrPluginInventoryRelease}},
		{name: "partial failure without catalog still reports", ctx: t.Context(), runErr: errUtils.ErrPluginInventoryRelease, succeeded: 1, released: firstOnly, wantPresented: true, wantErr: []error{errUtils.ErrPluginInventoryRelease}},
		{name: "fail fast skips catalog and output", ctx: t.Context(), runErr: errUtils.ErrPluginInventoryRelease, failFast: true, succeeded: 1, catalog: true, released: firstOnly, wantErr: []error{errUtils.ErrPluginInventoryRelease}},
		{name: "nothing released skips catalog and output", ctx: t.Context(), runErr: errUtils.ErrPluginInventoryRelease, catalog: true, wantErr: []error{errUtils.ErrPluginInventoryRelease}},
		{name: "cancellation skips catalog and output", ctx: cancelled, runErr: context.Canceled, succeeded: 1, catalog: true, released: firstOnly, wantErr: []error{context.Canceled}},
		{name: "catalog failure stops before output and keeps both errors", ctx: t.Context(), runErr: errUtils.ErrPluginInventoryRelease, succeeded: 1, catalog: true, released: firstOnly, reconcile: errUtils.ErrPluginCatalogConflict, wantReconciled: firstOnly, wantReconcile: true, wantErr: []error{errUtils.ErrPluginInventoryRelease, errUtils.ErrPluginCatalogConflict}},
		{name: "output failure is returned", ctx: t.Context(), succeeded: 1, present: errUtils.ErrPluginInvalidConfig, wantPresented: true, wantErr: []error{errUtils.ErrPluginInvalidConfig}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var reconciled []catalog.ReleaseSelection
			reconcileCalled, presentCalled := false, false
			err := completeInventoryRelease(test.ctx, test.runErr, &inventoryCompletion{
				failFast:  test.failFast,
				succeeded: test.succeeded,
				catalog:   test.catalog,
				released:  test.released,
				reconcile: func(released []catalog.ReleaseSelection) error {
					reconcileCalled = true
					reconciled = released
					return test.reconcile
				},
				present: func() error {
					presentCalled = true
					return test.present
				},
			})
			require.Equal(t, test.wantReconcile, reconcileCalled, "catalog reconciliation")
			require.Equal(t, test.wantReconciled, reconciled, "packages reconciled into the catalog")
			require.Equal(t, test.wantPresented, presentCalled, "release output")
			if len(test.wantErr) == 0 {
				require.NoError(t, err)
				return
			}
			for _, want := range test.wantErr {
				require.ErrorIs(t, err, want)
			}
		})
	}
}

func TestReleasedPackagesKeepsNameVersionAndRepository(t *testing.T) {
	results := []pluginrelease.InventoryRelease{
		{Entry: releaseplan.Entry{Name: "first", Version: "1.0.0", Repository: "oci://registry.example.com/first", Source: "first-source"}},
		{Entry: releaseplan.Entry{Name: "last", Version: "3.0.0", Repository: "oci://registry.example.com/last", Source: "last-source"}},
	}
	released := releasedPackages(results)
	require.Len(t, released, 2)
	require.Equal(t, catalog.ReleaseSelection{Name: "first", Version: "1.0.0", Repository: "oci://registry.example.com/first"}, released[0])
	require.Equal(t, catalog.ReleaseSelection{Name: "last", Version: "3.0.0", Repository: "oci://registry.example.com/last"}, released[1])
	require.Empty(t, releasedPackages(nil))
}

// Only package versions released in this run are reconciled. A failed package, a package added
// after planning, a newer version of a released package listed at the current head, and a
// released package whose repository changed all keep their existing catalog entry.
func TestOnlyPackagesKeepsExactlyTheReleasedVersions(t *testing.T) {
	released := []catalog.ReleaseSelection{
		{Name: "first", Version: "1.0.0", Repository: "oci://registry.example.com/first"},
		{Name: "moved", Version: "2.0.0", Repository: "oci://registry.example.com/moved"},
		{Name: "last", Version: "3.0.0", Repository: "oci://registry.example.com/last"},
	}
	selections := []catalog.ReleaseSelection{
		{Name: "first", Version: "v1.0.0", Repository: "oci://registry.example.com/first"},
		{Name: "broken", Version: "2.0.0", Repository: "oci://registry.example.com/broken"},
		{Name: "added-after-planning", Version: "1.0.0", Repository: "oci://registry.example.com/added"},
		{Name: "moved", Version: "2.0.0", Repository: "oci://registry.example.com/elsewhere"},
		{Name: "last", Version: "3.1.0", Repository: "oci://registry.example.com/last"},
		{Name: "last", Version: "3.0.0", Repository: "oci://registry.example.com/last"},
	}
	kept := onlyPackages(selections, released)
	require.Len(t, kept, 2)
	require.Equal(t, catalog.ReleaseSelection{Name: "first", Version: "v1.0.0", Repository: "oci://registry.example.com/first"}, kept[0], "versions match after normalization")
	require.Equal(t, catalog.ReleaseSelection{Name: "last", Version: "3.0.0", Repository: "oci://registry.example.com/last"}, kept[1])
	require.Empty(t, onlyPackages([]catalog.ReleaseSelection{{Name: "first", Version: "1.0.0"}}, nil))
}

func TestFailFastRequiresInventory(t *testing.T) {
	values := viper.New()
	values.Set("head", "HEAD")
	values.Set(releaseFailFastFlag, true)
	err := validateReleaseMode(values, []string{"plugins/example"})
	require.ErrorIs(t, err, errUtils.ErrPluginFlagConflict)
	require.Contains(t, err.Error(), "--fail-fast")
	values.Set(releaseInventoryFlag, "plugins/releases.yaml")
	require.NoError(t, validateReleaseMode(values, nil))
}

func TestReleaseCommandRegistersFailFast(t *testing.T) {
	flag := releaseCommand().Flags().Lookup(releaseFailFastFlag)
	require.NotNil(t, flag)
	require.Equal(t, "false", flag.DefValue)
}

//go:generate go run go.uber.org/mock/mockgen -destination=mock_streams_test.go -package=plugin github.com/cloudposse/atmos/pkg/io Streams

// captureStreams routes the data and UI channels to buffers for the duration of a test.
func captureStreams(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("ATMOS_FORCE_TTY", "false")
	t.Setenv("ATMOS_FORCE_COLOR", "false")
	t.Setenv("NO_COLOR", "1")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	previous := iolib.GetContext()
	streams := NewMockStreams(gomock.NewController(t))
	streams.EXPECT().RawOutput().Return(stdout).AnyTimes()
	streams.EXPECT().RawError().Return(stderr).AnyTimes()
	ctx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ctx)
	ui.InitFormatter(ctx)
	t.Cleanup(func() {
		data.InitWriter(previous)
		ui.InitFormatter(previous)
	})
	return stdout, stderr
}

// After a partial failure, each reported release keeps the repository of the package that
// produced it, and the failed package is not reported as published.
func TestPresentInventoryReleasesReportsOnlyReleasedPackages(t *testing.T) {
	entries := []releaseplan.Entry{{Name: "first", Repository: "oci://registry.example.com/first"}, {Name: "broken", Repository: "oci://registry.example.com/broken"}, {Name: "last", Repository: "oci://registry.example.com/last"}}
	plan := &releaseplan.Plan{}
	plan.Packages.Include = entries
	results := []pluginrelease.InventoryRelease{
		{Entry: entries[0], Release: &pluginoci.Release{Name: "acme/first", Version: "1.0.0", Digest: "sha256:first", Platforms: map[string]v1.Descriptor{}}},
		{Entry: entries[2], Release: &pluginoci.Release{Name: "acme/last", Version: "3.0.0", Digest: "sha256:last", Platforms: map[string]v1.Descriptor{}}},
	}
	values := viper.New()
	values.Set(outputFormatFlag, "json")
	stdout, _ := captureStreams(t)

	require.NoError(t, presentInventoryReleases(plan, results, values))

	var published []pluginoutput.PublishedRelease
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &published))
	require.Len(t, published, 2)
	require.Equal(t, "acme/first", published[0].Name)
	require.Equal(t, "oci://registry.example.com/first", published[0].Repository)
	require.Equal(t, "acme/last", published[1].Name)
	require.Equal(t, "oci://registry.example.com/last", published[1].Repository)
	require.NotContains(t, stdout.String(), "broken")
}

// In check mode the summary counts the packages that were checked, not every planned package.
func TestPresentInventoryReleasesCountsCheckedPackages(t *testing.T) {
	plan := &releaseplan.Plan{}
	plan.Packages.Include = []releaseplan.Entry{{Name: "first"}, {Name: "broken"}}
	values := viper.New()
	values.Set(releaseCheckFlag, true)
	_, stderr := captureStreams(t)

	require.NoError(t, presentInventoryReleases(plan, []pluginrelease.InventoryRelease{{Entry: plan.Packages.Include[0]}}, values))

	require.Contains(t, stderr.String(), "Checked 1 plugin releases.")
}
