package source

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendor"
)

func writeStampFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestLocalSourceStamp(t *testing.T) {
	root := t.TempDir()
	spec := &schema.VendorComponentSource{Uri: root}
	writeStampFile(t, filepath.Join(root, "main.tf"), "# one")
	base := localSourceStamp(spec)
	require.NotEmpty(t, base)
	assert.Equal(t, base, localSourceStamp(spec), "an untouched source has a stable stamp")
	assert.Len(t, base, 32, "the stamp is a hex-encoded 128-bit digest")

	t.Run("edit changes the stamp", func(t *testing.T) {
		writeStampFile(t, filepath.Join(root, "main.tf"), "# two, longer")
		edited := localSourceStamp(spec)
		assert.NotEqual(t, base, edited)
		base = edited
	})
	t.Run("same size edit with a new modification time changes the stamp", func(t *testing.T) {
		path := filepath.Join(root, "main.tf")
		before, err := os.Stat(path)
		require.NoError(t, err)
		writeStampFile(t, path, "# two, longer")
		require.NoError(t, os.Chtimes(path, time.Now(), before.ModTime().Add(time.Hour)))
		assert.NotEqual(t, base, localSourceStamp(spec))
		base = localSourceStamp(spec)
	})
	t.Run("added and removed files change the stamp", func(t *testing.T) {
		extra := filepath.Join(root, "nested", "extra.tf")
		writeStampFile(t, extra, "# extra")
		added := localSourceStamp(spec)
		assert.NotEqual(t, base, added)
		require.NoError(t, os.Remove(extra))
		assert.NotEqual(t, added, localSourceStamp(spec))
	})
	t.Run("ignored directories do not change the stamp", func(t *testing.T) {
		before := localSourceStamp(spec)
		for _, dir := range []string{".git", ".terraform", ".terragrunt-cache"} {
			writeStampFile(t, filepath.Join(root, dir, "state"), "x")
		}
		assert.Equal(t, before, localSourceStamp(spec))
	})
	t.Run("remote and missing sources have no stamp", func(t *testing.T) {
		assert.Empty(t, localSourceStamp(&schema.VendorComponentSource{Uri: "git::https://example.com/org/repo.git//module?ref=v1"}))
		assert.Empty(t, localSourceStamp(&schema.VendorComponentSource{Uri: filepath.Join(root, "absent")}))
	})
}

func TestLocalSourceStampRelativeSource(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeStampFile(t, filepath.Join(root, "modules", "unit", "main.tf"), "# one")
	relative := localSourceStamp(&schema.VendorComponentSource{Uri: "./modules/unit"})
	absolute := localSourceStamp(&schema.VendorComponentSource{Uri: filepath.Join(root, "modules", "unit")})
	require.NotEmpty(t, relative)
	assert.Equal(t, absolute, relative, "a relative and an absolute spelling name one directory")
}

func TestNeedsProvisioningLocalSourceStamp(t *testing.T) {
	sourceDir := t.TempDir()
	writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# one")
	spec := &schema.VendorComponentSource{Uri: sourceDir}
	target := t.TempDir()
	writeStampFile(t, filepath.Join(target, "main.tf"), "# one")
	require.NoError(t, writeWorkdirMetadata(target, "app", "dev", spec))

	t.Run("metadata written by an explicit pull records the stamp and is reused", func(t *testing.T) {
		metadata, err := workdir.ReadMetadata(target)
		require.NoError(t, err)
		assert.Equal(t, localSourceStamp(spec), metadata.SourceStamp)
		needs, reason := needsProvisioning(target, spec, true)
		assert.False(t, needs, reason)
	})
	t.Run("a local workdir without a stamp is refreshed once", func(t *testing.T) {
		metadata, err := workdir.ReadMetadata(target)
		require.NoError(t, err)
		metadata.SourceStamp = ""
		require.NoError(t, workdir.WriteMetadata(target, metadata))
		needs, reason := needsProvisioning(target, spec, true)
		assert.True(t, needs)
		assert.Contains(t, reason, "no recorded stamp")
	})

	recordSourceStamp(target, localSourceStamp(spec), ui.New(nil))
	t.Run("an unchanged source is reused", func(t *testing.T) {
		needs, reason := needsProvisioning(target, spec, true)
		assert.False(t, needs, reason)
	})
	t.Run("an edited source is refreshed", func(t *testing.T) {
		writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# two")
		needs, reason := needsProvisioning(target, spec, true)
		assert.True(t, needs)
		assert.Contains(t, reason, "Local source changed")
	})
	t.Run("an explicit TTL stays authoritative over edits", func(t *testing.T) {
		withTTL := &schema.VendorComponentSource{Uri: sourceDir, TTL: "1h"}
		needs, reason := needsProvisioning(target, withTTL, true)
		assert.False(t, needs, reason)
	})
	t.Run("a removed source is refreshed so the failure names it", func(t *testing.T) {
		// Keep the same spec, so URI-change detection cannot be what reports it.
		stamp := localSourceStamp(spec)
		require.NotEmpty(t, stamp)
		recordSourceStamp(target, stamp, ui.New(nil))
		needs, reason := needsProvisioning(target, spec, true)
		require.False(t, needs, reason)
		require.NoError(t, os.RemoveAll(sourceDir))
		needs, reason = needsProvisioning(target, spec, true)
		assert.True(t, needs)
		assert.Contains(t, reason, "Local source changed")
	})
}

// A relative and an absolute spelling of the same local source must not look like a source change.
func TestCheckMetadataChangesTreatsSpellingsOfOnePathAsEqual(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	metadata := &workdir.WorkdirMetadata{Source: filepath.Join(root, "modules", "unit")}
	changed, reason := checkMetadataChanges(metadata, &schema.VendorComponentSource{Uri: "./modules/unit"})
	assert.False(t, changed, reason)
	changed, _ = checkMetadataChanges(metadata, &schema.VendorComponentSource{Uri: "./modules/other"})
	assert.True(t, changed)
}

// Provisioning a local source through the workdir twice picks up an edit, copies nothing when
// nothing changed, and leaves runtime artifacts such as a saved plan in place.
func TestAutoProvisionSourceLocalEditDetection(t *testing.T) {
	sourceDir := t.TempDir()
	writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# v1")
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	newComponent := func() map[string]any {
		return map[string]any{"component": "example", "atmos_stack": "dev", "source": map[string]any{"uri": sourceDir}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}}
	}
	provision := func() string {
		component := newComponent()
		require.NoError(t, AutoProvisionSource(t.Context(), config, "terraform", component, nil, provisioner.OutputWriters{}))
		target, err := ResolveTarget(config, "terraform", "example", component)
		require.NoError(t, err)
		return target.Dir
	}

	dir := provision()
	assertContent := func(want string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, "main.tf"))
		require.NoError(t, err)
		assert.Equal(t, want, string(data))
	}
	assertContent("# v1")
	metadata, err := workdir.ReadMetadata(dir)
	require.NoError(t, err)
	require.NotEmpty(t, metadata.SourceStamp, "provisioning a local directory records its stamp")

	// Provisioning rewrites the metadata, so an unchanged timestamp proves nothing was copied.
	marker := filepath.Join(dir, "marker.txt")
	require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o644))
	provision()
	again, err := workdir.ReadMetadata(dir)
	require.NoError(t, err)
	assert.True(t, metadata.UpdatedAt.Equal(again.UpdatedAt), "an unchanged source must not be re-copied")

	// Editing the source refreshes the workdir in place: the new code appears, runtime state stays.
	writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# v2 edited")
	provision()
	assertContent("# v2 edited")
	refreshed, err := workdir.ReadMetadata(dir)
	require.NoError(t, err)
	assert.True(t, refreshed.UpdatedAt.After(metadata.UpdatedAt))
	assert.NotEqual(t, metadata.SourceStamp, refreshed.SourceStamp)
	assert.FileExists(t, marker, "a refresh must preserve files the source does not own")
}

// A source that is a directory symlink is stamped through its target, so an edit beneath the
// target refreshes the workdir.
func TestLocalSourceStampFollowsASymlinkedRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "modules", "unit")
	writeStampFile(t, filepath.Join(target, "main.tf"), "# one")
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	spec := &schema.VendorComponentSource{Uri: link}
	before := localSourceStamp(spec)
	require.NotEmpty(t, before)
	writeStampFile(t, filepath.Join(target, "nested", "extra.tf"), "# extra")
	assert.NotEqual(t, before, localSourceStamp(spec))
}

// The provisioning wrapper runs only when the source is actually copied, never when the
// workdir is already up to date, and its error is the provisioning error.
func TestAutoProvisionSourceRunsTheWrapperOnlyWhenProvisioning(t *testing.T) {
	sourceDir := t.TempDir()
	writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# v1")
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	calls := 0
	var wrapperErr error
	ctx := WithProvisionWrapper(t.Context(), func(ctx context.Context, target *Target, provision func(context.Context) error) error {
		calls++
		assert.NotEmpty(t, target.Dir)
		if wrapperErr != nil {
			return wrapperErr
		}
		return provision(ctx)
	})
	provision := func() error {
		component := map[string]any{"component": "example", "atmos_stack": "dev", "source": map[string]any{"uri": sourceDir}, "provision": map[string]any{"workdir": map[string]any{"enabled": true}}}
		return AutoProvisionSource(ctx, config, "terraform", component, nil, provisioner.OutputWriters{})
	}

	require.NoError(t, provision())
	assert.Equal(t, 1, calls, "the first fetch is wrapped")
	require.NoError(t, provision())
	assert.Equal(t, 1, calls, "an up-to-date workdir is not provisioned, so nothing is wrapped")

	writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# v2")
	require.NoError(t, provision())
	assert.Equal(t, 2, calls, "an edited local source is provisioned again")

	writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# v3")
	wrapperErr = errors.New("hook failed")
	assert.ErrorIs(t, provision(), wrapperErr)
	assert.Equal(t, 3, calls)
}

func TestProvisionWithoutWrapperRunsDirectly(t *testing.T) {
	ran := false
	require.NoError(t, provisionWithWrapper(t.Context(), nil, func(context.Context) error { ran = true; return nil }))
	assert.True(t, ran)
}

// An unstamped workdir of a local source that is gone is refreshed, so provisioning names the
// missing source instead of reusing stale configuration. A remote source is unaffected.
func TestLocalSourceChangedReportsAMissingUnstampedSource(t *testing.T) {
	missing := &schema.VendorComponentSource{Uri: filepath.Join(t.TempDir(), "absent")}
	changed, reason := localSourceChanged(&workdir.WorkdirMetadata{}, missing)
	assert.True(t, changed)
	assert.Contains(t, reason, "missing")

	remote := &schema.VendorComponentSource{Uri: "git::https://example.com/org/repo.git//module?ref=v1"}
	changed, reason = localSourceChanged(&workdir.WorkdirMetadata{}, remote)
	assert.False(t, changed, reason)

	present := t.TempDir()
	writeStampFile(t, filepath.Join(present, "main.tf"), "# one")
	changed, _ = localSourceChanged(&workdir.WorkdirMetadata{SourceStamp: localSourceStamp(&schema.VendorComponentSource{Uri: present})}, &schema.VendorComponentSource{Uri: present})
	assert.False(t, changed)
}

func TestLocalSourceStampCopiedDirectoryLinks(t *testing.T) {
	for _, dir := range []string{".terraform", ".terragrunt-cache"} {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, dir, "nested", "main.tf")
			writeStampFile(t, file, "# before")
			require.NoError(t, os.Symlink(dir, filepath.Join(root, "linked")))
			spec := &schema.VendorComponentSource{Uri: root}
			before := localSourceStamp(spec)
			require.NotEmpty(t, before)
			assert.Equal(t, before, localSourceStamp(spec))
			first := t.TempDir()
			require.NoError(t, vendor.CopyToTarget(root, first, vendor.CopyOptions{}))
			data, err := os.ReadFile(filepath.Join(first, "linked", "nested", "main.tf"))
			require.NoError(t, err)
			assert.Equal(t, "# before", string(data))
			parentInfo, err := os.Stat(filepath.Dir(file))
			require.NoError(t, err)
			writeStampFile(t, file, "# after, longer")
			// No directory metadata change can explain the changed stamp.
			require.NoError(t, os.Chtimes(filepath.Dir(file), parentInfo.ModTime(), parentInfo.ModTime()))
			assert.NotEqual(t, before, localSourceStamp(spec))
			changed, _ := localSourceChanged(&workdir.WorkdirMetadata{SourceStamp: before}, spec)
			assert.True(t, changed)
			second := t.TempDir()
			require.NoError(t, vendor.CopyToTarget(root, second, vendor.CopyOptions{}))
			data, err = os.ReadFile(filepath.Join(second, "linked", "nested", "main.tf"))
			require.NoError(t, err)
			assert.Equal(t, "# after, longer", string(data))
		})
	}
}

func TestLocalSourceStampDirectoryLinkExclusions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		target   string
		excludes []string
		includes []string
		external bool
	}{
		{name: "git target", target: ".git"},
		{name: "external target", external: true},
		{name: "excluded link", target: ".terraform", excludes: []string{"linked"}},
		{name: "excluded target", target: ".terraform", excludes: []string{".terraform"}},
		{name: "excluded descendant", target: ".terraform", excludes: []string{"**/main.tf"}},
		{name: "include filter", target: ".terraform", includes: []string{"**/*.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, tc.target)
			if tc.external {
				target = t.TempDir()
			}
			file := filepath.Join(target, "nested", "main.tf")
			writeStampFile(t, file, "# before")
			require.NoError(t, os.Symlink(target, filepath.Join(root, "linked")))
			spec := &schema.VendorComponentSource{Uri: root, ExcludedPaths: tc.excludes, IncludedPaths: tc.includes}
			before := localSourceStamp(spec)
			require.NotEmpty(t, before)
			writeStampFile(t, file, "# after, longer")
			assert.Equal(t, before, localSourceStamp(spec))
			copyDir := t.TempDir()
			require.NoError(t, vendor.CopyToTarget(root, copyDir, vendor.CopyOptions{IncludedPaths: tc.includes, ExcludedPaths: tc.excludes}))
			assert.NoFileExists(t, filepath.Join(copyDir, "linked", "nested", "main.tf"))
		})
	}
}

func TestLocalSourceStampDirectoryLinkCycles(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, ".terraform")
	file := filepath.Join(cache, "main.tf")
	writeStampFile(t, file, "# before")
	require.NoError(t, os.Symlink(cache, filepath.Join(root, "linked")))
	require.NoError(t, os.Symlink(cache, filepath.Join(root, "alias")))
	require.NoError(t, os.Symlink(root, filepath.Join(cache, "back")))
	require.NoError(t, os.Symlink("missing", filepath.Join(cache, "dangling")))
	spec := &schema.VendorComponentSource{Uri: root}
	before := localSourceStamp(spec)
	require.NotEmpty(t, before)
	assert.Equal(t, before, localSourceStamp(spec))
	writeStampFile(t, file, "# after, longer")
	assert.NotEqual(t, before, localSourceStamp(spec))
}

func replaceStampContents(t *testing.T, path string) {
	t.Helper()
	before, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("# two"), before.Mode()))
	require.NoError(t, os.Chtimes(path, before.ModTime(), before.ModTime()))
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, before.Size(), after.Size())
	require.Equal(t, before.Mode(), after.Mode())
	require.Equal(t, before.ModTime(), after.ModTime())
}

func TestLocalSourceStampEqualMetadataContents(t *testing.T) {
	for _, kind := range []string{"regular", "file link", "directory link"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "main.tf")
			if kind != "regular" {
				path = filepath.Join(root, ".terraform", "main.tf")
			}
			writeStampFile(t, path, "# one")
			switch kind {
			case "file link":
				require.NoError(t, os.Symlink(path, filepath.Join(root, "main.tf")))
			case "directory link":
				require.NoError(t, os.Symlink(filepath.Dir(path), filepath.Join(root, "module")))
			}
			spec := &schema.VendorComponentSource{Uri: root}
			before := localSourceStamp(spec)
			require.NotEmpty(t, before)
			require.Equal(t, before, localSourceStamp(spec))
			replaceStampContents(t, path)
			after := localSourceStamp(spec)
			require.NotEmpty(t, after)
			assert.NotEqual(t, before, after)
			assert.Equal(t, after, localSourceStamp(spec))
			changed, reason := localSourceChanged(&workdir.WorkdirMetadata{SourceStamp: before}, spec)
			assert.True(t, changed)
			assert.Contains(t, reason, "Local source changed")
		})
	}
}

func TestLocalSourceStampExcludedContents(t *testing.T) {
	for _, kind := range []string{"excluded file", "include filter", "runtime directory", "external link"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "main.tf")
			spec := &schema.VendorComponentSource{Uri: root}
			switch kind {
			case "excluded file":
				spec.ExcludedPaths = []string{"main.tf"}
			case "include filter":
				spec.IncludedPaths = []string{"**/*.txt"}
			case "runtime directory":
				path = filepath.Join(root, ".terraform", "main.tf")
			case "external link":
				path = filepath.Join(t.TempDir(), "main.tf")
				require.NoError(t, os.Symlink(path, filepath.Join(root, "main.tf")))
			}
			writeStampFile(t, path, "# one")
			before := localSourceStamp(spec)
			require.NotEmpty(t, before)
			replaceStampContents(t, path)
			assert.Equal(t, before, localSourceStamp(spec))
		})
	}
}

func TestStampFileContentsReadErrors(t *testing.T) {
	// A directory opens successfully but cannot be read as file contents.
	assert.Error(t, stampFileContents(io.Discard, t.TempDir()))
	assert.Error(t, stampFileContents(io.Discard, filepath.Join(t.TempDir(), "missing")))
}

func TestSourceRefreshLegacyRemovedResources(t *testing.T) {
	for _, name := range []string{"resources.tf", "exports/resources.tf"} {
		t.Run(name, func(t *testing.T) {
			src, target := t.TempDir(), t.TempDir()
			refreshWrite(t, src, "main.tf", "# updated")
			refreshWrite(t, target, "main.tf", "# original")
			refreshWrite(t, target, name, "resource \"null_resource\" \"old\" {}")
			runtime := map[string]string{
				"terraform.tfstate": "state", ".terraform/modules/example/resources.tf": "cached module",
				".terraform.lock.hcl": "provider lock", "reviewed-plan": "plan", "notes.txt": "user file",
			}
			for path, content := range runtime {
				refreshWrite(t, target, path, content)
			}
			spec := &schema.VendorComponentSource{Uri: src}
			require.NoError(t, writeWorkdirMetadata(target, "app", "dev", spec))
			metadata, err := workdir.ReadMetadata(target)
			require.NoError(t, err)
			metadata.SourceStamp = ""
			require.NoError(t, workdir.WriteMetadata(target, metadata))
			needs, _ := needsProvisioning(target, spec, true)
			require.True(t, needs)
			before, err := downloader.TreeSHA256(target)
			require.NoError(t, err)
			err = VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil))
			require.ErrorIs(t, err, errUtils.ErrSourceCopyFailed)
			assert.ErrorContains(t, err, name)
			assert.ErrorContains(t, err, "inspect this file")
			after, err := downloader.TreeSHA256(target)
			require.NoError(t, err)
			assert.Equal(t, before, after, "failure must preserve all source, runtime files and metadata")
			assert.NoFileExists(t, filepath.Join(target, sourceFilesManifest))
			// Once the caller confirms and removes the obsolete file, migration succeeds.
			require.NoError(t, os.Remove(filepath.Join(target, name)))
			require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
			assert.FileExists(t, filepath.Join(target, sourceFilesManifest))
			for path, content := range runtime {
				actual, err := os.ReadFile(filepath.Join(target, path))
				require.NoError(t, err)
				assert.Equal(t, content, string(actual))
			}
		})
	}
}

func TestSourceRefreshLegacyIncomingResourcesBecomeOwned(t *testing.T) {
	src, target := t.TempDir(), t.TempDir()
	refreshWrite(t, src, "resources.tf", "# current")
	refreshWrite(t, target, "resources.tf", "# previous")
	spec := &schema.VendorComponentSource{Uri: src}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	actual, err := os.ReadFile(filepath.Join(target, "resources.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# current", string(actual))
	require.NoError(t, os.Remove(filepath.Join(src, "resources.tf")))
	refreshWrite(t, src, "main.tf", "# remaining")
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	assert.NoFileExists(t, filepath.Join(target, "resources.tf"))
}
