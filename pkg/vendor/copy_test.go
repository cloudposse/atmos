package vendor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFileInfo implements os.FileInfo for testing skip functions.
type mockFileInfo struct {
	name  string
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }

// trySymlink attempts to create a symlink and skips the test if unsupported.
// On Windows or locked-down environments, creating symlinks may fail with EPERM.
func trySymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("skipping symlink test: cannot create symlink (%v)", err)
	}
}

func TestCopyToTarget_BasicCopy(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "variables.tf"), []byte("# vars"), 0o644))

	modulesDir := filepath.Join(srcDir, "modules", "vpc")
	require.NoError(t, os.MkdirAll(modulesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modulesDir, "main.tf"), []byte("# vpc"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{})
	require.NoError(t, err)

	// All files should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(targetDir, "variables.tf"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(targetDir, "modules", "vpc", "main.tf"))
	assert.NoError(t, err)
}

func TestCopyToTarget_WithExcludedPaths(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# readme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "notes.txt"), []byte("notes"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{
		ExcludedPaths: []string{"*.md", "*.txt"},
	})
	require.NoError(t, err)

	// .tf file should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err)

	// Excluded files should NOT be copied.
	_, err = os.Stat(filepath.Join(targetDir, "README.md"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(targetDir, "notes.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestCopyToTarget_WithIncludedPaths(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "variables.tf"), []byte("# vars"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# readme"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{
		IncludedPaths: []string{"*.tf"},
	})
	require.NoError(t, err)

	// .tf files should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(targetDir, "variables.tf"))
	assert.NoError(t, err)

	// Non-.tf files should NOT be copied.
	_, err = os.Stat(filepath.Join(targetDir, "README.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestCopyToTarget_WithDoublestarExcludedPaths(t *testing.T) {
	// This is the core bug fix test: ** patterns must work at any depth.
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "providers.tf"), []byte("# root providers"), 0o644))

	modulesDir := filepath.Join(srcDir, "modules", "vpc")
	require.NoError(t, os.MkdirAll(modulesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modulesDir, "main.tf"), []byte("# vpc"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modulesDir, "providers.tf"), []byte("# vpc providers"), 0o644))

	deepDir := filepath.Join(srcDir, "a", "b", "c")
	require.NoError(t, os.MkdirAll(deepDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deepDir, "providers.tf"), []byte("# deep providers"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{
		ExcludedPaths: []string{"**/providers.tf"},
	})
	require.NoError(t, err)

	// main.tf should be copied at all levels.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err, "root main.tf should be copied")
	_, err = os.Stat(filepath.Join(targetDir, "modules", "vpc", "main.tf"))
	assert.NoError(t, err, "nested main.tf should be copied")

	// providers.tf should be excluded at ALL levels.
	_, err = os.Stat(filepath.Join(targetDir, "providers.tf"))
	assert.True(t, os.IsNotExist(err), "root providers.tf should be excluded by **/providers.tf")
	_, err = os.Stat(filepath.Join(targetDir, "modules", "vpc", "providers.tf"))
	assert.True(t, os.IsNotExist(err), "nested providers.tf should be excluded by **/providers.tf")
	_, err = os.Stat(filepath.Join(targetDir, "a", "b", "c", "providers.tf"))
	assert.True(t, os.IsNotExist(err), "deeply nested providers.tf should be excluded by **/providers.tf")
}

func TestCopyToTarget_SkipsGitDirectory(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))

	gitDir := filepath.Join(srcDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{})
	require.NoError(t, err)

	// main.tf should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err)

	// .git directory should NOT be copied.
	_, err = os.Stat(filepath.Join(targetDir, ".git"))
	assert.True(t, os.IsNotExist(err), ".git directory should be skipped")
}

func TestCopyToTarget_SymlinkInsideSrcDir(t *testing.T) {
	srcDir := t.TempDir()

	// Create a real file and a symlink pointing to it (inside srcDir).
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "real.tf"), []byte("# real"), 0o644))
	trySymlink(t, filepath.Join(srcDir, "real.tf"), filepath.Join(srcDir, "link.tf"))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{})
	require.NoError(t, err)

	// Both should be copied (symlink followed since target is inside srcDir).
	_, err = os.Stat(filepath.Join(targetDir, "real.tf"))
	assert.NoError(t, err, "real file should be copied")
	_, err = os.Stat(filepath.Join(targetDir, "link.tf"))
	assert.NoError(t, err, "symlink with target inside srcDir should be followed")

	// Verify the symlink target was dereferenced (copied as regular file).
	content, err := os.ReadFile(filepath.Join(targetDir, "link.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# real", string(content))
}

func TestCopyToTarget_SymlinkOutsideSrcDir(t *testing.T) {
	srcDir := t.TempDir()

	// Create a file outside srcDir.
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret data"), 0o644))

	// Create a real file and a symlink pointing outside srcDir.
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))
	trySymlink(t, outsideFile, filepath.Join(srcDir, "escape.txt"))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{})
	require.NoError(t, err)

	// Real file should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err, "real file should be copied")

	// Symlink pointing outside srcDir should be skipped.
	_, err = os.Stat(filepath.Join(targetDir, "escape.txt"))
	assert.True(t, os.IsNotExist(err), "symlink pointing outside srcDir should be skipped")
}

func TestCopyToTarget_SymlinkToGitDir(t *testing.T) {
	srcDir := t.TempDir()

	// Create a .git directory with a HEAD file inside srcDir.
	gitDir := filepath.Join(srcDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0o644))

	// Create a regular file.
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))

	// Create a symlink pointing into .git/ (e.g., head.txt -> .git/HEAD).
	// This should be skipped because the resolved target is inside .git/.
	trySymlink(t, filepath.Join(gitDir, "HEAD"), filepath.Join(srcDir, "head.txt"))

	targetDir := filepath.Join(t.TempDir(), "target")
	err := CopyToTarget(srcDir, targetDir, CopyOptions{})
	require.NoError(t, err)

	// main.tf should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err, "real file should be copied")

	// Symlink pointing into .git/ should be skipped.
	_, err = os.Stat(filepath.Join(targetDir, "head.txt"))
	assert.True(t, os.IsNotExist(err), "symlink pointing into .git/ should be skipped")

	// .git directory itself should also be skipped.
	_, err = os.Stat(filepath.Join(targetDir, ".git"))
	assert.True(t, os.IsNotExist(err), ".git directory should be skipped")
}

func TestCreateSkipFunc_GitDirectory(t *testing.T) {
	srcBase := filepath.Join(string(filepath.Separator), "tmp", "src")
	dstBase := filepath.Join(string(filepath.Separator), "tmp", "dst")
	skipFunc := CreateSkipFunc(srcBase, nil, nil)
	info := &mockFileInfo{name: ".git", isDir: true}
	skip, err := skipFunc(info, filepath.Join(srcBase, ".git"), filepath.Join(dstBase, ".git"))
	assert.NoError(t, err)
	assert.True(t, skip, ".git directory should always be skipped")
}

func TestCreateSkipFunc_NoPatterns(t *testing.T) {
	srcBase := filepath.Join(string(filepath.Separator), "tmp", "src")
	dstBase := filepath.Join(string(filepath.Separator), "tmp", "dst")
	skipFunc := CreateSkipFunc(srcBase, nil, nil)
	info := &mockFileInfo{name: "main.tf", isDir: false}
	skip, err := skipFunc(info, filepath.Join(srcBase, "main.tf"), filepath.Join(dstBase, "main.tf"))
	assert.NoError(t, err)
	assert.False(t, skip, "without patterns, no files should be skipped")
}

func TestCreateSkipFunc_ExcludePattern(t *testing.T) {
	srcBase := filepath.Join(string(filepath.Separator), "tmp", "src")
	dstBase := filepath.Join(string(filepath.Separator), "tmp", "dst")
	skipFunc := CreateSkipFunc(srcBase, nil, []string{"*.md"})
	// .md file should be skipped.
	info := &mockFileInfo{name: "README.md", isDir: false}
	skip, err := skipFunc(info, filepath.Join(srcBase, "README.md"), filepath.Join(dstBase, "README.md"))
	assert.NoError(t, err)
	assert.True(t, skip, ".md file should be skipped by exclude pattern")

	// .tf file should not be skipped.
	info = &mockFileInfo{name: "main.tf", isDir: false}
	skip, err = skipFunc(info, filepath.Join(srcBase, "main.tf"), filepath.Join(dstBase, "main.tf"))
	assert.NoError(t, err)
	assert.False(t, skip, ".tf file should not be skipped")
}

func TestCreateSkipFunc_IncludePattern(t *testing.T) {
	srcBase := filepath.Join(string(filepath.Separator), "tmp", "src")
	dstBase := filepath.Join(string(filepath.Separator), "tmp", "dst")
	skipFunc := CreateSkipFunc(srcBase, []string{"*.tf"}, nil)
	// .tf file should not be skipped.
	info := &mockFileInfo{name: "main.tf", isDir: false}
	skip, err := skipFunc(info, filepath.Join(srcBase, "main.tf"), filepath.Join(dstBase, "main.tf"))
	assert.NoError(t, err)
	assert.False(t, skip, ".tf file matches include pattern, should not be skipped")

	// .md file should be skipped.
	info = &mockFileInfo{name: "README.md", isDir: false}
	skip, err = skipFunc(info, filepath.Join(srcBase, "README.md"), filepath.Join(dstBase, "README.md"))
	assert.NoError(t, err)
	assert.True(t, skip, ".md file does not match include pattern, should be skipped")

	// Directories should never be skipped when include patterns are set.
	info = &mockFileInfo{name: "modules", isDir: true}
	skip, err = skipFunc(info, filepath.Join(srcBase, "modules"), filepath.Join(dstBase, "modules"))
	assert.NoError(t, err)
	assert.False(t, skip, "directories should not be skipped when traversing for includes")
}

func TestCreateSkipFunc_CombinedPatterns(t *testing.T) {
	srcBase := filepath.Join(string(filepath.Separator), "tmp", "src")
	dstBase := filepath.Join(string(filepath.Separator), "tmp", "dst")
	skipFunc := CreateSkipFunc(srcBase, []string{"*.tf", "*.md"}, []string{"README.md"})

	// main.tf: matches include, not in exclude.
	info := &mockFileInfo{name: "main.tf", isDir: false}
	skip, err := skipFunc(info, filepath.Join(srcBase, "main.tf"), filepath.Join(dstBase, "main.tf"))
	assert.NoError(t, err)
	assert.False(t, skip)

	// CHANGELOG.md: matches include, not in exclude.
	info = &mockFileInfo{name: "CHANGELOG.md", isDir: false}
	skip, err = skipFunc(info, filepath.Join(srcBase, "CHANGELOG.md"), filepath.Join(dstBase, "CHANGELOG.md"))
	assert.NoError(t, err)
	assert.False(t, skip)

	// README.md: matches both include AND exclude; exclude wins.
	info = &mockFileInfo{name: "README.md", isDir: false}
	skip, err = skipFunc(info, filepath.Join(srcBase, "README.md"), filepath.Join(dstBase, "README.md"))
	assert.NoError(t, err)
	assert.True(t, skip, "excluded pattern should take priority over included")

	// main.go: not in include patterns, should be skipped.
	info = &mockFileInfo{name: "main.go", isDir: false}
	skip, err = skipFunc(info, filepath.Join(srcBase, "main.go"), filepath.Join(dstBase, "main.go"))
	assert.NoError(t, err)
	assert.True(t, skip, "file not matching any include pattern should be skipped")
}

func TestShouldExcludeFile(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		skip     bool
		hasErr   bool
	}{
		{name: "exact match", patterns: []string{"providers.tf"}, path: "providers.tf", skip: true},
		{name: "wildcard match", patterns: []string{"*.md"}, path: "README.md", skip: true},
		{name: "no match", patterns: []string{"*.md"}, path: "main.tf", skip: false},
		{name: "doublestar match nested", patterns: []string{"**/providers.tf"}, path: "modules/vpc/providers.tf", skip: true},
		{name: "doublestar match root", patterns: []string{"**/providers.tf"}, path: "providers.tf", skip: true},
		{name: "single star no nested match", patterns: []string{"*.tf"}, path: "modules/vpc/main.tf", skip: false},
		{name: "empty patterns", patterns: []string{}, path: "main.tf", skip: false},
		{name: "invalid pattern", patterns: []string{"[invalid"}, path: "main.tf", skip: false, hasErr: true},
		// Mixed valid+invalid: the invalid pattern is encountered first and returns an error.
		{name: "mixed valid and invalid patterns - error on first", patterns: []string{"[invalid", "*.md"}, path: "README.md", skip: false, hasErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := ShouldExcludeFile(tt.patterns, tt.path)
			if tt.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.skip, skip)
		})
	}
}

func TestShouldIncludeFile(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		skip     bool
		hasErr   bool
	}{
		{name: "match - not skipped", patterns: []string{"*.tf"}, path: "main.tf", skip: false},
		{name: "no match - skipped", patterns: []string{"*.tf"}, path: "README.md", skip: true},
		{name: "doublestar match nested", patterns: []string{"**/*.tf"}, path: "modules/vpc/main.tf", skip: false},
		{name: "single star no nested match", patterns: []string{"*.tf"}, path: "modules/vpc/main.tf", skip: true},
		{name: "multiple patterns first matches", patterns: []string{"*.tf", "*.md"}, path: "main.tf", skip: false},
		{name: "multiple patterns second matches", patterns: []string{"*.tf", "*.md"}, path: "README.md", skip: false},
		{name: "multiple patterns none match", patterns: []string{"*.tf", "*.md"}, path: "main.go", skip: true},
		{name: "invalid pattern", patterns: []string{"[invalid"}, path: "main.tf", skip: false, hasErr: true},
		// Empty patterns: the loop never executes, so no pattern matched → skip=true.
		{name: "empty patterns", patterns: []string{}, path: "main.tf", skip: true},
		// Mixed valid+invalid: the invalid pattern is encountered first and returns an error.
		{name: "mixed valid and invalid patterns - error on first", patterns: []string{"[invalid", "*.tf"}, path: "main.tf", skip: false, hasErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := ShouldIncludeFile(tt.patterns, tt.path)
			if tt.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.skip, skip)
		})
	}
}

// TestCopyToTarget_InvalidIncludedPattern verifies that CopyToTarget returns an error
// when IncludedPaths contains an invalid glob pattern, and does not copy any files.
func TestCopyToTarget_InvalidIncludedPattern(t *testing.T) {
srcDir := t.TempDir()
require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))

targetDir := filepath.Join(t.TempDir(), "target")
err := CopyToTarget(srcDir, targetDir, CopyOptions{
IncludedPaths: []string{"[invalid"},
})
require.Error(t, err, "CopyToTarget with an invalid IncludedPaths pattern should return an error")

// No file should have been copied to the target directory.
_, statErr := os.Stat(filepath.Join(targetDir, "main.tf"))
assert.True(t, os.IsNotExist(statErr), "no files should be copied when IncludedPaths is invalid")
}

// TestCopyToTarget_InvalidExcludedPattern verifies that CopyToTarget returns an error
// when ExcludedPaths contains an invalid glob pattern, and does not copy any files.
func TestCopyToTarget_InvalidExcludedPattern(t *testing.T) {
srcDir := t.TempDir()
require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))

targetDir := filepath.Join(t.TempDir(), "target")
err := CopyToTarget(srcDir, targetDir, CopyOptions{
ExcludedPaths: []string{"[invalid"},
})
require.Error(t, err, "CopyToTarget with an invalid ExcludedPaths pattern should return an error")
}

// TestCopyToTarget_SymlinkWithIncludedPaths verifies that symlinks are filtered by IncludedPaths.
// A symlink whose resolved target does not match the include patterns should be skipped.
func TestCopyToTarget_SymlinkWithIncludedPaths(t *testing.T) {
srcDir := t.TempDir()

// Create a .tf file and a .md file; the symlink points to the .md file.
require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644))
require.NoError(t, os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# readme"), 0o644))
// Symlink to a .md file — should be skipped when includedPaths is ["*.tf"].
trySymlink(t, filepath.Join(srcDir, "README.md"), filepath.Join(srcDir, "link-to-readme.md"))

targetDir := filepath.Join(t.TempDir(), "target")
err := CopyToTarget(srcDir, targetDir, CopyOptions{
IncludedPaths: []string{"*.tf"},
})
require.NoError(t, err)

// .tf file should be copied.
_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
assert.NoError(t, err, "main.tf should be copied")

// .md file and the symlink pointing to it should NOT be copied.
_, err = os.Stat(filepath.Join(targetDir, "README.md"))
assert.True(t, os.IsNotExist(err), "README.md does not match include pattern and should be skipped")
_, err = os.Stat(filepath.Join(targetDir, "link-to-readme.md"))
assert.True(t, os.IsNotExist(err), "symlink to .md file should be skipped by include pattern")
}
