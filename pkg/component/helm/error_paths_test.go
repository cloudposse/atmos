package helm

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	release "helm.sh/helm/v4/pkg/release/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// renderManifest must wrap a chart-resolution failure as ErrHelmRenderFailed so
// callers can classify it, rather than leaking the raw Helm error.
func TestRenderManifest_BadChartErrors(t *testing.T) {
	_, err := renderManifest(context.Background(), &chartSpec{
		Chart:       filepath.Join(t.TempDir(), "does-not-exist"),
		ReleaseName: "x",
		Namespace:   "ns",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrHelmRenderFailed)
}

// buildValues must surface a missing values_files entry as an error instead of
// silently dropping the overlay.
func TestBuildValues_MissingValuesFileErrors(t *testing.T) {
	_, err := buildValues(&schema.AtmosConfiguration{}, map[string]any{
		"values_files": []any{filepath.Join(t.TempDir(), "missing.yaml")},
	}, "")
	require.Error(t, err)
}

// applyRelease takes the upgrade branch for an existing release; a bad chart
// reference there must fail at chart location, not panic or silently succeed.
func TestApplyRelease_UpgradeBadChartErrors(t *testing.T) {
	actx := memoryActionContext(t)
	require.NoError(t, actx.cfg.Releases.Create(release.Mock(&release.MockReleaseOptions{
		Name:      "seeded",
		Namespace: "testns",
	})))
	stubActionContext(t, actx)

	_, err := applyRelease(context.Background(), &chartSpec{
		Chart:       filepath.Join(t.TempDir(), "does-not-exist"),
		ReleaseName: "seeded",
		Namespace:   "testns",
	}, false)
	require.Error(t, err)
}
