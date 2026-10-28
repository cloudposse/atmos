package source

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func refreshWrite(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func TestSourceRefreshPreservesRuntimeAndRemovesOwnedSource(t *testing.T) {
	src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	refreshWrite(t, src, "exports/main.tf", "# first")
	refreshWrite(t, src, "examples/terraform.tfstate", "source fixture, not runtime")
	refreshWrite(t, src, "exports/old.tf", "# obsolete")
	config := map[string]any{"component": "exports", "backend": map[string]any{"local": map[string]any{"path": "custom/state.data", "workspace_dir": "custom/workspaces"}}}
	spec := &schema.VendorComponentSource{Uri: src}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(config)))
	runtime := map[string]string{
		"exports/custom/state.data": "custom state bytes", "exports/custom/state.data.backup": "backup", "exports/custom/workspaces/blue/terraform.tfstate": "workspace",
		"exports/.terraform/providers/provider": "installed provider", "exports/.terraform/modules/modules.json": "module manifest", "exports/.terraform.lock.hcl": "provider lock", "exports/.dev-a.terraform.lock.hcl": "instance lock",
		"exports/reviewed-any-name": "reviewed saved plan", "exports/generated.txt": "generated content", "exports/backend.tf.json": "generated backend",
	}
	for name, contents := range runtime {
		refreshWrite(t, target, name, contents)
	}
	stateBefore, err := os.Stat(filepath.Join(target, "exports", "custom", "state.data"))
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(src, "examples", "terraform.tfstate")))
	require.NoError(t, os.Remove(filepath.Join(src, "exports", "old.tf")))
	refreshWrite(t, src, "exports/main.tf", "# second")
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(config)))
	for name, contents := range runtime {
		got, err := os.ReadFile(filepath.Join(target, name))
		require.NoError(t, err, name)
		assert.Equal(t, contents, string(got), name)
	}
	stateAfter, err := os.Stat(filepath.Join(target, "exports", "custom", "state.data"))
	require.NoError(t, err)
	assert.True(t, os.SameFile(stateBefore, stateAfter), "runtime inode must not move")
	assert.NoFileExists(t, filepath.Join(target, "examples", "terraform.tfstate"))
	assert.NoFileExists(t, filepath.Join(target, "exports", "old.tf"))
	got, err := os.ReadFile(filepath.Join(target, "exports", "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# second", string(got))
}

func TestSourceRefreshRollbackAndCancellation(t *testing.T) {
	for _, mode := range []string{"copy-failure", "publish-failure", "cancel", "active-lock"} {
		t.Run(mode, func(t *testing.T) {
			src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
			refreshWrite(t, src, "a.tf", "v1-a")
			refreshWrite(t, src, "b.tf", "v1-b")
			opts := WithRuntimePreservation(nil)
			spec := &schema.VendorComponentSource{Uri: src}
			require.NoError(t, VendorSource(t.Context(), nil, spec, target, opts))
			refreshWrite(t, target, "terraform.tfstate", `{"version":4,"lineage":"original","serial":2}`)
			before, err := downloader.TreeSHA256(target)
			require.NoError(t, err)
			refreshWrite(t, src, "a.tf", "v2-a")
			refreshWrite(t, src, "b.tf", "v2-b")
			switch mode {
			case "copy-failure":
				spec.IncludedPaths = []string{"["}
				err = VendorSource(t.Context(), nil, spec, target, opts)
			case "publish-failure", "cancel":
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				count := 0
				rename := func(from, to string) error {
					count++
					if count == 3 {
						if mode == "cancel" {
							cancel()
						} else {
							return errors.New("injected publish failure")
						}
					}
					return os.Rename(from, to)
				}
				err = publishSource(ctx, src, target, nil, rename)
			case "active-lock":
				refreshWrite(t, target, ".terraform.tfstate.lock.info", "active operation")
				err = VendorSource(t.Context(), nil, spec, target, opts)
				require.NoError(t, os.Remove(filepath.Join(target, ".terraform.tfstate.lock.info")))
			}
			require.Error(t, err)
			after, e := downloader.TreeSHA256(target)
			require.NoError(t, e)
			assert.Equal(t, before, after, "failure must preserve source, state and ownership receipt")
		})
	}
}

func TestSourceRefreshRefusesRuntimeCollisionAndSymlinkTraversal(t *testing.T) {
	for _, kind := range []string{"configured-state", "state-content", "saved-plan", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
			refreshWrite(t, src, "nested/owned.data", "source")
			config := map[string]any{}
			spec := &schema.VendorComponentSource{Uri: src}
			require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(config)))
			switch kind {
			case "configured-state":
				config["backend"] = map[string]any{"local": map[string]any{"path": "nested/owned.data"}}
				refreshWrite(t, target, "nested/owned.data", "custom state")
			case "state-content":
				refreshWrite(t, target, "nested/owned.data", `{"version":4,"lineage":"existing-state","serial":7}`)
			case "saved-plan":
				file, e := os.Create(filepath.Join(target, "nested", "owned.data"))
				require.NoError(t, e)
				writer := zip.NewWriter(file)
				entry, e := writer.Create("tfplan")
				require.NoError(t, e)
				_, e = entry.Write([]byte("reviewed plan"))
				require.NoError(t, e)
				require.NoError(t, writer.Close())
				require.NoError(t, file.Close())
			case "symlink":
				outside := t.TempDir()
				refreshWrite(t, outside, "owned.data", "outside")
				require.NoError(t, os.RemoveAll(filepath.Join(target, "nested")))
				if e := os.Symlink(outside, filepath.Join(target, "nested")); e != nil {
					t.Skipf("symlinks unavailable: %v", e)
				}
			}
			before, e := downloader.TreeSHA256(target)
			require.NoError(t, e)
			require.Error(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(config)))
			after, e := downloader.TreeSHA256(target)
			require.NoError(t, e)
			assert.Equal(t, before, after)
		})
	}
}

func TestSourceInvocationReuseFreshMapsAndChangedSources(t *testing.T) {
	src := t.TempDir()
	refreshWrite(t, src, "main.tf", "# v1")
	conf := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	component := func() map[string]any {
		return map[string]any{"component": "example", "atmos_component": "alias", "atmos_stack": "dev", "source": map[string]any{"uri": src, "ttl": "0s"}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}}
	}
	first := component()
	restore := BeginInvocation()
	t.Cleanup(restore)
	ctx := workdir.WithOutputSuppressed(t.Context())
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", first, nil, provisioner.OutputWriters{}))
	target, e := ResolveTarget(conf, "terraform", "example", first)
	require.NoError(t, e)
	refreshWrite(t, src, "main.tf", "# v2")
	second := component()
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", second, nil, provisioner.OutputWriters{}))
	got, e := os.ReadFile(filepath.Join(target.Dir, "main.tf"))
	require.NoError(t, e)
	assert.Equal(t, "# v1", string(got))
	assert.Equal(t, target.Dir, second[workdir.WorkdirPathKey])
	// A hook on the same config must not undo a resolved nested module directory.
	require.NoError(t, os.MkdirAll(filepath.Join(target.Dir, "exports"), 0o755))
	second[workdir.WorkdirPathKey] = filepath.Join(target.Dir, "exports")
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", second, nil, provisioner.OutputWriters{}))
	assert.Equal(t, filepath.Join(target.Dir, "exports"), second[workdir.WorkdirPathKey])
	// A changed selector within the same invocation must fetch; a failed attempt must retry.
	other := t.TempDir()
	refreshWrite(t, other, "main.tf", "# other")
	changed := component()
	changed["source"].(map[string]any)["uri"] = other
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", changed, nil, provisioner.OutputWriters{}))
	got, e = os.ReadFile(filepath.Join(target.Dir, "main.tf"))
	require.NoError(t, e)
	assert.Equal(t, "# other", string(got))
	// Returning to a prior selector after another source occupied the target must
	// refresh rather than reuse an earlier success for that selector.
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", first, nil, provisioner.OutputWriters{}))
	got, e = os.ReadFile(filepath.Join(target.Dir, "main.tf"))
	require.NoError(t, e)
	assert.Equal(t, "# v2", string(got))
	require.NoError(t, os.RemoveAll(target.Dir))
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", component(), nil, provisioner.OutputWriters{}))
	refreshWrite(t, src, "main.tf", "# next invocation")
	restore()
	restore = BeginInvocation()
	defer restore()
	// Even reusing the same component map cannot leak success across CLI invocations.
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", first, nil, provisioner.OutputWriters{}))
	got, e = os.ReadFile(filepath.Join(target.Dir, "main.tf"))
	require.NoError(t, e)
	assert.Equal(t, "# next invocation", string(got))
}

func TestSourceGitReceiptUsesFetchedCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
		cmd.Dir = dir
		out, e := cmd.CombinedOutput()
		require.NoError(t, e, string(out))
		return strings.TrimSpace(string(out))
	}
	repo, consumer := t.TempDir(), t.TempDir()
	git(repo, "init", "-q")
	refreshWrite(t, repo, "exports/main.tf", "# source")
	git(repo, "add", ".")
	git(repo, "commit", "-qm", "source")
	sourceCommit := git(repo, "rev-parse", "HEAD")
	git(consumer, "init", "-q")
	refreshWrite(t, consumer, "README.md", "consumer")
	git(consumer, "add", ".")
	git(consumer, "commit", "-qm", "consumer")
	var receipt downloader.ResolvedArtifact
	target := filepath.Join(consumer, "vendored")
	spec := &schema.VendorComponentSource{Uri: "git::file://" + filepath.ToSlash(repo) + "//exports", Version: sourceCommit}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil), WithArtifactReceipt(&receipt)))
	assert.Equal(t, "git", receipt.Kind)
	assert.Equal(t, sourceCommit, receipt.Identity)
	assert.NotEqual(t, git(consumer, "rev-parse", "HEAD"), receipt.Identity)
	require.NoError(t, writeWorkdirMetadata(target, "example", "dev", spec, &receipt))
	md, e := workdir.ReadMetadata(target)
	require.NoError(t, e)
	assert.Equal(t, sourceCommit, md.SourceIdentity)
}

func TestSourceInvocationConcurrentFreshMapsAndRetry(t *testing.T) {
	var requests atomic.Int64
	var fail atomic.Bool
	fail.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			requests.Add(1)
		}
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, "# fetched source")
	}))
	defer server.Close()
	restore := BeginInvocation()
	defer restore()
	conf := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	makeComponent := func() map[string]any {
		return map[string]any{"component": "example", "atmos_component": "alias", "atmos_stack": "dev", "source": map[string]any{"uri": server.URL + "/main.tf", "ttl": "0s"}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}}
	}
	ctx := workdir.WithOutputSuppressed(t.Context())
	require.Error(t, AutoProvisionSource(ctx, conf, "terraform", makeComponent(), nil, provisioner.OutputWriters{}))
	failedRequests := requests.Load()
	fail.Store(false)
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failures <- AutoProvisionSource(ctx, conf, "terraform", makeComponent(), nil, provisioner.OutputWriters{})
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	assert.Equal(t, failedRequests+1, requests.Load(), "one successful fetch across independently resolved maps; failed attempt was not cached")
	// Auth and checkout scopes remain independent, even with identical stack/component names.
	scopedAuth := &schema.AuthContext{AWS: &schema.AWSAuthContext{Region: "us-east-2"}}
	require.NoError(t, AutoProvisionSource(ctx, conf, "terraform", makeComponent(), scopedAuth, provisioner.OutputWriters{}))
	assert.Equal(t, failedRequests+2, requests.Load())
	other := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	require.NoError(t, AutoProvisionSource(ctx, other, "terraform", makeComponent(), nil, provisioner.OutputWriters{}))
	assert.Equal(t, failedRequests+3, requests.Load())
}

func TestSourceRefreshMetadataFailureLeavesPriorReceipt(t *testing.T) {
	src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	refreshWrite(t, src, "main.tf", "# old source")
	spec := &schema.VendorComponentSource{Uri: src}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	refreshWrite(t, target, ".atmos/metadata.json", `{"source_identity":"previous"}`)
	before, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	refreshWrite(t, src, "main.tf", "# new source")
	failMetadata := func(opts *vendorSourceOptions) {
		opts.metadata = func(string) error { return errors.New("injected receipt write failure") }
	}
	require.ErrorContains(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil), failMetadata), "injected receipt")
	after, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestSourceExplicitTargetOwnership(t *testing.T) {
	src := t.TempDir()
	refreshWrite(t, src, "main.tf", "# module")
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	target := filepath.Join(config.BasePath, "explicit")
	component := func(name string) map[string]any {
		return map[string]any{
			"component": "module", "atmos_component": name, "atmos_stack": "dev",
			"source":    map[string]any{"uri": src},
			"settings":  map[string]any{"working_directory": target},
			"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
		}
	}
	ctx := workdir.WithOutputSuppressed(t.Context())
	// The publication lock must enforce ownership even when both initial checks saw an empty target.
	var wait sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, name := range []string{"first", "second"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			outcomes <- AutoProvisionSource(ctx, config, "terraform", component(name), nil, provisioner.OutputWriters{})
		}()
	}
	wait.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
		}
	}
	require.Equal(t, 1, successes)
	metadata, err := workdir.ReadMetadata(target)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	require.Contains(t, []string{"first", "second"}, metadata.Component)
	loser := "first"
	if metadata.Component == loser {
		loser = "second"
	}
	before, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	require.ErrorIs(t, AutoProvisionSource(ctx, config, "terraform", component(loser), nil, provisioner.OutputWriters{}), errUtils.ErrWorkdirProvision)
	require.ErrorIs(t, Provision(ctx, &ProvisionParams{AtmosConfig: config, ComponentType: "terraform", Component: loser, Stack: "dev", ComponentConfig: component(loser), Force: true}), errUtils.ErrWorkdirProvision)
	after, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestSourceReceiptPublicationFailureRollsBackCode(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	refreshWrite(t, source, "main.tf", "# next")
	refreshWrite(t, source, ".atmos/metadata.json", `{"source_identity":"next"}`)
	refreshWrite(t, target, "main.tf", "# previous")
	refreshWrite(t, target, ".atmos/metadata.json", `{"source_identity":"previous"}`)
	before, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	rename := func(from, to string) error {
		if from == filepath.Join(source, ".atmos", "metadata.json") {
			return errors.New("receipt publication failed")
		}
		return os.Rename(from, to)
	}
	require.ErrorContains(t, publishSource(t.Context(), source, target, nil, rename), "receipt publication failed")
	after, err := downloader.TreeSHA256(target)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestSourceRefreshRejectsUnsafeOwnershipAndLegacyConflicts(t *testing.T) {
	for _, scenario := range []string{"invalid-json", "escaping-path", "reserved-manifest", "legacy-conflict", "legacy-identical", "target-symlink", "replacement-disabled", "cancelled", "blocked-parent", "nonempty-directory"} {
		t.Run(scenario, func(t *testing.T) {
			src := t.TempDir()
			parent := t.TempDir()
			target := filepath.Join(parent, "target")
			refreshWrite(t, src, "config.yaml", "new configuration")
			refreshWrite(t, target, "preserved.state", "caller-owned")
			spec := &schema.VendorComponentSource{Uri: src}
			options := []VendorSourceOption{WithRuntimePreservation(nil)}
			ctx := t.Context()
			wantSuccess := false
			switch scenario {
			case "invalid-json":
				refreshWrite(t, target, sourceFilesManifest, "not json")
			case "escaping-path":
				refreshWrite(t, target, sourceFilesManifest, `{"files":["../escape"]}`)
			case "reserved-manifest":
				refreshWrite(t, src, sourceFilesManifest, `{"files":[]}`)
			case "legacy-conflict":
				refreshWrite(t, target, "config.yaml", "unidentified existing file")
			case "legacy-identical":
				refreshWrite(t, target, "config.yaml", "new configuration")
				wantSuccess = true
			case "target-symlink":
				outside := t.TempDir()
				require.NoError(t, os.RemoveAll(target))
				if err := os.Symlink(outside, target); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "replacement-disabled":
				options = append(options, WithReplaceTarget(false))
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "blocked-parent":
				refreshWrite(t, parent, "blocker", "file")
				target = filepath.Join(parent, "blocker", "target")
			case "nonempty-directory":
				refreshWrite(t, target, "config.yaml/runtime", "owned runtime")
			}
			err := VendorSource(ctx, nil, spec, target, options...)
			if wantSuccess {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			if scenario != "target-symlink" && scenario != "blocked-parent" {
				contents, e := os.ReadFile(filepath.Join(target, "preserved.state"))
				require.NoError(t, e)
				assert.Equal(t, "caller-owned", string(contents))
			}
		})
	}
}

func TestSourceRefreshKeepsManagedProviderLock(t *testing.T) {
	src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	refreshWrite(t, src, "main.tf", "# v1")
	refreshWrite(t, src, ".terraform.lock.hcl", "source lock")
	spec := &schema.VendorComponentSource{Uri: src}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	refreshWrite(t, target, ".terraform.lock.hcl", "instance-managed lock")
	refreshWrite(t, src, "main.tf", "# v2")
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	actual, err := os.ReadFile(filepath.Join(target, ".terraform.lock.hcl"))
	require.NoError(t, err)
	assert.Equal(t, "instance-managed lock", string(actual))
}

func TestSourceRefreshRetainsBackupWhenRollbackFails(t *testing.T) {
	stage, parent := t.TempDir(), t.TempDir()
	target := filepath.Join(parent, "target")
	refreshWrite(t, stage, "main.tf", "# new")
	refreshWrite(t, target, "main.tf", "# previous")
	rename := func(from, to string) error {
		if from == filepath.Join(stage, sourceFilesManifest) {
			require.NoError(t, os.Remove(filepath.Join(target, "main.tf")))
			refreshWrite(t, target, "main.tf/blocker", "simulated concurrent filesystem failure")
			return errors.New("publication failed after source install")
		}
		return os.Rename(from, to)
	}
	err := publishSource(t.Context(), stage, target, nil, rename)
	require.ErrorContains(t, err, "rollback journal retained")
	backups, err := filepath.Glob(filepath.Join(parent, ".atmos-source-rollback-*"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	previous, err := os.ReadFile(filepath.Join(backups[0], "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# previous", string(previous))
}
