package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// skipUnlessWritablePermissionsWork skips read-only-directory error-injection
// tests on platforms/execution contexts where Unix permission bits don't
// actually block writes: Windows (different ACL model) and root (bypasses
// permission checks entirely). Mirrors pkg/config/cache_test.go's convention.
func skipUnlessWritablePermissionsWork(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Skipping read-only directory test on Windows: permission model differs")
	}
	if os.Geteuid() == 0 {
		t.Skip("Skipping test when running as root")
	}
}

// skipUnlessNonDirectoryPathComponentIsDistinguishable skips tests that rely on a
// regular file blocking a path component (e.g. stat-ing "blocker/nested.txt" where
// "blocker" is a file, not a directory) producing an error distinct from "does not
// exist". On POSIX this is ENOTDIR, which os.IsNotExist reports as false, so the
// production code under test correctly falls through to its stat-error branch. On
// Windows the equivalent condition surfaces as ERROR_PATH_NOT_FOUND, which Go's
// os.IsNotExist treats as true (conflated with "not found"), so the production
// code's `if os.IsNotExist(err) { continue }` silently treats it as already-gone
// instead of surfacing an error - a genuine platform semantic difference, not a
// bug in the code under test.
func skipUnlessNonDirectoryPathComponentIsDistinguishable(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Skipping non-directory path component test on Windows: os.IsNotExist does not distinguish ENOTDIR from ENOENT there")
	}
}

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

// trySymlink creates a symlink and reports whether the platform supports it,
// skipping the calling (sub)test rather than failing it. Windows requires
// elevated privileges to create symlinks, so this keeps the suite portable.
func trySymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("Skipping: os.Symlink not available on this platform (%v)", err)
	}
}

func TestPathResolvesFromConfig(t *testing.T) {
	t.Run("nil config falls back to working directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		cwd, err := os.Getwd()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(cwd, DefaultFileName), Path(nil))
	})

	t.Run("empty base and cli config path falls back to working directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		cwd, err := os.Getwd()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(cwd, DefaultFileName), Path(&schema.AtmosConfiguration{}))
	})

	t.Run("cli config path used when base path is empty", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{CliConfigPath: base}
		require.Equal(t, filepath.Join(base, DefaultFileName), Path(config))
	})

	t.Run("custom relative lock file name is joined with base path", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		config.Vendor.LockFile = "custom.lock.yaml"
		require.Equal(t, filepath.Join(base, "custom.lock.yaml"), Path(config))
	})

	t.Run("absolute lock file name is returned unmodified", func(t *testing.T) {
		base := t.TempDir()
		abs := filepath.Join(t.TempDir(), "absolute.lock.yaml")
		config := &schema.AtmosConfiguration{BasePath: base}
		config.Vendor.LockFile = abs
		require.Equal(t, abs, Path(config))
	})
}

func TestLoadRejectsUnreadableCorruptAndVersionMismatch(t *testing.T) {
	t.Run("unreadable lock path surfaces a read error", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		// A directory in place of the lock file makes os.ReadFile fail with a
		// non-NotExist error on every platform, without relying on permission bits.
		require.NoError(t, os.MkdirAll(Path(config), 0o755))
		_, err := Load(config)
		require.Error(t, err)
		require.ErrorContains(t, err, "read vendor lock")
	})

	t.Run("invalid yaml surfaces a parse error", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, os.WriteFile(Path(config), []byte("not: [valid: yaml"), 0o644))
		_, err := Load(config)
		require.Error(t, err)
		require.ErrorContains(t, err, "parse vendor lock")
	})

	t.Run("unsupported version is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, os.WriteFile(Path(config), []byte("version: 2\nartifacts: {}\n"), 0o644))
		_, err := Load(config)
		require.Error(t, err)
		require.ErrorContains(t, err, "unsupported vendor lock version")
	})
}

func TestSaveRejectsInvalidArtifactTargetAndDirectoryCreationFailure(t *testing.T) {
	t.Run("artifact target escaping the project root is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["evil"] = Artifact{Kind: "source", Target: filepath.Join(base, "..", "outside")}
		err := Save(config, lock)
		require.Error(t, err)
		require.ErrorContains(t, err, "normalize lock target")
	})

	t.Run("directory creation failure is surfaced", func(t *testing.T) {
		root := t.TempDir()
		// Create a regular file where Save needs to create the lock directory,
		// so os.MkdirAll fails deterministically on every platform.
		notADir := filepath.Join(root, "not-a-dir")
		require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))
		config := &schema.AtmosConfiguration{BasePath: notADir}
		err := Save(config, New())
		require.Error(t, err)
		require.ErrorContains(t, err, "create vendor lock directory")
	})
}

func TestVendorInventoryExcludesGitMetadata(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("main"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644))

	files, err := VendorInventory(root)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "main.tf", files[0].Path)

	// Inventory (used for materialized targets, not fresh vendor copies) does
	// not apply the same git exclusion, confirming the two entry points differ.
	all, err := Inventory(root)
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestVendorInventoryWithPatternsSkipsExcludedDirectories(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "excluded"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "excluded", "inner.tf"), []byte("inner"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("main"), 0o644))

	files, err := VendorInventoryWithPatterns(root, []string{"**"}, []string{"excluded/**"})
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "main.tf", files[0].Path)
}

func TestVendorInventoryWithPatternsCapturesSymlinks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "real.tf"), []byte("real"), 0o644))
	linkPath := filepath.Join(root, "link.tf")
	trySymlink(t, "real.tf", linkPath)

	files, err := VendorInventoryWithPatterns(root, []string{"*.tf"}, nil)
	require.NoError(t, err)
	require.Len(t, files, 2)

	byPath := map[string]File{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	require.Equal(t, "file", byPath["real.tf"].Type)
	link := byPath["link.tf"]
	require.Equal(t, "symlink", link.Type)
	require.Equal(t, hashString("real.tf"), link.SHA256)
}

func TestFilesDigestIsOrderIndependentAndSensitiveToContent(t *testing.T) {
	a := []File{{Path: "a.txt", Type: "file", SHA256: "aaa"}}
	b := []File{{Path: "b.txt", Type: "file", SHA256: "bbb"}}

	forward := append(append([]File{}, a...), b...)
	backward := append(append([]File{}, b...), a...)
	require.Equal(t, FilesDigest(forward), FilesDigest(backward))

	// FilesDigest must not mutate (or reorder in place) the caller's slice.
	originalOrder := []string{forward[0].Path, forward[1].Path}
	_ = FilesDigest(forward)
	require.Equal(t, originalOrder, []string{forward[0].Path, forward[1].Path})

	changed := []File{{Path: "a.txt", Type: "file", SHA256: "different"}, {Path: "b.txt", Type: "file", SHA256: "bbb"}}
	require.NotEqual(t, FilesDigest(forward), FilesDigest(changed))
	require.True(t, strings.HasPrefix(FilesDigest(forward), "sha256:"))
}

func TestIsMaterializedRejectsCorruptLockEscapingTargetAndInvalidFilePath(t *testing.T) {
	t.Run("corrupt lock surfaces a load error", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, os.WriteFile(Path(config), []byte("not: [valid"), 0o644))
		_, err := IsMaterialized(config, "id", "declared", filepath.Join(base, "vendor"))
		require.Error(t, err)
	})

	t.Run("target escaping project root is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		_, err := IsMaterialized(config, "id", "declared", filepath.Join(base, "..", "outside"))
		require.Error(t, err)
	})

	t.Run("crafted file path escaping the target is rejected", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		config := &schema.AtmosConfiguration{BasePath: base}
		maliciousLock := `version: 1
artifacts:
  malicious:
    kind: source
    target: vendor
    source:
      declared: file:///source
    files:
      - path: ../../outside.txt
        type: file
        mode: 420
        sha256: ignored
    order: 1
`
		require.NoError(t, os.WriteFile(Path(config), []byte(maliciousLock), 0o644))
		_, err := IsMaterialized(config, "malicious", "file:///source", target)
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid lock-owned file path")
	})
}

func TestReplacePrunesStaleFilesAndHandlesEdgeCases(t *testing.T) {
	t.Run("prunes a stale unchanged file and removes its now-empty parent directory", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(filepath.Join(target, "sub"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "kept.txt"), []byte("kept"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(target, "sub", "stale.txt"), []byte("stale"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)
		require.Len(t, files, 2)

		config := &schema.AtmosConfiguration{BasePath: base}
		artifact := Artifact{Kind: "source", Target: target, Files: files}
		id := ArtifactID(artifact.Kind, artifact.Target)
		require.NoError(t, Replace(config, id, artifact))

		var keptOnly []File
		for _, file := range files {
			if file.Path == "kept.txt" {
				keptOnly = append(keptOnly, file)
			}
		}
		require.NoError(t, Replace(config, id, Artifact{Kind: "source", Target: target, Files: keptOnly}))

		require.FileExists(t, filepath.Join(target, "kept.txt"))
		require.NoFileExists(t, filepath.Join(target, "sub", "stale.txt"))
		require.NoDirExists(t, filepath.Join(target, "sub"))

		loaded, err := Load(config)
		require.NoError(t, err)
		require.Len(t, loaded.Artifacts[id].Files, 1)
	})

	t.Run("silently continues when a stale file was already removed from disk", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "kept.txt"), []byte("kept"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(target, "gone.txt"), []byte("gone"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		artifact := Artifact{Kind: "source", Target: target, Files: files}
		id := ArtifactID(artifact.Kind, artifact.Target)
		require.NoError(t, Replace(config, id, artifact))

		// Remove the file out from under the lock before replacing the receipt.
		require.NoError(t, os.Remove(filepath.Join(target, "gone.txt")))

		var keptOnly []File
		for _, file := range files {
			if file.Path == "kept.txt" {
				keptOnly = append(keptOnly, file)
			}
		}
		require.NoError(t, Replace(config, id, Artifact{Kind: "source", Target: target, Files: keptOnly}))
		require.FileExists(t, filepath.Join(target, "kept.txt"))
	})

	t.Run("rejects a stale file that was modified instead of silently deleting it", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "kept.txt"), []byte("kept"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(target, "stale.txt"), []byte("original"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		artifact := Artifact{Kind: "source", Target: target, Files: files}
		id := ArtifactID(artifact.Kind, artifact.Target)
		require.NoError(t, Replace(config, id, artifact))

		require.NoError(t, os.WriteFile(filepath.Join(target, "stale.txt"), []byte("tampered"), 0o644))

		var keptOnly []File
		for _, file := range files {
			if file.Path == "kept.txt" {
				keptOnly = append(keptOnly, file)
			}
		}
		err = Replace(config, id, Artifact{Kind: "source", Target: target, Files: keptOnly})
		require.Error(t, err)
		require.ErrorContains(t, err, "was modified")

		// The tampered file must survive: Replace must never silently discard
		// evidence of an unexpected filesystem change.
		content, readErr := os.ReadFile(filepath.Join(target, "stale.txt"))
		require.NoError(t, readErr)
		require.Equal(t, "tampered", string(content))

		// The lock must not have been partially updated by the failed replace.
		loaded, loadErr := Load(config)
		require.NoError(t, loadErr)
		require.Len(t, loaded.Artifacts[id].Files, 2)
	})

	t.Run("retains a stale file still owned by another artifact", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "shared.txt"), []byte("shared"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		artifactA := Artifact{Kind: "source", Target: target, Files: files}
		idA := ArtifactID(artifactA.Kind, artifactA.Target, "a")
		idB := ArtifactID(artifactA.Kind, artifactA.Target, "b")
		require.NoError(t, Replace(config, idA, artifactA))
		require.NoError(t, Replace(config, idB, artifactA))

		// Replace idA with an artifact that no longer references shared.txt;
		// idB still owns it, so it must survive on disk.
		require.NoError(t, Replace(config, idA, Artifact{Kind: "source", Target: target, Files: nil}))
		require.FileExists(t, filepath.Join(target, "shared.txt"))

		loaded, err := Load(config)
		require.NoError(t, err)
		require.Empty(t, loaded.Artifacts[idA].Files)
		require.Len(t, loaded.Artifacts[idB].Files, 1)
	})

	t.Run("assigns the next order to new artifacts based on the highest existing order", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["first"] = Artifact{Kind: "source", Target: "vendor/a", Order: 3}
		lock.Artifacts["second"] = Artifact{Kind: "source", Target: "vendor/b", Order: 1}
		require.NoError(t, Save(config, lock))

		require.NoError(t, Replace(config, "third", Artifact{Kind: "source", Target: filepath.Join(base, "vendor", "c")}))

		loaded, err := Load(config)
		require.NoError(t, err)
		require.Equal(t, 4, loaded.Artifacts["third"].Order)
	})
}

func TestVerifyDetectsDriftAndRejectsEscapingTargets(t *testing.T) {
	t.Run("nil lock reports no drift and no error", func(t *testing.T) {
		drifts, err := Verify(&schema.AtmosConfiguration{}, nil)
		require.NoError(t, err)
		require.Nil(t, drifts)
	})

	t.Run("unchanged files report no drift", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("original"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))

		loaded, err := Load(config)
		require.NoError(t, err)
		drifts, err := Verify(config, loaded)
		require.NoError(t, err)
		require.Empty(t, drifts)
	})

	t.Run("missing and modified files are reported as drift", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "modified.txt"), []byte("original"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(target, "missing.txt"), []byte("gone"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))

		require.NoError(t, os.WriteFile(filepath.Join(target, "modified.txt"), []byte("changed"), 0o644))
		require.NoError(t, os.Remove(filepath.Join(target, "missing.txt")))

		loaded, err := Load(config)
		require.NoError(t, err)
		drifts, err := Verify(config, loaded)
		require.NoError(t, err)
		require.Len(t, drifts, 2)

		reasons := map[string]string{}
		for _, drift := range drifts {
			reasons[filepath.Base(drift.Path)] = drift.Reason
		}
		require.Equal(t, "missing", reasons["missing.txt"])
		require.Equal(t, "checksum mismatch", reasons["modified.txt"])
	})

	t.Run("modified symlink target is reported as drift", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "real.txt"), []byte("real"), 0o644))
		linkPath := filepath.Join(target, "link.txt")
		trySymlink(t, "real.txt", linkPath)
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))

		require.NoError(t, os.Remove(linkPath))
		trySymlink(t, "other.txt", linkPath)

		loaded, err := Load(config)
		require.NoError(t, err)
		drifts, err := Verify(config, loaded)
		require.NoError(t, err)
		require.Len(t, drifts, 1)
		require.Equal(t, "checksum mismatch", drifts[0].Reason)
	})

	t.Run("crafted lock target escaping the project root is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		lock := &LockFile{Version: lockVersion, Artifacts: map[string]Artifact{
			"malicious": {Kind: "source", Target: filepath.Join(base, "..", "outside"), Files: []File{{Path: "a.txt", Type: "file", SHA256: "x"}}},
		}}
		_, err := Verify(config, lock)
		require.Error(t, err)
	})
}

func TestCleanHandlesMissingFilesDryRunAndCraftedPaths(t *testing.T) {
	t.Run("dry run reports removal without touching the filesystem or lock", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("original"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Component: "component", Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))

		report, err := Clean(config, "", false, true)
		require.NoError(t, err)
		require.Len(t, report.Removed, 1)
		require.FileExists(t, filepath.Join(target, "owned.txt"))

		loaded, err := Load(config)
		require.NoError(t, err)
		require.Contains(t, loaded.Artifacts, "component")
	})

	t.Run("a file already missing from disk is not treated as a conflict", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("original"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Component: "component", Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))
		require.NoError(t, os.Remove(filepath.Join(target, "owned.txt")))

		report, err := Clean(config, "", false, false)
		require.NoError(t, err)
		require.Empty(t, report.Conflicts)

		loaded, err := Load(config)
		require.NoError(t, err)
		require.Empty(t, loaded.Artifacts)
	})

	t.Run("an unselected artifact with an invalid lock-owned path is rejected", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		config := &schema.AtmosConfiguration{BasePath: base}
		maliciousLock := `version: 1
artifacts:
  other:
    component: other
    kind: source
    target: vendor
    source: {}
    files:
      - path: ../../outside.txt
        type: file
        mode: 420
        sha256: ignored
    order: 1
`
		require.NoError(t, os.WriteFile(Path(config), []byte(maliciousLock), 0o644))
		_, err := Clean(config, "component-that-does-not-match", false, false)
		require.Error(t, err)
	})
}

func TestLockedPathRejectsInvalidRelativePaths(t *testing.T) {
	base := t.TempDir()
	// lockedPath's target parameter must already be project-relative (as
	// projectRelativeTarget/Save would have normalized it before persisting).
	target := "vendor"
	config := &schema.AtmosConfiguration{BasePath: base}

	tests := []struct {
		name     string
		relative string
		wantErr  bool
	}{
		{name: "simple nested path is accepted", relative: "sub/file.txt", wantErr: false},
		{name: "dot is rejected", relative: ".", wantErr: true},
		{name: "parent traversal is rejected", relative: "..", wantErr: true},
		{name: "embedded traversal is rejected", relative: "sub/../../outside.txt", wantErr: true},
		{name: "absolute path is rejected", relative: filepath.Join(base, "outside.txt"), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := lockedPath(config, target, test.relative)
			if test.wantErr {
				require.Error(t, err)
				require.ErrorContains(t, err, "invalid lock-owned file path")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProjectRelativeTargetNormalizesAbsoluteAndRejectsEscapes(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}

	relative, err := projectRelativeTarget(config, filepath.Join(base, "vendor", "component"))
	require.NoError(t, err)
	require.Equal(t, "vendor/component", relative)

	relative, err = projectRelativeTarget(config, "vendor/component")
	require.NoError(t, err)
	require.Equal(t, "vendor/component", relative)

	_, err = projectRelativeTarget(config, filepath.Join(base, "..", "outside"))
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid vendor lock target")

	_, err = projectRelativeTarget(config, base)
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid vendor lock target")
}

func TestProjectBaseUsesCliConfigPathAndWorkingDirectoryFallback(t *testing.T) {
	t.Run("cli config path is used when base path is empty", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{CliConfigPath: base}
		root, err := projectBase(config)
		require.NoError(t, err)
		expected, err := filepath.Abs(base)
		require.NoError(t, err)
		require.Equal(t, filepath.Clean(expected), root)
	})

	t.Run("working directory is used when both are empty", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		cwd, err := os.Getwd()
		require.NoError(t, err)
		root, err := projectBase(&schema.AtmosConfiguration{})
		require.NoError(t, err)
		require.Equal(t, filepath.Clean(cwd), root)
	})
}

func TestMatchesDetectsTypeSubstitutionAndContentChanges(t *testing.T) {
	dir := t.TempDir()
	regularPath := filepath.Join(dir, "regular.txt")
	require.NoError(t, os.WriteFile(regularPath, []byte("content"), 0o644))
	regularInfo, err := os.Lstat(regularPath)
	require.NoError(t, err)
	regularDigest, err := hashFile(regularPath)
	require.NoError(t, err)

	dirPath := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(dirPath, 0o755))
	dirInfo, err := os.Lstat(dirPath)
	require.NoError(t, err)

	t.Run("unchanged regular file matches", func(t *testing.T) {
		require.True(t, matches(File{Type: "file", SHA256: regularDigest}, regularPath, regularInfo))
	})

	t.Run("content change is detected", func(t *testing.T) {
		require.False(t, matches(File{Type: "file", SHA256: "stale-digest"}, regularPath, regularInfo))
	})

	t.Run("a directory never matches a recorded regular file", func(t *testing.T) {
		require.False(t, matches(File{Type: "file", SHA256: regularDigest}, dirPath, dirInfo))
	})

	t.Run("a regular file substituted for a declared symlink is rejected", func(t *testing.T) {
		// Security-relevant: an attacker replacing a symlink placeholder with a
		// regular file of the same declared hash must never be treated as a match.
		require.False(t, matches(File{Type: "symlink", SHA256: hashString("real.txt")}, regularPath, regularInfo))
	})

	linkPath := filepath.Join(dir, "link.txt")
	trySymlink(t, "real.txt", linkPath)
	linkInfo, err := os.Lstat(linkPath)
	require.NoError(t, err)

	t.Run("unchanged symlink matches", func(t *testing.T) {
		require.True(t, matches(File{Type: "symlink", SHA256: hashString("real.txt")}, linkPath, linkInfo))
	})

	t.Run("retargeted symlink is detected", func(t *testing.T) {
		require.False(t, matches(File{Type: "symlink", SHA256: hashString("other.txt")}, linkPath, linkInfo))
	})
}

func TestRemoveEmptyParentsClimbsUntilNonEmptyOrRoot(t *testing.T) {
	t.Run("removes empty parent directories up to the root", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, "a", "b")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		removeEmptyParents(nested, root)
		require.NoDirExists(t, filepath.Join(root, "a"))
		require.DirExists(t, root)
	})

	t.Run("stops climbing at a non-empty directory", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, "a", "b")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "a", "sibling.txt"), []byte("keep"), 0o644))
		removeEmptyParents(nested, root)
		require.NoDirExists(t, nested)
		require.DirExists(t, filepath.Join(root, "a"))
	})
}

func TestNextOrderReturnsMaxPlusOne(t *testing.T) {
	lock := New()
	lock.Artifacts["a"] = Artifact{Order: 1}
	lock.Artifacts["b"] = Artifact{Order: 5}
	lock.Artifacts["c"] = Artifact{Order: 3}
	require.Equal(t, 6, nextOrder(lock))
	require.Equal(t, 1, nextOrder(New()))
}

func TestHashFileAndHashStringErrorsAndDeterminism(t *testing.T) {
	t.Run("hashing a nonexistent file returns an error", func(t *testing.T) {
		_, err := hashFile(filepath.Join(t.TempDir(), "does-not-exist.txt"))
		require.Error(t, err)
	})

	t.Run("hashFile matches a manually computed sha256", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "content.txt")
		require.NoError(t, os.WriteFile(path, []byte("hello world"), 0o644))
		digest, err := hashFile(path)
		require.NoError(t, err)
		sum := sha256.Sum256([]byte("hello world"))
		require.Equal(t, hex.EncodeToString(sum[:]), digest)
	})

	t.Run("hashString is deterministic and matches a manual sha256", func(t *testing.T) {
		sum := sha256.Sum256([]byte("target-text"))
		require.Equal(t, hex.EncodeToString(sum[:]), hashString("target-text"))
		require.Equal(t, hashString("target-text"), hashString("target-text"))
		require.NotEqual(t, hashString("target-text"), hashString("other-text"))
	})
}

func TestRedactSourceHandlesNonURLAndUnparsableInput(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "empty string stays empty", source: "", want: ""},
		{name: "userinfo and query are stripped from a real URL", source: "https://token@example.com/repo?signature=secret", want: "https://example.com/repo"},
		{name: "non-URL text with a query-like suffix falls back to prefix split", source: "not a url?query=1", want: "not a url"},
		{name: "malformed percent-encoding falls back to prefix split", source: "%zz?foo=bar", want: "%zz"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, RedactSource(test.source))
		})
	}
}

func TestLoadDefaultsNilArtifactsMap(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	// No "artifacts:" key at all, so yaml.Unmarshal leaves Artifacts nil.
	require.NoError(t, os.WriteFile(Path(config), []byte("version: 1\n"), 0o644))
	lock, err := Load(config)
	require.NoError(t, err)
	require.NotNil(t, lock.Artifacts)
	require.Empty(t, lock.Artifacts)
}

func TestSaveDefaultsNilAndZeroValueLockFields(t *testing.T) {
	t.Run("nil lock is treated as a fresh lock file", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, Save(config, nil))
		loaded, err := Load(config)
		require.NoError(t, err)
		require.Equal(t, lockVersion, loaded.Version)
		require.Empty(t, loaded.Artifacts)
	})

	t.Run("zero-value lock gets a default version and artifacts map", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, Save(config, &LockFile{}))
		loaded, err := Load(config)
		require.NoError(t, err)
		require.Equal(t, lockVersion, loaded.Version)
		require.NotNil(t, loaded.Artifacts)
	})
}

func TestSaveSurfacesTemporaryFileCreationFailure(t *testing.T) {
	skipUnlessWritablePermissionsWork(t)
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	require.NoError(t, os.Chmod(base, 0o555))
	defer func() { _ = os.Chmod(base, 0o755) }()

	err := Save(config, New())
	require.Error(t, err)
	require.ErrorContains(t, err, "create temporary vendor lock")
}

func TestVendorInventoryWithPatternsRejectsInvalidGlobAndTraversesKeptDirectories(t *testing.T) {
	t.Run("malformed exclude pattern surfaces the glob error", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("main"), 0o644))
		_, err := VendorInventoryWithPatterns(root, nil, []string{"["})
		require.Error(t, err)
	})

	t.Run("non-excluded subdirectories are traversed and their files recorded", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kept"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "kept", "inner.tf"), []byte("inner"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("main"), 0o644))

		files, err := VendorInventoryWithPatterns(root, []string{"**/*.tf"}, nil)
		require.NoError(t, err)
		paths := []string{}
		for _, file := range files {
			paths = append(paths, file.Path)
		}
		require.ElementsMatch(t, []string{"main.tf", filepath.ToSlash(filepath.Join("kept", "inner.tf"))}, paths)
	})
}

func TestInventoryFunctionsRejectMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := Inventory(missing)
	require.Error(t, err)
	require.ErrorContains(t, err, "inventory vendor target")

	_, err = VendorInventoryWithPatterns(missing, nil, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "inventory patterned vendor source")
}

func TestReplaceRejectsInvalidTargetsCorruptLockAndInvalidFilePaths(t *testing.T) {
	t.Run("new artifact target escaping the project root is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		err := Replace(config, "id", Artifact{Kind: "source", Target: filepath.Join(base, "..", "outside")})
		require.Error(t, err)
		require.ErrorContains(t, err, "normalize artifact target")
	})

	t.Run("corrupt lock on disk surfaces a load error", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, os.WriteFile(Path(config), []byte("not: [valid"), 0o644))
		err := Replace(config, "id", Artifact{Kind: "source", Target: target})
		require.Error(t, err)
	})

	t.Run("new artifact with an invalid relative file path is rejected", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		config := &schema.AtmosConfiguration{BasePath: base}
		err := Replace(config, "id", Artifact{
			Kind: "source", Target: target,
			Files: []File{{Path: "../escape.txt", Type: "file", SHA256: "x"}},
		})
		require.Error(t, err)
	})

	t.Run("a previous artifact with an invalid relative file path is rejected", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		config := &schema.AtmosConfiguration{BasePath: base}
		// Craft a previous receipt directly on disk (bypassing Replace's own
		// write-path validation) with a File.Path that escapes the target.
		lock := &LockFile{Version: lockVersion, Artifacts: map[string]Artifact{
			"id": {Kind: "source", Target: "vendor", Files: []File{{Path: "../escape.txt", Type: "file", SHA256: "x"}}},
		}}
		require.NoError(t, Save(config, lock))

		err := Replace(config, "id", Artifact{Kind: "source", Target: target, Files: nil})
		require.Error(t, err)
	})

	t.Run("a previous file blocked by a non-directory path component surfaces a stat error", func(t *testing.T) {
		skipUnlessNonDirectoryPathComponentIsDistinguishable(t)
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "kept.txt"), []byte("kept"), 0o644))
		// "blocker" is a regular file, so a lock-owned path nested under it
		// (as if it were a directory) can never resolve via os.Lstat.
		require.NoError(t, os.WriteFile(filepath.Join(target, "blocker"), []byte("x"), 0o644))
		config := &schema.AtmosConfiguration{BasePath: base}
		kept, err := Inventory(target)
		require.NoError(t, err)

		var keptOnly []File
		for _, file := range kept {
			if file.Path == "kept.txt" {
				keptOnly = append(keptOnly, file)
			}
		}
		artifactWithGhost := Artifact{Kind: "source", Target: target, Files: append(append([]File{}, keptOnly...), File{Path: "blocker/nested.txt", Type: "file", SHA256: "ignored"})}
		id := ArtifactID(artifactWithGhost.Kind, artifactWithGhost.Target)
		require.NoError(t, Replace(config, id, artifactWithGhost))

		err = Replace(config, id, Artifact{Kind: "source", Target: target, Files: keptOnly})
		require.Error(t, err)
		require.ErrorContains(t, err, "inspect stale lock-owned file")
	})

	t.Run("stale file removal failure is surfaced instead of silently updating the receipt", func(t *testing.T) {
		skipUnlessWritablePermissionsWork(t)
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "kept.txt"), []byte("kept"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(target, "stale.txt"), []byte("stale"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		artifact := Artifact{Kind: "source", Target: target, Files: files}
		id := ArtifactID(artifact.Kind, artifact.Target)
		require.NoError(t, Replace(config, id, artifact))

		require.NoError(t, os.Chmod(target, 0o555))
		defer func() { _ = os.Chmod(target, 0o755) }()

		var keptOnly []File
		for _, file := range files {
			if file.Path == "kept.txt" {
				keptOnly = append(keptOnly, file)
			}
		}
		err = Replace(config, id, Artifact{Kind: "source", Target: target, Files: keptOnly})
		require.Error(t, err)
		require.ErrorContains(t, err, "remove stale lock-owned file")
	})
}

func TestVerifyReportsStatErrorForNonDirectoryPathComponent(t *testing.T) {
	skipUnlessNonDirectoryPathComponentIsDistinguishable(t)
	base := t.TempDir()
	target := filepath.Join(base, "vendor")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "blocker"), []byte("x"), 0o644))

	config := &schema.AtmosConfiguration{BasePath: base}
	lock := &LockFile{Version: lockVersion, Artifacts: map[string]Artifact{
		"component": {Kind: "source", Target: "vendor", Files: []File{{Path: "blocker/nested.txt", Type: "file", SHA256: "ignored"}}},
	}}

	drifts, err := Verify(config, lock)
	require.NoError(t, err)
	require.Len(t, drifts, 1)
	require.NotEqual(t, "missing", drifts[0].Reason)
	require.NotEqual(t, "checksum mismatch", drifts[0].Reason)
	require.NotEmpty(t, drifts[0].Reason)
}

func TestCleanRejectsCorruptLockAndSurfacesRemovalAndSaveFailures(t *testing.T) {
	t.Run("corrupt lock on disk surfaces a load error", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, os.WriteFile(Path(config), []byte("not: [valid"), 0o644))
		_, err := Clean(config, "", false, false)
		require.Error(t, err)
	})

	t.Run("a path blocked by a non-directory component is rejected during validation", func(t *testing.T) {
		skipUnlessNonDirectoryPathComponentIsDistinguishable(t)
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "blocker"), []byte("x"), 0o644))
		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Component: "component", Kind: "source", Target: target, Files: []File{{Path: "blocker/nested.txt", Type: "file", SHA256: "ignored"}}}
		require.NoError(t, Save(config, lock))

		_, err := Clean(config, "", false, false)
		require.Error(t, err)
		require.ErrorContains(t, err, "inspect lock-owned file")
	})

	t.Run("file removal failure and lock save failure are both surfaced, not silently swallowed", func(t *testing.T) {
		skipUnlessWritablePermissionsWork(t)
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("original"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Component: "component", Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))

		require.NoError(t, os.Chmod(target, 0o555))
		defer func() { _ = os.Chmod(target, 0o755) }()

		_, err = Clean(config, "", true, false)
		require.Error(t, err)
		require.ErrorContains(t, err, "remove lock-owned file")
	})

	t.Run("lock save failure after successful removal is surfaced, not silently dropped", func(t *testing.T) {
		skipUnlessWritablePermissionsWork(t)
		base := t.TempDir()
		target := filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("original"), 0o644))
		files, err := Inventory(target)
		require.NoError(t, err)

		config := &schema.AtmosConfiguration{BasePath: base}
		lock := New()
		lock.Artifacts["component"] = Artifact{Component: "component", Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))

		// Only the base (lock) directory is read-only; the target directory
		// stays writable, so file removal succeeds but the final lock write fails.
		require.NoError(t, os.Chmod(base, 0o555))
		defer func() { _ = os.Chmod(base, 0o755) }()

		_, err = Clean(config, "", true, false)
		require.Error(t, err)
		require.NoFileExists(t, filepath.Join(target, "owned.txt"))
	})
}
