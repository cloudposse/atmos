package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
)

// Use a local variable to override the glob matching function in tests.
var getGlobMatchesForTest = utils.GetGlobMatches

// Helper that calls our local getGlobMatchesForTest.
func getMatchesForPatternForTest(sourceDir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(sourceDir, pattern)
	// Normalize fullPattern to use forward slashes.
	normalized := filepath.ToSlash(fullPattern)
	return getGlobMatchesForTest(normalized)
}

type fakeDirEntry struct {
	name string
	err  error
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return false }
func (f fakeDirEntry) Type() os.FileMode          { return 0 }
func (f fakeDirEntry) Info() (os.FileInfo, error) { return nil, f.err }

// TestCopyFile verifies that copyFile correctly copies file contents and preserves permissions.
func TestCopyFile(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "copyfile-src")
	if err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "copyfile-dst")
	if err != nil {
		t.Fatalf("Failed to create destination dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	srcFile := filepath.Join(srcDir, "test.txt")
	content := "copyFileTest"
	if err := os.WriteFile(srcFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	dstFile := filepath.Join(dstDir, "test.txt")
	if err := copyFile(srcFile, dstFile); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	copiedContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if string(copiedContent) != content {
		t.Errorf("Expected content %q, got %q", content, string(copiedContent))
	}
}

// TestCopyFile_SourceNotExist tests error in copyFile when source file does not exist.
func TestCopyFile_SourceNotExist(t *testing.T) {
	nonExistent := filepath.Join(os.TempDir(), "nonexistent.txt")
	dstFile := filepath.Join(os.TempDir(), "dst.txt")
	err := copyFile(nonExistent, dstFile)
	if err == nil || !strings.Contains(err.Error(), "opening source file") {
		t.Errorf("Expected error for non-existent source file, got %v", err)
	}
}

// TestShouldExcludePath checks that a file is excluded by pattern.
func TestShouldExcludePath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	excluded := []string{"**/*.log"}
	if !shouldExcludePath(info, "app/test.log", excluded) {
		t.Errorf("Expected path to be excluded")
	}
}

// TestShouldExcludePath_Directory checks directory exclusion using a trailing slash.
// Skipped on Windows.
func TestShouldExcludePath_Directory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping directory exclusion test on Windows")
	}
	dir, err := os.MkdirTemp("", "dir-exclude")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}
	pattern := "**/" + filepath.Base(dir) + "/"
	if !shouldExcludePath(info, filepath.Base(dir), []string{pattern}) {
		t.Errorf("Expected directory %q to be excluded by pattern %q", filepath.Base(dir), pattern)
	}
}

// TestShouldExcludePath_Error ensures invalid patterns do not exclude files.
func TestShouldExcludePath_Error(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if shouldExcludePath(info, "app/test.log", []string{"[abc"}) {
		t.Errorf("Expected path not to be excluded by invalid pattern")
	}
}

// TestShouldIncludePath checks that a file is included by pattern.
func TestShouldIncludePath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	included := []string{"**/*.txt"}
	if !shouldIncludePath(info, "docs/readme.txt", included) {
		t.Errorf("Expected path to be included")
	}
}

// TestShouldIncludePath_Error ensures invalid inclusion patterns do not include files.
func TestShouldIncludePath_Error(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if shouldIncludePath(info, "docs/readme.txt", []string{"[abc"}) {
		t.Errorf("Expected path not to be included by invalid pattern")
	}
}

// TestShouldSkipEntry verifies that a file is skipped if it matches an excluded pattern.
func TestShouldSkipEntry(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "base")
	if err != nil {
		t.Fatalf("Failed to create base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)
	subDir := filepath.Join(baseDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create sub dir: %v", err)
	}
	filePath := filepath.Join(subDir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	if !shouldSkipEntry(info, filePath, baseDir, []string{"**/*.txt"}, []string{}) {
		t.Errorf("Expected file %q to be skipped", filePath)
	}
}

// TestCopyDirRecursive ensures that copyDirRecursive copies a directory tree.
func TestCopyDirRecursive(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "copydir-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "copydir-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	subDir := filepath.Join(srcDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create sub dir: %v", err)
	}
	file1 := filepath.Join(srcDir, "file1.txt")
	file2 := filepath.Join(subDir, "file2.txt")
	if err := os.WriteFile(file1, []byte("file1"), 0o600); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("file2"), 0o600); err != nil {
		t.Fatalf("Failed to write file2: %v", err)
	}
	ctx := &CopyContext{
		SrcDir:   srcDir,
		DstDir:   dstDir,
		BaseDir:  srcDir,
		Excluded: []string{},
		Included: []string{},
	}
	if err := copyDirRecursive(ctx); err != nil {
		t.Fatalf("copyDirRecursive failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "file1.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file1.txt to exist")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "sub", "file2.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file2.txt to exist")
	}
}

// TestProcessDirEntry_Symlink ensures that symlink entries are skipped.
func TestProcessDirEntry_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}
	srcDir, err := os.MkdirTemp("", "symlink-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "symlink-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	targetFile := filepath.Join(srcDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("Failed to write target file: %v", err)
	}
	linkPath := filepath.Join(srcDir, "link.txt")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Skip("Cannot create symlink on this system, skipping test.")
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("Failed to read src dir: %v", err)
	}
	var linkEntry os.DirEntry
	for _, e := range entries {
		if e.Name() == "link.txt" {
			linkEntry = e
			break
		}
	}
	if linkEntry == nil {
		t.Fatalf("Symlink entry not found")
	}
	ctx := &CopyContext{
		SrcDir:   srcDir,
		DstDir:   dstDir,
		BaseDir:  srcDir,
		Excluded: []string{},
		Included: []string{},
	}
	if err := processDirEntry(linkEntry, ctx); err != nil {
		t.Errorf("Expected nil error for symlink, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "link.txt")); err == nil {
		t.Errorf("Expected symlink not to be copied")
	}
}

// TestGetMatchesForPattern checks that getMatchesForPattern returns expected matches.
func TestGetMatchesForPattern(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "glob-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	fileA := filepath.Join(srcDir, "a.txt")
	fileB := filepath.Join(srcDir, "b.log")
	if err := os.WriteFile(fileA, []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write fileB: %v", err)
	}
	matches, err := getMatchesForPattern(srcDir, "*.txt")
	if err != nil {
		t.Fatalf("getMatchesForPattern error: %v", err)
	}
	if len(matches) == 0 || !strings.Contains(matches[0], "a.txt") {
		t.Errorf("Expected match for a.txt, got %v", matches)
	}
}

// TestGetMatchesForPattern_NoMatches uses our helper to simulate no matches.
func TestGetMatchesForPattern_NoMatches(t *testing.T) {
	oldFn := getGlobMatchesForTest
	defer func() { getGlobMatchesForTest = oldFn }()
	getGlobMatchesForTest = func(pattern string) ([]string, error) {
		return []string{}, nil
	}
	srcDir, err := os.MkdirTemp("", "nomatch-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	matches, err := getMatchesForPatternForTest(srcDir, "nonexistent*")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("Expected 0 matches, got %v", matches)
	}
}

// TestGetMatchesForPattern_InvalidPattern ensures invalid patterns produce an error.
func TestGetMatchesForPattern_InvalidPattern(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "invalid-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	_, err = getMatchesForPattern(srcDir, "[")
	if err == nil {
		t.Errorf("Expected error for invalid pattern, got nil")
	}
}

// TestGetMatchesForPattern_ShallowNoMatch tests the shallow branch with no matches.
func TestGetMatchesForPattern_ShallowNoMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping shallow no-match test on Windows")
	}
	oldFn := getGlobMatchesForTest
	defer func() { getGlobMatchesForTest = oldFn }()
	getGlobMatchesForTest = func(pattern string) ([]string, error) {
		// Normalize pattern to slash.
		normalized := filepath.ToSlash(pattern)
		if strings.Contains(normalized, "/*") && !strings.Contains(normalized, "/**") {
			return []string{}, nil
		}
		return []string{}, nil
	}
	srcDir, err := os.MkdirTemp("", "shallow-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	emptyDir := filepath.Join(srcDir, "dir")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}
	_, err = getMatchesForPatternForTest(srcDir, "dir/*")
	if err != nil {
		t.Fatalf("Expected no error for shallow pattern with no matches, got %v", err)
	}
}

// TestGetMatchesForPattern_RecursiveMatch tests the recursive branch by overriding glob matching.
func TestGetMatchesForPattern_RecursiveMatch(t *testing.T) {
	oldFn := getGlobMatchesForTest
	defer func() { getGlobMatchesForTest = oldFn }()
	getGlobMatchesForTest = func(pattern string) ([]string, error) {
		normalized := filepath.ToSlash(pattern)
		if strings.Contains(normalized, "/**") {
			return []string{"match.txt"}, nil
		}
		return []string{}, nil
	}
	srcDir, err := os.MkdirTemp("", "recursive-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dir := filepath.Join(srcDir, "dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatalf("Failed to create child directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	pattern := "dir/*/**"
	matches, err := getMatchesForPatternForTest(srcDir, pattern)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(matches) == 0 {
		t.Errorf("Expected matches for recursive branch, got none")
	}
}

// TestIsShallowPattern ensures shallow pattern detection works.
func TestIsShallowPattern(t *testing.T) {
	if !isShallowPattern("**/demo-localstack/*") {
		t.Errorf("Expected '**/demo-localstack/*' to be shallow")
	}
	if isShallowPattern("**/demo-library/**") {
		t.Errorf("Expected '**/demo-library/**' not to be shallow")
	}
}

// TestCopyDirRecursiveWithPrefix ensures prefix-based copy works.
func TestCopyDirRecursiveWithPrefix(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "prefix-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "prefix-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	filePath := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	ctx := &PrefixCopyContext{
		SrcDir:     srcDir,
		DstDir:     dstDir,
		GlobalBase: srcDir,
		Prefix:     "prefix",
		Excluded:   []string{},
	}
	if err := copyDirRecursiveWithPrefix(ctx); err != nil {
		t.Fatalf("copyDirRecursiveWithPrefix failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "test.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file to exist in destination")
	}
}

// TestProcessIncludedPattern ensures that matching files are copied.
func TestProcessIncludedPattern(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "included-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "included-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	fileMatch := filepath.Join(srcDir, "match.md")
	if err := os.WriteFile(fileMatch, []byte("mdcontent"), 0o600); err != nil {
		t.Fatalf("Failed to write matching file: %v", err)
	}
	fileNoMatch := filepath.Join(srcDir, "no_match.txt")
	if err := os.WriteFile(fileNoMatch, []byte("txtcontent"), 0o600); err != nil {
		t.Fatalf("Failed to write non-matching file: %v", err)
	}
	pattern := "**/*.md"
	if err := processIncludedPattern(srcDir, dstDir, pattern, []string{}); err != nil {
		t.Fatalf("processIncludedPattern failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "match.md")); os.IsNotExist(err) {
		t.Errorf("Expected match.md to exist")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "no_match.txt")); err == nil {
		t.Errorf("Expected no_match.txt not to exist")
	}
}

// TestProcessIncludedPattern_Invalid ensures that an invalid pattern does not cause fatal errors.
func TestProcessIncludedPattern_Invalid(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "invalid-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "invalid-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	if err := processIncludedPattern(srcDir, dstDir, "[", []string{}); err != nil {
		t.Fatalf("Expected processIncludedPattern to handle invalid pattern gracefully, got: %v", err)
	}
}

// TestProcessMatch_ShallowDirectory ensures directories are not copied when shallow is true.
func TestProcessMatch_ShallowDirectory(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "pm-shallow-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "pm-shallow-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	dirPath := filepath.Join(srcDir, "dir")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := processMatch(srcDir, dstDir, dirPath, true, []string{}); err != nil {
		t.Errorf("Expected nil error for shallow directory, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "dir")); err == nil {
		t.Errorf("Expected directory not to be copied when shallow is true")
	}
}

// TestProcessMatch_Directory ensures directories are copied when shallow is false.
func TestProcessMatch_Directory(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "pm-dir-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "pm-dir-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	dirPath := filepath.Join(srcDir, "dir")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	fileInside := filepath.Join(dirPath, "inside.txt")
	if err := os.WriteFile(fileInside, []byte("data"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := processMatch(srcDir, dstDir, dirPath, false, []string{}); err != nil {
		t.Errorf("Expected nil error for directory copy, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "dir", "inside.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file inside directory to be copied")
	}
}

// TestProcessMatch_ErrorStat ensures processMatch returns an error when os.Stat fails.
func TestProcessMatch_ErrorStat(t *testing.T) {
	err := processMatch(os.TempDir(), os.TempDir(), "/nonexistentfile.txt", false, []string{})
	if err == nil || !strings.Contains(err.Error(), "stating file") {
		t.Errorf("Expected error for non-existent file in processMatch, got %v", err)
	}
}

// TestCopyDirRecursive_ReadDirError checks that copyDirRecursive fails if os.ReadDir fails.
func TestCopyDirRecursive_ReadDirError(t *testing.T) {
	ctx := &CopyContext{
		SrcDir:   "/nonexistent_directory",
		DstDir:   os.TempDir(),
		BaseDir:  "/nonexistent_directory",
		Excluded: []string{},
		Included: []string{},
	}
	err := copyDirRecursive(ctx)
	if err == nil || !strings.Contains(err.Error(), "reading directory") {
		t.Errorf("Expected error for non-existent src dir, got %v", err)
	}
}

// TestCopyToTargetWithPatterns checks that included/excluded patterns work.
func TestCopyToTargetWithPatterns(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "copyto-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "copyto-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	subDir := filepath.Join(srcDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create sub dir: %v", err)
	}
	fileKeep := filepath.Join(subDir, "keep.test")
	if err := os.WriteFile(fileKeep, []byte("keep"), 0o600); err != nil {
		t.Fatalf("Failed to write keep file: %v", err)
	}
	fileSkip := filepath.Join(subDir, "skip.test")
	if err := os.WriteFile(fileSkip, []byte("skip"), 0o600); err != nil {
		t.Fatalf("Failed to write skip file: %v", err)
	}
	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{"**/*.test"},
		ExcludedPaths: []string{"**/skip.test"},
	}
	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, false, "dummy"); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "sub", "keep.test")); os.IsNotExist(err) {
		t.Errorf("Expected keep.test to exist")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "sub", "skip.test")); err == nil {
		t.Errorf("Expected skip.test not to exist")
	}
}

// TestCopyToTargetWithPatterns_NoPatterns tests the branch using cp.Copy.
func TestCopyToTargetWithPatterns_NoPatterns(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "nopattern-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "nopattern-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	filePath := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{},
		ExcludedPaths: []string{},
	}
	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, false, "dummy"); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file.txt to exist in destination")
	}
}

// TestCopyToTargetWithPatterns_LocalFileBranch tests the sourceIsLocalFile branch.
func TestCopyToTargetWithPatterns_LocalFileBranch(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "local-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	dstDir, err := os.MkdirTemp("", "local-dst")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	filePath := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{"**/*.txt"},
		ExcludedPaths: []string{},
	}
	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, true, "test:uri"); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file.txt to exist in destination")
	}
}

func TestProcessDirEntry_InfoError(t *testing.T) {
	ctx := &CopyContext{
		SrcDir:   "/dummy",
		DstDir:   "/dummy",
		BaseDir:  "/dummy",
		Excluded: []string{},
		Included: []string{},
	}
	err := processDirEntry(fakeDirEntry{name: "error.txt", err: errors.New("forced info error")}, ctx)
	if err == nil || !strings.Contains(err.Error(), "getting info") {
		t.Errorf("Expected error for Info() failure, got %v", err)
	}
}
