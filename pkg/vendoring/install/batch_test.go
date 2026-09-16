package install

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

func batchLocalPackage(t *testing.T, base, name, contents string) VendorPackage {
	t.Helper()
	source := filepath.Join(base, "source-"+name)
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte(contents), 0o644))
	return NewAtmosVendorPackage(&AtmosPackageParams{Name: name, URI: source, TargetPath: filepath.Join(base, "target-"+name), PkgType: PkgTypeLocal})
}

func TestInstallBatchParallelPreparationOrderedReceipts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var active, peak atomic.Int32
	started := make(chan int, 6)
	release := make(chan struct{})
	secondReady := make(chan struct{}, 1)
	firstArrived := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "1")
			return
		}
		i, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		started <- i
		if i == 0 {
			close(firstArrived)
		}
		if i == 1 {
			select {
			case <-firstArrived:
			case <-r.Context().Done():
				return
			}
		}
		if i != 1 {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = fmt.Fprint(w, i)
	}))
	defer func() { releaseOnce.Do(func() { close(release) }); server.Close() }()
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	target := filepath.Join(base, "component")
	var packages []VendorPackage
	for i := range 6 {
		packages = append(packages, NewComponentVendorPackage(&ComponentPackageParams{Name: fmt.Sprintf("mixin-%d", i), URI: fmt.Sprintf("%s/%d", server.URL, i), ComponentPath: target, PkgType: PkgTypeRemote, IsMixin: true, MixinFilename: "main.tf", Version: "1.0"}))
	}
	var report *BatchReport
	var batchErr error
	var events []batch.Event
	done := make(chan struct{})
	go func() {
		defer close(done)
		report, batchErr = InstallBatch(ctx, config, packages, InstallOptions{MaxConcurrency: 2, RefreshLock: true}, func(e batch.Event) {
			events = append(events, e)
			if e.ID == 1 && e.Phase == "Ready" {
				select {
				case secondReady <- struct{}{}:
				default:
				}
			}
		})
	}()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("two downloads did not start")
		}
	}
	select {
	case <-secondReady:
	case <-ctx.Done():
		t.Fatal("second download did not finish while the first was blocked")
	}
	require.NoDirExists(t, target, "out-of-order ready package must not materialize")
	releaseOnce.Do(func() { close(release) })
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("batch did not finish")
	}
	require.NoError(t, batchErr)
	require.Len(t, report.Results, 6)
	require.Equal(t, int32(2), peak.Load(), "preparation must overlap and obey the worker bound")
	contents, err := os.ReadFile(filepath.Join(target, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "5", string(contents), "last declaration must win despite reversed preparation")
	receipt, err := lockfile.Load(config)
	require.NoError(t, err)
	require.Len(t, receipt.Artifacts, 6)
	var completed []int
	phases := map[string]bool{}
	bytesSeen := false
	for _, e := range events {
		if e.Phase == "Ready" {
			assert.Equal(t, 0.5, e.Fraction, "prepared packages contribute before their ordered commit")
			if e.ID == 1 {
				assert.Empty(t, completed, "the second package prepares while the first is still downloading")
			}
		}
		if e.Done {
			completed = append(completed, e.ID)
			assert.Equal(t, "installed", e.Outcome)
		}
		phases[e.Phase] = true
		bytesSeen = bytesSeen || e.Bytes
	}
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, completed)
	assert.True(t, phases["Preparing"])
	assert.True(t, phases["Installing"])
	assert.True(t, bytesSeen)
	for i, pkg := range packages {
		id, err := pkg.installer.(*componentVendorInstaller).artifactID(config)
		require.NoError(t, err)
		assert.EqualValues(t, i+1, receipt.Artifacts[id].Order)
	}
}

func TestBatchDownloadProgressWeightsKnownBytes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		done, total int64
		fraction    float64
	}{
		{"unknown", 42, 0, 0},
		{"unknown negative", 42, -1, 0},
		{"empty", 0, 100, 0},
		{"partial", 42, 100, 0.21},
		{"download finished", 100, 100, 0.5},
		{"over total", 200, 100, 0.5},
		{"negative bytes", -1, 100, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []batch.Event
			b := &batchInstaller{observer: func(e batch.Event) { events = append(events, e) }}
			b.downloadProgress(7, tc.done, tc.total)
			require.Equal(t, []batch.Event{{ID: 7, Bytes: true, Downloaded: tc.done, Total: tc.total, Fraction: tc.fraction}}, events)
		})
	}
}

func TestInstallBatchLocalSourcesObserveEarlierWrites(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	first := batchLocalPackage(t, base, "first", "new content")
	second := NewComponentVendorPackage(&ComponentPackageParams{Name: "second", URI: first.Target(), ComponentPath: filepath.Join(base, "second"), PkgType: PkgTypeLocal})
	mixin := NewComponentVendorPackage(&ComponentPackageParams{Name: "mixin", URI: filepath.Join(second.Target(), "main.tf"), ComponentPath: filepath.Join(base, "third"), PkgType: PkgTypeLocal, IsMixin: true, MixinFilename: "context.tf"})
	report, err := InstallBatch(context.Background(), config, []VendorPackage{first, second, mixin}, InstallOptions{MaxConcurrency: 4, RefreshLock: true}, nil)
	require.NoError(t, err)
	require.Len(t, report.Results, 3)
	contents, err := os.ReadFile(filepath.Join(mixin.Target(), "context.tf"))
	require.NoError(t, err)
	assert.Equal(t, "new content", string(contents))
}

func TestInstallBatchRechecksUnchangedAfterOverlay(t *testing.T) {
	config, unchanged, target := newMaterializedAtmosPackage(t, "original")
	overlay := batchLocalPackage(t, config.BasePath, "overlay", "overlay content")
	overlay.installer.(*atmosVendorInstaller).targetPath = target
	report, err := InstallBatch(context.Background(), config, []VendorPackage{overlay, unchanged}, InstallOptions{MaxConcurrency: 4, LockEnforcement: LockEnforcementSilent}, nil)
	require.NoError(t, err)
	assert.Equal(t, "installed", report.Results[1].Outcome, "initially unchanged target must be repaired after preceding overlay")
	contents, err := os.ReadFile(filepath.Join(target, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# original\n", string(contents))
	report, err = InstallBatch(context.Background(), config, []VendorPackage{unchanged}, InstallOptions{}, nil)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", report.Results[0].Outcome)
}

func TestInstallBatchReinstallsAfterCleanPreservesReceipt(t *testing.T) {
	config, pkg, target := newMaterializedAtmosPackage(t, "original")
	before, err := os.ReadFile(lockfile.Path(config))
	require.NoError(t, err)
	_, err = lockfile.Clean(config, "", false, false)
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(target, "main.tf"))
	after, err := os.ReadFile(lockfile.Path(config))
	require.NoError(t, err)
	assert.Equal(t, before, after, "clean must leave the receipt untouched")

	var warnings []string
	report, err := InstallBatch(context.Background(), config, []VendorPackage{pkg}, InstallOptions{LockEnforcement: LockEnforcementWarn}, func(event batch.Event) {
		if event.Warning != "" {
			warnings = append(warnings, event.Warning)
		}
	})
	require.NoError(t, err)
	assert.Empty(t, warnings, "a cleaned installation must be restored without drift warnings")
	require.Len(t, report.Results, 1)
	assert.Equal(t, "installed", report.Results[0].Outcome)
	contents, err := os.ReadFile(filepath.Join(target, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# original\n", string(contents))
	receipt, err := lockfile.Load(config)
	require.NoError(t, err)
	drifts, err := lockfile.Verify(config, receipt)
	require.NoError(t, err)
	assert.Empty(t, drifts)
}

func TestInstallBatchFailureContinuesAndStrictPreflight(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	good := batchLocalPackage(t, base, "good", "good")
	missing := NewAtmosVendorPackage(&AtmosPackageParams{Name: "missing", URI: filepath.Join(base, "missing"), TargetPath: filepath.Join(base, "missing-target"), PkgType: PkgTypeLocal})
	report, err := InstallBatch(context.Background(), config, []VendorPackage{missing, good}, InstallOptions{MaxConcurrency: 2, RefreshLock: true}, nil)
	require.ErrorIs(t, err, ErrCopyPackage)
	require.Len(t, report.Results, 2)
	assert.Equal(t, "failed", report.Results[0].Outcome)
	assert.Equal(t, "installed", report.Results[1].Outcome)
	require.FileExists(t, filepath.Join(good.Target(), "main.tf"))
	strict := batchLocalPackage(t, base, "strict", "strict")
	report, err = InstallBatch(context.Background(), config, []VendorPackage{strict, missing}, InstallOptions{LockEnforcement: LockEnforcementStrict}, nil)
	require.ErrorIs(t, err, ErrLockDriftBlocked)
	assert.Nil(t, report)
	require.NoDirExists(t, strict.Target())
	report, err = InstallBatch(context.Background(), config, []VendorPackage{good}, InstallOptions{MaxConcurrency: -1}, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Nil(t, report)
}

func TestInstallBatchDryRunAndCanceled(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	for _, component := range []bool{false, true} {
		t.Run(fmt.Sprint(component), func(t *testing.T) {
			pkg := NewAtmosVendorPackage(&AtmosPackageParams{Name: "remote", URI: "https://example.com/component.tar.gz", TargetPath: filepath.Join(base, "target"), PkgType: PkgTypeRemote})
			if component {
				pkg = NewComponentVendorPackage(&ComponentPackageParams{Name: "remote", URI: "https://example.com/component.tar.gz", ComponentPath: filepath.Join(base, "target"), PkgType: PkgTypeRemote})
			}
			report, err := InstallBatch(context.Background(), config, []VendorPackage{pkg}, InstallOptions{DryRun: true}, nil)
			require.NoError(t, err)
			assert.Equal(t, "checked", report.Results[0].Outcome)
			require.NoDirExists(t, pkg.Target())
			require.NoFileExists(t, lockfile.Path(config))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var events []batch.Event
			report, err = InstallBatch(ctx, config, []VendorPackage{pkg}, InstallOptions{DryRun: true}, func(e batch.Event) { events = append(events, e) })
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, "canceled", report.Results[0].Outcome)
			require.Len(t, events, 2)
			assert.True(t, events[0].Reset)
			assert.Equal(t, 1, events[0].Count)
			assert.False(t, events[0].Done)
			assert.True(t, events[1].Done)
			result, err := InstallContext(ctx, config, pkg, InstallOptions{DryRun: true})
			require.NoError(t, err)
			require.ErrorIs(t, result.Err, context.Canceled)
		})
	}
	report, err := InstallBatch(context.Background(), config, nil, InstallOptions{}, nil)
	require.NoError(t, err)
	assert.Empty(t, report.Results)
}

func TestPrepareOwnsStagingAndCancellation(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	pkg := batchLocalPackage(t, base, "stage", "prepared")
	prepared, err := Prepare(context.Background(), config, pkg, nil)
	require.NoError(t, err)
	staged := prepared.dir
	require.DirExists(t, staged)
	require.NoDirExists(t, pkg.Target())
	require.NoFileExists(t, lockfile.Path(config))
	require.NoError(t, prepared.Materialize(context.Background(), config))
	require.FileExists(t, filepath.Join(pkg.Target(), "main.tf"))
	prepared.Close()
	prepared.Close()
	require.NoDirExists(t, staged)
	var empty *PreparedPackage
	empty.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled, err := Prepare(ctx, config, pkg, nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, canceled)
}

func TestDownloadProgressReportsConsumedBytes(t *testing.T) {
	stream := io.NopCloser(strings.NewReader("abcdef"))
	var progress [][2]int64
	reader := (downloadProgress{notify: func(current, total int64) { progress = append(progress, [2]int64{current, total}) }}).TrackProgress("file", 2, 8, stream)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, "abcdef", string(body))
	require.GreaterOrEqual(t, len(progress), 2)
	assert.Equal(t, [2]int64{2, 8}, progress[0])
	assert.Equal(t, [2]int64{8, 8}, progress[len(progress)-1])
	untouched := io.NopCloser(strings.NewReader("untouched"))
	assert.Equal(t, untouched, (downloadProgress{}).TrackProgress("file", 0, 0, untouched))
}

func TestInstallBatchCancelWaitingForMutationReportsOneCanceledResult(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		t.Run(fmt.Sprint(refresh), func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			pkg := batchLocalPackage(t, config.BasePath, "waiting", "content")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			locked := make(chan struct{})
			release := make(chan struct{})
			holder := make(chan error, 1)
			go func() {
				holder <- lockfile.WithMutation(context.Background(), config, func() error { close(locked); <-release; return nil })
			}()
			<-locked
			defer func() { close(release); require.NoError(t, <-holder) }()
			var events []batch.Event
			waiting := make(chan struct{}, 1)
			done := make(chan struct{})
			var report *BatchReport
			var batchErr error
			go func() {
				defer close(done)
				report, batchErr = InstallBatch(ctx, config, []VendorPackage{pkg}, InstallOptions{RefreshLock: refresh}, func(e batch.Event) {
					events = append(events, e)
					if e.Phase == "Waiting to install" {
						waiting <- struct{}{}
					}
				})
			}()
			if refresh {
				select {
				case <-waiting:
				case <-time.After(5 * time.Second):
					t.Fatal("prepared package never reached mutation lock")
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("lock wait ignored cancellation")
			}
			require.ErrorIs(t, batchErr, context.Canceled)
			require.NotNil(t, report)
			require.Len(t, report.Results, 1)
			assert.Equal(t, "waiting", report.Results[0].Name)
			assert.Equal(t, "canceled", report.Results[0].Outcome)
			finished := 0
			for _, e := range events {
				if e.Done {
					finished++
					assert.Equal(t, "canceled", e.Outcome)
				}
			}
			assert.Equal(t, 1, finished, "cancellation must not print a second completion event")
			require.NoDirExists(t, pkg.Target())
		})
	}
}

func TestPackageLabelOmitsVersionAndDestination(t *testing.T) {
	for _, target := range []string{filepath.Join(t.TempDir(), "components", "vpc"), filepath.Join("components", "vpc")} {
		pkg := NewAtmosVendorPackage(&AtmosPackageParams{Name: "vpc", Version: "1.2.0", TargetPath: target})
		assert.Equal(t, "vpc", packageLabel(pkg))
	}
}

func TestPackageLabelDistinguishesMixinOutputFilenames(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	for _, filename := range []string{"providers.tf", "context.tf"} {
		pkg := NewComponentVendorPackage(&ComponentPackageParams{Name: "mixin https://example.com/shared.tf", ComponentPath: filepath.Join(config.BasePath, "components", "vpc"), IsMixin: true, MixinFilename: filename})
		assert.Equal(t, "mixin "+filename, packageLabel(pkg))
	}
}

func TestInstallBatchPreflightEventsUseObserver(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	pkg := batchLocalPackage(t, config.BasePath, "vpc", "content")
	var phases, warnings []string
	report, err := InstallBatch(context.Background(), config, []VendorPackage{pkg}, InstallOptions{MaxConcurrency: 1}, func(event batch.Event) {
		if event.Phase != "" {
			phases = append(phases, event.Phase)
		}
		if event.Warning != "" {
			warnings = append(warnings, event.Warning)
		}
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "installed", report.Results[0].Outcome)
	require.GreaterOrEqual(t, len(phases), 2)
	assert.Equal(t, []string{"Waiting for vendoring lock", "Checking"}, phases[:2])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "Vendor lock drift detected for vpc")
}
