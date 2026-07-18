package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSaveRedactsSourcesAndIsDeterministic(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "vendor")
	config := &schema.AtmosConfiguration{BasePath: base}
	lock := New()
	lock.Artifacts["b"] = Artifact{
		Kind:   "source",
		Target: target,
		Source: Source{Declared: "https://token@example.com/repository?signature=secret"},
		Files: []File{
			{Path: "z.txt", Type: "file", SHA256: "z"},
			{Path: "a.txt", Type: "file", SHA256: "a"},
		},
	}

	require.NoError(t, Save(config, lock))
	first, err := os.ReadFile(Path(config))
	require.NoError(t, err)
	require.NotContains(t, string(first), "token")
	require.NotContains(t, string(first), "secret")
	require.Less(t, strings.Index(string(first), "a.txt"), strings.Index(string(first), "z.txt"))
	require.Contains(t, string(first), "target: vendor")
	require.NotContains(t, string(first), filepath.ToSlash(target))

	require.NoError(t, Save(config, lock))
	second, err := os.ReadFile(Path(config))
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestInventoryAndCleanProtectsModifiedFiles(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "vendor")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("original"), 0o644))
	files, err := Inventory(target)
	require.NoError(t, err)
	require.Len(t, files, 1)

	config := &schema.AtmosConfiguration{BasePath: base}
	lock := New()
	lock.Artifacts["component"] = Artifact{Component: "component", Kind: "source", Target: target, Files: files}
	require.NoError(t, Save(config, lock))
	require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("modified"), 0o644))

	report, err := Clean(config, "", false, false)
	require.NoError(t, err)
	require.Len(t, report.Conflicts, 1)
	require.FileExists(t, filepath.Join(target, "owned.txt"))

	report, err = Clean(config, "", true, false)
	require.NoError(t, err)
	require.Empty(t, report.Conflicts)
	require.Len(t, report.Removed, 1)
	require.NoFileExists(t, filepath.Join(target, "owned.txt"))
	loaded, err := Load(config)
	require.NoError(t, err)
	require.Empty(t, loaded.Artifacts)
}

func TestCleanRetainsPathsOwnedByUnselectedArtifact(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "vendor")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "shared.txt"), []byte("shared"), 0o644))
	files, err := Inventory(target)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{BasePath: base}
	lock := New()
	lock.Artifacts["first"] = Artifact{Component: "first", Kind: "source", Target: target, Files: files, Order: 1}
	lock.Artifacts["second"] = Artifact{Component: "second", Kind: "source", Target: target, Files: files, Order: 2}
	require.NoError(t, Save(config, lock))

	report, err := Clean(config, "first", false, false)
	require.NoError(t, err)
	require.Empty(t, report.Removed)
	require.FileExists(t, filepath.Join(target, "shared.txt"))
	loaded, err := Load(config)
	require.NoError(t, err)
	require.NotContains(t, loaded.Artifacts, "first")
	require.Contains(t, loaded.Artifacts, "second")
}

func TestCleanRejectsLockTargetOutsideProject(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		target func(string, string) string
	}{
		{name: "traversal", target: func(_ string, _ string) string { return "../outside" }},
		{name: "absolute", target: func(_ string, outside string) string { return outside }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			base := filepath.Join(root, "project")
			outside := filepath.Join(root, "outside")
			require.NoError(t, os.MkdirAll(base, 0o755))
			require.NoError(t, os.MkdirAll(outside, 0o755))
			outsideFile := filepath.Join(outside, "owned.txt")
			require.NoError(t, os.WriteFile(outsideFile, []byte("do not remove"), 0o644))

			config := &schema.AtmosConfiguration{BasePath: base}
			maliciousLock := `version: 1
artifacts:
  malicious:
    component: malicious
    kind: local
    target: ` + test.target(base, outside) + `
    source: {}
    files:
      - path: owned.txt
        type: file
        mode: 420
        sha256: ignored
    order: 1
`
			require.NoError(t, os.WriteFile(Path(config), []byte(maliciousLock), 0o644))

			_, err := Clean(config, "", true, false)
			require.Error(t, err)
			require.FileExists(t, outsideFile)
		})
	}
}

func TestIsMaterializedRequiresMatchingSourceAndFiles(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "vendor")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("original"), 0o644))
	files, err := Inventory(target)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{BasePath: base}
	artifact := Artifact{Kind: "local", Target: target, Source: Source{Declared: "file:///source"}, Files: files}
	id := ArtifactID(artifact.Kind, artifact.Target)
	require.NoError(t, Replace(config, id, artifact))

	materialized, err := IsMaterialized(config, id, "file:///source", target)
	require.NoError(t, err)
	require.True(t, materialized)

	materialized, err = IsMaterialized(config, id, "file:///other", target)
	require.NoError(t, err)
	require.False(t, materialized)

	require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("changed"), 0o644))
	materialized, err = IsMaterialized(config, id, "file:///source", target)
	require.NoError(t, err)
	require.False(t, materialized)
}

func TestVendorInventoryWithPatternsRecordsOnlyCopiedFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("readme"), 0o644))
	files, err := VendorInventoryWithPatterns(root, []string{"*.tf"}, nil)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "main.tf", files[0].Path)
}
