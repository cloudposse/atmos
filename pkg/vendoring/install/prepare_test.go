package install

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

func TestPackageDescriptorsPreserveSourceAndMixinIdentity(t *testing.T) {
	for _, kind := range []string{"atmos", "component", "mixin"} {
		t.Run(kind, func(t *testing.T) {
			params := &ComponentPackageParams{Name: "example", URI: "source.tf", ComponentPath: "target", PkgType: PkgTypeLocal, SourceIsLocalFile: true, IsMixin: kind == "mixin"}
			if params.IsMixin {
				params.MixinFilename = "context.tf"
			}
			pkg := NewComponentVendorPackage(params)
			if kind == "atmos" {
				pkg = NewAtmosVendorPackage(&AtmosPackageParams{Name: "example", URI: "source.tf", TargetPath: "target", PkgType: PkgTypeLocal, SourceIsLocalFile: true})
			}
			assert.Equal(t, "source.tf", pkg.URI())
			assert.Equal(t, "target", pkg.Target())
			assert.Equal(t, PkgTypeLocal, pkg.PkgType())
			assert.Equal(t, kind == "mixin", pkg.IsMixin())
			assert.Equal(t, params.MixinFilename, pkg.MixinFilename())
			assert.True(t, pkg.SourceIsLocalFile())
		})
	}
	assert.False(t, (VendorPackage{}).IsMixin())
}

func TestPrepareVersionReceiptAndInvalidTarget(t *testing.T) {
	for _, kind := range []string{"atmos", "component", "mixin"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			config := &schema.AtmosConfiguration{BasePath: base}
			pkg := batchLocalPackage(t, base, "example", "content")
			original := pkg.installer.(*atmosVendorInstaller)
			original.rawVersion = ">= 1.0.0"
			original.version = "1.2.0"
			if kind != "atmos" {
				source := pkg.URI()
				if kind == "mixin" {
					source = filepath.Join(source, "main.tf")
				}
				pkg = NewComponentVendorPackage(&ComponentPackageParams{Name: "example", URI: source, ComponentPath: pkg.Target(), PkgType: PkgTypeLocal, RawVersion: ">= 1.0.0", Version: "1.2.0", IsMixin: kind == "mixin", MixinFilename: "context.tf"})
			}
			prepared, err := Prepare(context.Background(), config, pkg, nil)
			require.NoError(t, err)
			defer prepared.Close()
			require.NoError(t, os.WriteFile(pkg.Target(), []byte("target is a file"), 0o644))
			// Snapshot preflight rejects this invalid destination before copy begins.
			var pathErr *os.PathError
			require.ErrorAs(t, prepared.Materialize(context.Background(), config), &pathErr)
			assert.Equal(t, pkg.Target(), pathErr.Path)
			assert.Contains(t, pathErr.Err.Error(), "not a directory")
			unchanged, err := os.ReadFile(pkg.Target())
			require.NoError(t, err)
			assert.Equal(t, "target is a file", string(unchanged))
			require.NoFileExists(t, lockfile.Path(config), "failed copy must not record success")
			require.NoError(t, os.Remove(pkg.Target()))
			require.NoError(t, prepared.Materialize(context.Background(), config))
			lock, err := lockfile.Load(config)
			require.NoError(t, err)
			require.Len(t, lock.Artifacts, 1)
			if kind != "mixin" {
				for _, artifact := range lock.Artifacts {
					assert.Equal(t, ">= 1.0.0", artifact.Source.VersionConstraint)
					assert.Equal(t, "1.2.0", artifact.Source.ResolvedVersion)
				}
			}
		})
	}
}

func TestPrepareFailedFetchAndInvalidReceiptCleanStaging(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	t.Setenv("TMP", scratch)
	t.Setenv("TEMP", scratch)
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	for _, kind := range []string{"component missing", "mixin missing", "empty mixin", "outside project", "unknown type"} {
		t.Run(kind, func(t *testing.T) {
			pkg := NewComponentVendorPackage(&ComponentPackageParams{Name: kind, URI: filepath.Join(base, "missing"), ComponentPath: filepath.Join(base, "target"), PkgType: PkgTypeLocal})
			switch kind {
			case "mixin missing":
				pkg.installer.(*componentVendorInstaller).mixin = true
				pkg.installer.(*componentVendorInstaller).mixinFile = "main.tf"
			case "empty mixin":
				pkg.installer.(*componentVendorInstaller).mixin = true
				pkg.installer.(*componentVendorInstaller).srcURI = ""
			case "outside project":
				pkg = batchLocalPackage(t, base, "outside", "content")
				pkg.installer.(*atmosVendorInstaller).targetPath = t.TempDir()
			case "unknown type":
				pkg.installer.(*componentVendorInstaller).pType = PkgType(42)
			}
			before, err := filepath.Glob(filepath.Join(scratch, "atmos-vendor*"))
			require.NoError(t, err)
			prepared, err := Prepare(context.Background(), config, pkg, nil)
			require.Error(t, err)
			assert.Nil(t, prepared)
			after, globErr := filepath.Glob(filepath.Join(scratch, "atmos-vendor*"))
			require.NoError(t, globErr)
			assert.Equal(t, before, after, "failed preparation must remove its staging directory")
			if kind == "empty mixin" {
				require.ErrorIs(t, err, ErrMixinEmpty)
			}
			if kind == "unknown type" {
				require.ErrorIs(t, err, errUtils.ErrUnknownPackageType)
			}
		})
	}
}

func TestPrepareTempDirectoryFailure(t *testing.T) {
	scratch := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", scratch)
	t.Setenv("TMP", scratch)
	t.Setenv("TEMP", scratch)
	config := &schema.AtmosConfiguration{}
	pkg := NewAtmosVendorPackage(&AtmosPackageParams{Name: "example", URI: "source", PkgType: PkgTypeLocal})
	prepared, err := Prepare(context.Background(), config, pkg, nil)
	require.Error(t, err)
	assert.Nil(t, prepared)
	result, err := InstallContext(context.Background(), config, pkg, InstallOptions{})
	require.NoError(t, err)
	require.Error(t, result.Err)
}

func TestDryRunDetectionAndInvalidSources(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "main.tf")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0o644))
	for _, tc := range []struct {
		source string
		custom bool
	}{
		{file, false},
		{base, false},
		{"git::https://example.com/repo.git//modules/vpc", false},
		{"https://example.com/archive.tar.gz", false},
		{"missing-relative-source", true},
		{"%invalid", true},
		{"custom://example.com/repo", true},
	} {
		t.Run(tc.source, func(t *testing.T) { assert.Equal(t, tc.custom, needsCustomDetection(tc.source)) })
	}
	for _, component := range []bool{false, true} {
		pkg := NewAtmosVendorPackage(&AtmosPackageParams{Name: "broken", URI: "%invalid", PkgType: PkgTypeRemote})
		if component {
			pkg = NewComponentVendorPackage(&ComponentPackageParams{Name: "broken", URI: "%invalid", PkgType: PkgTypeRemote})
		}
		result, err := InstallContext(context.Background(), &schema.AtmosConfiguration{}, pkg, InstallOptions{DryRun: true})
		require.NoError(t, err)
		require.ErrorIs(t, result.Err, ErrDryRunDetectionFailed)
	}
}

func TestPrepareRemoteCancellationRemovesStaging(t *testing.T) {
	requested := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	base := t.TempDir()
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	t.Setenv("TMP", scratch)
	t.Setenv("TEMP", scratch)
	config := &schema.AtmosConfiguration{BasePath: base}
	pkg := NewComponentVendorPackage(&ComponentPackageParams{Name: "canceled", URI: server.URL + "/source.tf", ComponentPath: filepath.Join(base, "target"), PkgType: PkgTypeRemote, IsMixin: true, MixinFilename: "context.tf"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { prepared, err := Prepare(ctx, config, pkg, nil); prepared.Close(); done <- err }()
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop download")
	}
	require.NoDirExists(t, pkg.Target())
	staging, err := filepath.Glob(filepath.Join(scratch, "atmos-vendor*"))
	require.NoError(t, err)
	assert.Empty(t, staging)
}

func TestLocalPreparationStopsWhenCanceledBeforeCopyOrInventory(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	pkg := batchLocalPackage(t, base, "local-cancel", "contents")
	t.Run("local copy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		target := filepath.Join(t.TempDir(), "staged")
		_, _, err := fetchToTempDir(ctx, config, pkg.URI(), PkgTypeLocal, target, fetchOptions{})
		require.ErrorIs(t, err, context.Canceled)
		require.NoDirExists(t, target)
	})
	t.Run("inventory", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		prepared, err := prepareWithProgress(ctx, config, pkg, preparationProgress{phase: func(phase string) {
			if phase == "Preparing" {
				cancel()
			}
		}})
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, prepared)
		require.NoDirExists(t, pkg.Target())
		require.NoFileExists(t, lockfile.Path(config))
	})
}
