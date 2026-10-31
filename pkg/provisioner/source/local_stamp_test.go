package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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

	t.Run("a workdir without a stamp is reused", func(t *testing.T) {
		needs, reason := needsProvisioning(target, spec, true)
		assert.False(t, needs, reason)
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
		missing := &schema.VendorComponentSource{Uri: filepath.Join(sourceDir, "gone")}
		needs, _ := needsProvisioning(target, missing, true)
		assert.True(t, needs)
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

// The provisioning wrapper runs only when the source is actually copied, never when the
// workdir is already up to date, and its error is the provisioning error.
func TestAutoProvisionSourceRunsTheWrapperOnlyWhenProvisioning(t *testing.T) {
	sourceDir := t.TempDir()
	writeStampFile(t, filepath.Join(sourceDir, "main.tf"), "# v1")
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	calls := 0
	var wrapperErr error
	ctx := WithProvisionWrapper(t.Context(), func(ctx context.Context, provision func(context.Context) error) error {
		calls++
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
	require.NoError(t, provisionWithWrapper(t.Context(), func(context.Context) error { ran = true; return nil }))
	assert.True(t, ran)
}
