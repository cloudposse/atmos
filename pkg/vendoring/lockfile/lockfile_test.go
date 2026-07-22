package lockfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mustArtifactID wraps ArtifactID for tests, using the same kind/target/writers shape throughout
// this file and failing the test immediately on error instead of threading one through every call
// site individually.
func mustArtifactID(t *testing.T, config *schema.AtmosConfiguration, kind, target string, writers ...string) string {
	t.Helper()
	id, err := ArtifactID(config, kind, target, writers...)
	require.NoError(t, err)
	return id
}

// TestArtifactID_StableAcrossAbsoluteCheckoutPaths proves the same logical artifact (same kind,
// same target relative to the project base, same writers) hashes to the same ID regardless of the
// checkout's absolute filesystem path -- e.g. two different developers' clones, or a developer's
// clone versus a CI runner's clone.
//
// Found via real-world verification. ResolveComponentPath deliberately returns an absolute path
// (see its own doc comment), and before this fix ArtifactID hashed that absolute path verbatim, so
// a vendor.lock.yaml committed to git -- its entire purpose -- would never match on any checkout
// other than the one that originally ran `vendor pull`.
func TestArtifactID_StableAcrossAbsoluteCheckoutPaths(t *testing.T) {
	base1 := t.TempDir()
	base2 := t.TempDir()
	config1 := &schema.AtmosConfiguration{BasePath: base1}
	config2 := &schema.AtmosConfiguration{BasePath: base2}

	// Same relative target ("components/terraform/vpc"), but resolved to two different absolute
	// paths -- exactly what ResolveComponentPath produces on two different checkouts.
	target1 := filepath.Join(base1, "components", "terraform", "vpc")
	target2 := filepath.Join(base2, "components", "terraform", "vpc")
	require.NotEqual(t, target1, target2, "test setup: the two absolute targets must actually differ")

	id1, err := ArtifactID(config1, "remote", target1, "vpc")
	require.NoError(t, err)
	id2, err := ArtifactID(config2, "remote", target2, "vpc")
	require.NoError(t, err)

	require.Equal(t, id1, id2, "the same logical artifact must hash to the same ID regardless of the checkout's absolute path")
}

// TestArtifactID_RejectsTargetOutsideProjectBase proves a target that can't be expressed relative
// to the project base (escapes it, e.g. via "..") is rejected with an error rather than silently
// hashing something unexpected.
func TestArtifactID_RejectsTargetOutsideProjectBase(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	outside := filepath.Join(filepath.Dir(base), "not-the-project")

	_, err := ArtifactID(config, "remote", outside, "vpc")

	require.Error(t, err)
}

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
	lock.Artifacts["component"] = Artifact{Name: "component", Kind: "source", Target: target, Files: files}
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
	lock.Artifacts["first"] = Artifact{Name: "first", Kind: "source", Target: target, Files: files, Order: 1}
	lock.Artifacts["second"] = Artifact{Name: "second", Kind: "source", Target: target, Files: files, Order: 2}
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
    name: malicious
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
	id := mustArtifactID(t, config, artifact.Kind, artifact.Target)
	require.NoError(t, Replace(config, id, artifact))

	check, err := IsMaterialized(config, MaterializationParams{ID: id, Declared: "file:///source", Target: target})
	require.NoError(t, err)
	require.True(t, check.Materialized)
	require.Empty(t, check.Reason)

	check, err = IsMaterialized(config, MaterializationParams{ID: id, Declared: "file:///other", Target: target})
	require.NoError(t, err)
	require.False(t, check.Materialized)
	require.Equal(t, "declared source changed", check.Reason)

	require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("changed"), 0o644))
	check, err = IsMaterialized(config, MaterializationParams{ID: id, Declared: "file:///source", Target: target})
	require.NoError(t, err)
	require.False(t, check.Materialized)
	require.Equal(t, `file "owned.txt" checksum mismatch`, check.Reason)
}

// TestIsMaterializedDetectsIncludedExcludedPathsDrift proves a source whose only change is its
// declared included_paths/excluded_paths (with no change to the declared source URI itself) is
// correctly detected as drifted, and that an artifact recorded with no patterns at all (the
// pre-existing lock-file shape, before Artifact gained IncludedPaths/ExcludedPaths) never
// spuriously reports drift when compared against an equally-empty declared call.
func TestIsMaterializedDetectsIncludedExcludedPathsDrift(t *testing.T) {
	t.Parallel()

	// Each subtest gets its own isolated base/target/lock file (rather than sharing one across
	// t.Run calls) so both this test and its subtests can safely call t.Parallel(): Replace
	// read-modifies-writes the whole vendor.lock.yaml file, which is not safe for concurrent
	// writers sharing the same config/base.
	newFixture := func(t *testing.T) (config *schema.AtmosConfiguration, target, declared string) {
		t.Helper()
		base := t.TempDir()
		target = filepath.Join(base, "vendor")
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("main"), 0o644))
		return &schema.AtmosConfiguration{BasePath: base}, target, "file:///source"
	}

	t.Run("patterns-only change is detected as drift", func(t *testing.T) {
		t.Parallel()
		config, target, declared := newFixture(t)
		files, err := Inventory(target)
		require.NoError(t, err)

		artifact := Artifact{
			Kind:          "local",
			Target:        target,
			Source:        Source{Declared: declared},
			Files:         files,
			IncludedPaths: []string{"*.tf"},
		}
		id := mustArtifactID(t, config, artifact.Kind, artifact.Target, "patterned")
		require.NoError(t, Replace(config, id, artifact))

		// Same declared source and files, unchanged included patterns: still materialized.
		check, err := IsMaterialized(config, MaterializationParams{
			ID: id, Declared: declared, Target: target, IncludedPaths: []string{"*.tf"},
		})
		require.NoError(t, err)
		require.True(t, check.Materialized)

		// Same declared source, but included_paths in vendor.yaml/component.yaml changed: drift.
		check, err = IsMaterialized(config, MaterializationParams{
			ID: id, Declared: declared, Target: target, IncludedPaths: []string{"*.tf", "*.md"},
		})
		require.NoError(t, err)
		require.False(t, check.Materialized)
		require.Equal(t, "included/excluded paths changed", check.Reason)

		// Excluded-paths changes are detected the same way.
		check, err = IsMaterialized(config, MaterializationParams{
			ID: id, Declared: declared, Target: target, IncludedPaths: []string{"*.tf"}, ExcludedPaths: []string{"README.md"},
		})
		require.NoError(t, err)
		require.False(t, check.Materialized)
		require.Equal(t, "included/excluded paths changed", check.Reason)
	})

	t.Run("artifact with no recorded patterns does not spuriously drift", func(t *testing.T) {
		t.Parallel()
		config, target, declared := newFixture(t)
		files, err := Inventory(target)
		require.NoError(t, err)

		artifact := Artifact{Kind: "local", Target: target, Source: Source{Declared: declared}, Files: files}
		id := mustArtifactID(t, config, artifact.Kind, artifact.Target, "unfiltered")
		require.NoError(t, Replace(config, id, artifact))

		check, err := IsMaterialized(config, MaterializationParams{ID: id, Declared: declared, Target: target})
		require.NoError(t, err)
		require.True(t, check.Materialized, "an artifact recorded with nil patterns must compare equal to a call declaring none")
	})
}

// TestRecordPopulatesHTTPMetadataOnlyWhenProvided proves Record copies RecordOptions.HTTPMetadata
// into Source.ETag/Source.LastModified when the caller supplies it (an HTTP(S) fetch), and leaves
// both fields empty when it doesn't (git, OCI, or local sources), matching their omitempty tags.
func TestRecordPopulatesHTTPMetadataOnlyWhenProvided(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	tempDir := filepath.Join(base, "staged")
	require.NoError(t, os.MkdirAll(tempDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.tf"), []byte("main"), 0o644))

	config := &schema.AtmosConfiguration{BasePath: base}

	httpTarget := filepath.Join(base, "vendor", "http-source")
	require.NoError(t, Record(context.Background(), config, "remote", "http-source", tempDir, httpTarget, "https://example.com/archive.tar.gz", RecordOptions{
		HTTPMetadata: downloader.FetchMetadata{ETag: `"abc123"`, LastModified: "Wed, 21 Oct 2015 07:28:00 GMT"},
	}))

	localTarget := filepath.Join(base, "vendor", "local-source")
	require.NoError(t, Record(context.Background(), config, "local", "local-source", tempDir, localTarget, "file:///source", RecordOptions{}))

	lock, err := Load(config)
	require.NoError(t, err)

	httpArtifact, ok := lock.Artifacts[mustArtifactID(t, config, "remote", httpTarget, "http-source")]
	require.True(t, ok)
	require.Equal(t, `"abc123"`, httpArtifact.Source.ETag)
	require.Equal(t, "Wed, 21 Oct 2015 07:28:00 GMT", httpArtifact.Source.LastModified)

	localArtifact, ok := lock.Artifacts[mustArtifactID(t, config, "local", localTarget, "local-source")]
	require.True(t, ok)
	require.Empty(t, localArtifact.Source.ETag)
	require.Empty(t, localArtifact.Source.LastModified)
}

// TestRecordPopulatesVersionResolutionOnlyWhenProvided proves Record copies
// RecordOptions.VersionConstraint/ResolvedVersion into Source.VersionConstraint/Source.ResolvedVersion
// when the caller supplies them (a range-declared `version:`), and leaves both fields empty
// otherwise (an exact-pinned `version:`), matching their omitempty tags -- mirroring
// TestRecordPopulatesHTTPMetadataOnlyWhenProvided's shape for the sibling ETag/LastModified fields.
func TestRecordPopulatesVersionResolutionOnlyWhenProvided(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	tempDir := filepath.Join(base, "staged")
	require.NoError(t, os.MkdirAll(tempDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.tf"), []byte("main"), 0o644))

	config := &schema.AtmosConfiguration{BasePath: base}

	rangeTarget := filepath.Join(base, "vendor", "range-source")
	require.NoError(t, Record(context.Background(), config, "remote", "range-source", tempDir, rangeTarget, "https://example.com/archive.tar.gz", RecordOptions{
		VersionConstraint: "^1.0.0",
		ResolvedVersion:   "1.2.3",
	}))

	pinnedTarget := filepath.Join(base, "vendor", "pinned-source")
	require.NoError(t, Record(context.Background(), config, "remote", "pinned-source", tempDir, pinnedTarget, "https://example.com/archive.tar.gz", RecordOptions{}))

	lock, err := Load(config)
	require.NoError(t, err)

	rangeArtifact, ok := lock.Artifacts[mustArtifactID(t, config, "remote", rangeTarget, "range-source")]
	require.True(t, ok)
	require.Equal(t, "^1.0.0", rangeArtifact.Source.VersionConstraint)
	require.Equal(t, "1.2.3", rangeArtifact.Source.ResolvedVersion)

	pinnedArtifact, ok := lock.Artifacts[mustArtifactID(t, config, "remote", pinnedTarget, "pinned-source")]
	require.True(t, ok)
	require.Empty(t, pinnedArtifact.Source.VersionConstraint)
	require.Empty(t, pinnedArtifact.Source.ResolvedVersion)

	// Round-trip through YAML: omitempty means a pinned source's marshaled artifact must not even
	// contain the version_constraint/resolved_version keys.
	require.NoError(t, Save(config, lock))
	data, err := os.ReadFile(Path(config))
	require.NoError(t, err)
	require.Contains(t, string(data), "version_constraint: ^1.0.0")
	require.Contains(t, string(data), "resolved_version: 1.2.3")
}

// TestIsMaterializedAndVerifyIgnoreETagAndLastModified is the dedicated "cache metadata is never
// authoritative" guarantee: two artifacts identical except for ETag/LastModified must compare as
// materialized/drift-free identically, and mutating only those fields in an already-recorded
// artifact must never flip IsMaterialized or Verify's result.
func TestIsMaterializedAndVerifyIgnoreETagAndLastModified(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "vendor")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "owned.txt"), []byte("content"), 0o644))
	files, err := Inventory(target)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{BasePath: base}
	declared := "https://example.com/a.tar.gz"

	withETag := Artifact{
		Kind:   "remote",
		Target: target,
		Source: Source{Declared: declared, Digest: "sha256:deadbeef", ETag: `"etag-1"`, LastModified: "Mon, 01 Jan 2024 00:00:00 GMT"},
		Files:  files,
	}
	idWith := mustArtifactID(t, config, withETag.Kind, withETag.Target, "with-etag")
	require.NoError(t, Replace(config, idWith, withETag))

	withoutETag := withETag
	withoutETag.Source.ETag = ""
	withoutETag.Source.LastModified = ""
	idWithout := mustArtifactID(t, config, withoutETag.Kind, withoutETag.Target, "without-etag")
	require.NoError(t, Replace(config, idWithout, withoutETag))

	checkWith, err := IsMaterialized(config, MaterializationParams{ID: idWith, Declared: declared, Target: target})
	require.NoError(t, err)
	checkWithout, err := IsMaterialized(config, MaterializationParams{ID: idWithout, Declared: declared, Target: target})
	require.NoError(t, err)
	require.True(t, checkWith.Materialized)
	require.Equal(t, checkWith.Materialized, checkWithout.Materialized, "ETag/LastModified presence must never affect materialization")

	lock, err := Load(config)
	require.NoError(t, err)
	drifts, err := Verify(config, lock)
	require.NoError(t, err)
	require.Empty(t, drifts, "identical file contents must verify cleanly regardless of ETag/LastModified presence")

	// Mutate only ETag/LastModified on the already-recorded artifact (simulating a re-fetch that
	// returned a new cache-validator but identical bytes) and confirm neither function reacts.
	mutated := lock.Artifacts[idWith]
	mutated.Source.ETag = `"etag-2-different"`
	mutated.Source.LastModified = "Tue, 02 Jan 2024 00:00:00 GMT"
	lock.Artifacts[idWith] = mutated
	require.NoError(t, Save(config, lock))

	checkAfterMutation, err := IsMaterialized(config, MaterializationParams{ID: idWith, Declared: declared, Target: target})
	require.NoError(t, err)
	require.True(t, checkAfterMutation.Materialized, "changing only ETag/LastModified in the lock must not invalidate materialization")

	drifts, err = Verify(config, lock)
	require.NoError(t, err)
	require.Empty(t, drifts, "changing only ETag/LastModified in the lock must not report drift")
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
		require.ErrorIs(t, err, ErrReadVendorLock)
	})

	t.Run("invalid yaml surfaces a parse error", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, os.WriteFile(Path(config), []byte("not: [valid: yaml"), 0o644))
		_, err := Load(config)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrParseVendorLock)
	})

	t.Run("unsupported version is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		require.NoError(t, os.WriteFile(Path(config), []byte("version: 2\nartifacts: {}\n"), 0o644))
		_, err := Load(config)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrUnsupportedVendorLockVersion)
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
		require.ErrorIs(t, err, ErrNormalizeLockTarget)
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
		require.ErrorIs(t, err, ErrCreateVendorLockDir)
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
		_, err := IsMaterialized(config, MaterializationParams{ID: "id", Declared: "declared", Target: filepath.Join(base, "vendor")})
		require.Error(t, err)
	})

	t.Run("target escaping project root is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		_, err := IsMaterialized(config, MaterializationParams{ID: "id", Declared: "declared", Target: filepath.Join(base, "..", "outside")})
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
		_, err := IsMaterialized(config, MaterializationParams{ID: "malicious", Declared: "file:///source", Target: target})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInvalidLockOwnedFilePath)
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
		id := mustArtifactID(t, config, artifact.Kind, artifact.Target)
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
		id := mustArtifactID(t, config, artifact.Kind, artifact.Target)
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
		id := mustArtifactID(t, config, artifact.Kind, artifact.Target)
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
		require.ErrorIs(t, err, ErrStaleLockOwnedFileModified)

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
		idA := mustArtifactID(t, config, artifactA.Kind, artifactA.Target, "a")
		idB := mustArtifactID(t, config, artifactA.Kind, artifactA.Target, "b")
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
		lock.Artifacts["component"] = Artifact{Name: "component", Kind: "source", Target: target, Files: files}
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
		lock.Artifacts["component"] = Artifact{Name: "component", Kind: "source", Target: target, Files: files}
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
    name: other
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
				require.ErrorIs(t, err, ErrInvalidLockOwnedFilePath)
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
	require.ErrorIs(t, err, ErrInvalidVendorLockTarget)

	_, err = projectRelativeTarget(config, base)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidVendorLockTarget)
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
	require.ErrorIs(t, err, ErrCreateTempVendorLock)
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
	require.ErrorIs(t, err, ErrInventoryWalk)

	_, err = VendorInventoryWithPatterns(missing, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInventoryWalk)
}

func TestReplaceRejectsInvalidTargetsCorruptLockAndInvalidFilePaths(t *testing.T) {
	t.Run("new artifact target escaping the project root is rejected", func(t *testing.T) {
		base := t.TempDir()
		config := &schema.AtmosConfiguration{BasePath: base}
		err := Replace(config, "id", Artifact{Kind: "source", Target: filepath.Join(base, "..", "outside")})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNormalizeArtifactTarget)
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
		id := mustArtifactID(t, config, artifactWithGhost.Kind, artifactWithGhost.Target)
		require.NoError(t, Replace(config, id, artifactWithGhost))

		err = Replace(config, id, Artifact{Kind: "source", Target: target, Files: keptOnly})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInspectLockOwnedFile)
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
		id := mustArtifactID(t, config, artifact.Kind, artifact.Target)
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
		require.ErrorIs(t, err, ErrRemoveLockOwnedFile)
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
		lock.Artifacts["component"] = Artifact{Name: "component", Kind: "source", Target: target, Files: []File{{Path: "blocker/nested.txt", Type: "file", SHA256: "ignored"}}}
		require.NoError(t, Save(config, lock))

		_, err := Clean(config, "", false, false)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInspectLockOwnedFile)
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
		lock.Artifacts["component"] = Artifact{Name: "component", Kind: "source", Target: target, Files: files}
		require.NoError(t, Save(config, lock))

		require.NoError(t, os.Chmod(target, 0o555))
		defer func() { _ = os.Chmod(target, 0o755) }()

		_, err = Clean(config, "", true, false)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrRemoveLockOwnedFile)
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
		lock.Artifacts["component"] = Artifact{Name: "component", Kind: "source", Target: target, Files: files}
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
