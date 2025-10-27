package exec

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	errSimulatedChmodFailure  = errors.New("simulated chmod failure")
	errSimulatedGlobError     = errors.New("simulated glob error")
	errForcedInfoError        = errors.New("forced info error")
	errSimulatedMkdirAllError = errors.New("simulated MkdirAll error")
	errSimulatedRelPathError  = errors.New("simulated relative path error")
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
	srcDir := t.TempDir()
	dstDir := t.TempDir()
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
	if err == nil || !errors.Is(err, errUtils.ErrOpenFile) {
		t.Errorf("Expected ErrOpenFile for non-existent source file, got %v", err)
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

// Unix-specific test moved to copy_glob_unix_test.go:
// - TestShouldExcludePath_Directory

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
	baseDir := t.TempDir()
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
	srcDir := t.TempDir()
	dstDir := t.TempDir()
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

// Unix-specific test moved to copy_glob_unix_test.go:
// - TestProcessDirEntry_Symlink

// TestGetMatchesForPattern checks that getMatchesForPattern returns expected matches.
func TestGetMatchesForPattern(t *testing.T) {
	srcDir := t.TempDir()
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
	srcDir := t.TempDir()
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
	srcDir := t.TempDir()
	_, err := getMatchesForPattern(srcDir, "[")
	if err == nil {
		t.Errorf("Expected error for invalid pattern, got nil")
	}
}

// Unix-specific test moved to copy_glob_unix_test.go:
// - TestGetMatchesForPattern_ShallowNoMatch

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
	srcDir := t.TempDir()
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
	srcDir := t.TempDir()
	dstDir := t.TempDir()
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
	srcDir := t.TempDir()
	dstDir := t.TempDir()
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
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	if err := processIncludedPattern(srcDir, dstDir, "[", []string{}); err != nil {
		t.Fatalf("Expected processIncludedPattern to handle invalid pattern gracefully, got: %v", err)
	}
}

// TestProcessMatch_ShallowDirectory ensures directories are not copied when shallow is true.
func TestProcessMatch_ShallowDirectory(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
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
	srcDir := t.TempDir()
	dstDir := t.TempDir()
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
	if err == nil || !errors.Is(err, errUtils.ErrStatFile) {
		t.Errorf("Expected ErrStatFile for non-existent file in processMatch, got %v", err)
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
	if err == nil || !errors.Is(err, errUtils.ErrReadDirectory) {
		t.Errorf("Expected ErrReadDirectory for non-existent src dir, got %v", err)
	}
}

// TestCopyToTargetWithPatterns checks that included/excluded patterns work.
func TestCopyToTargetWithPatterns(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
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
	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, false); err != nil {
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
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	filePath := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{},
		ExcludedPaths: []string{},
	}
	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, false); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file.txt to exist in destination")
	}
}

// TestCopyToTargetWithPatterns_LocalFileBranch tests the sourceIsLocalFile branch.
func TestCopyToTargetWithPatterns_LocalFileBranch(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	filePath := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Pass a target path with a file extension to trigger file-to-file copy.
	targetFile := filepath.Join(dstDir, "file.txt")

	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{"**/*.txt"},
		ExcludedPaths: []string{},
	}
	if err := copyToTargetWithPatterns(srcDir, targetFile, dummy, true); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		t.Errorf("Expected %q to exist in destination", targetFile)
	}
}

// TestProcessDirEntry_InfoError tests error handling in processDirEntry when Info() fails.
func TestProcessDirEntry_InfoError(t *testing.T) {
	ctx := &CopyContext{
		SrcDir:   "/dummy",
		DstDir:   "/dummy",
		BaseDir:  "/dummy",
		Excluded: []string{},
		Included: []string{},
	}
	err := processDirEntry(fakeDirEntry{name: "error.txt", err: errForcedInfoError}, ctx)
	if err == nil || !errors.Is(err, errUtils.ErrStatFile) {
		t.Errorf("Expected ErrStatFile for Info() failure, got %v", err)
	}
}

// TestCopyFile_FailCreateDir simulates failure when creating the destination directory.
func TestCopyFile_FailCreateDir(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	content := "copyFileTest"
	if err := os.WriteFile(srcFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	tmpFile, err := os.CreateTemp("", "non-dir")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	dstFile := filepath.Join(tmpFile.Name(), "test.txt")
	err = copyFile(srcFile, dstFile)
	if err == nil || !errors.Is(err, errUtils.ErrCreateDirectory) {
		t.Errorf("Expected ErrCreateDirectory when creating destination directory fails, got %v", err)
	}
}

// TestCopyFile_FailChmod simulates failure when setting file permissions.
// If the patch doesn't take effect, the test will be skipped.
func TestCopyFile_FailChmod(t *testing.T) {
	patches := gomonkey.ApplyFunc(os.Chmod, func(name string, mode os.FileMode) error {
		return errSimulatedChmodFailure
	})
	defer patches.Reset()

	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	content := "copyFileTest"
	if err := os.WriteFile(srcFile, []byte(content), 0o600); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	dstFile := filepath.Join(dstDir, "test.txt")
	err := copyFile(srcFile, dstFile)
	if err == nil {
		t.Skipf("Skipping test: os.Chmod not effective on this platform")
	}
	if !errors.Is(err, errUtils.ErrSetPermissions) {
		t.Errorf("Expected ErrSetPermissions for chmod failure, got %v", err)
	}
}

// TestGetMatchesForPattern_GlobError forces u.GetGlobMatches to return an error.
func TestGetMatchesForPattern_GlobError(t *testing.T) {
	if runtime.GOARCH == "arm64" {
		t.Skip("Skipping gomonkey test on ARM64 due to memory protection issues")
	}

	patches := gomonkey.ApplyFunc(u.GetGlobMatches, func(pattern string) ([]string, error) {
		return nil, errSimulatedGlobError
	})
	defer patches.Reset()

	srcDir := "/dummy/src"
	pattern := "*.txt"
	_, err := getMatchesForPattern(srcDir, pattern)
	if err == nil || !strings.Contains(err.Error(), "simulated glob error") {
		t.Errorf("Expected simulated glob error, got %v", err)
	}
}

// TestInclusionExclusion_TrailingSlash tests the trailing-slash branch in shouldExcludePath for directories.
func TestInclusionExclusion_TrailingSlash(t *testing.T) {
	tmpDir := t.TempDir()

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat temporary directory: %v", err)
	}

	relPath := filepath.Base(tmpDir)

	// Test that the directory is excluded when the exclusion pattern expects a trailing slash.
	if !shouldExcludePath(info, relPath, []string{relPath + "/"}) {
		t.Errorf("Expected directory %q to be excluded by pattern %q", relPath, relPath+"/")
	}

	// Test that the directory is not excluded when the pattern does not match.
	if shouldExcludePath(info, relPath, []string{relPath + "x/"}) {
		t.Errorf("Did not expect directory %q to be excluded by pattern %q", relPath, relPath+"x/")
	}
}

// TestProcessPrefixEntry_InfoError simulates an error when calling Info() in processPrefixEntry.
func TestProcessPrefixEntry_InfoError(t *testing.T) {
	ctx := &PrefixCopyContext{
		SrcDir:     "dummySrc",
		DstDir:     "dummyDst",
		GlobalBase: "dummyGlobal",
		Prefix:     "dummyPrefix",
		Excluded:   []string{},
	}
	fakeEntry := fakeDirEntry{
		name: "error.txt",
		err:  errForcedInfoError,
	}
	err := processPrefixEntry(fakeEntry, ctx)
	if err == nil || !errors.Is(err, errUtils.ErrStatFile) {
		t.Errorf("Expected ErrStatFile when getting info fails, got %v", err)
	}
}

// fakeFileInfo is a minimal implementation of os.FileInfo for testing.
type fakeFileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
}

func (fi fakeFileInfo) Name() string       { return fi.name }
func (fi fakeFileInfo) Size() int64        { return fi.size }
func (fi fakeFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fi fakeFileInfo) IsDir() bool        { return fi.isDir }
func (fi fakeFileInfo) Sys() any           { return nil }

// fakeDirEntryWithInfo implements os.DirEntry using fakeFileInfo.
type fakeDirEntryWithInfo struct {
	name string
	info os.FileInfo
}

func (fde fakeDirEntryWithInfo) Name() string               { return fde.name }
func (fde fakeDirEntryWithInfo) IsDir() bool                { return fde.info.IsDir() }
func (fde fakeDirEntryWithInfo) Type() os.FileMode          { return fde.info.Mode() }
func (fde fakeDirEntryWithInfo) Info() (os.FileInfo, error) { return fde.info, nil }

// TestProcessPrefixEntry_FailMkdir simulates an error when creating a directory in processPrefixEntry.
func TestProcessPrefixEntry_FailMkdir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a fake directory entry.
	fi := fakeFileInfo{
		name:  "testDir",
		mode:  0o755,
		isDir: true,
	}
	fakeEntry := fakeDirEntryWithInfo{
		name: "testDir",
		info: fi,
	}

	ctx := &PrefixCopyContext{
		SrcDir:     srcDir,
		DstDir:     dstDir,
		GlobalBase: srcDir,
		Prefix:     "prefix",
		Excluded:   []string{},
	}

	patches := gomonkey.ApplyFunc(os.MkdirAll, func(path string, perm os.FileMode) error {
		return errSimulatedMkdirAllError
	})
	defer patches.Reset()

	err := processPrefixEntry(fakeEntry, ctx)
	if err == nil || !errors.Is(err, errUtils.ErrCreateDirectory) {
		t.Errorf("Expected ErrCreateDirectory when creating directory fails, got %v", err)
	}
}

// TestCopyToTargetWithPatterns_UseCpCopy ensures that when no inclusion/exclusion patterns are defined, the cp.Copy branch is used.
func TestCopyToTargetWithPatterns_UseCpCopy(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create a test file in the source directory.
	filePath := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Patch the cp.Copy function to verify that it is called.
	called := false
	patch := gomonkey.ApplyFunc(cp.Copy, func(src, dst string) error {
		called = true
		// For testing purposes, simulate a copy by using our copyFile function.
		return copyFile(filepath.Join(src, "file.txt"), filepath.Join(dst, "file.txt"))
	})
	defer patch.Reset()

	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{},
		ExcludedPaths: []string{},
	}
	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, false); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}
	if !called {
		t.Errorf("Expected cp.Copy to be called, but it was not")
	}
	// Verify that the file was "copied" to the destination.
	if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); os.IsNotExist(err) {
		t.Errorf("Expected file.txt to exist in destination")
	}
}

// TestGetMatchesForPattern_ShallowNoMatches tests a shallow pattern (ending with "/*" but not "/**")
// when no matches are found, expecting an empty result.
func TestGetMatchesForPattern_ShallowNoMatches(t *testing.T) {
	patches := gomonkey.ApplyFunc(u.GetGlobMatches, func(pattern string) ([]string, error) {
		return []string{}, nil
	})
	defer patches.Reset()

	srcDir := "/dummy/src"
	pattern := "dummy/*" // Shallow pattern without recursive "**"
	matches, err := getMatchesForPattern(srcDir, pattern)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("Expected no matches for shallow pattern, got %v", matches)
	}
}

// TestProcessMatch_RelPathError simulates an error in computing the relative path in processMatch.
func TestProcessMatch_RelPathError(t *testing.T) {
	srcDir := "/dummy/src"
	dstPath := "/dummy/dst"

	// Create a temporary file to act as the target file.
	tmpFile, err := os.CreateTemp("", "relerr")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	filePath := tmpFile.Name()

	patches := gomonkey.ApplyFunc(filepath.Rel, func(basepath, targpath string) (string, error) {
		return "", errSimulatedRelPathError
	})
	defer patches.Reset()

	err = processMatch(srcDir, dstPath, filePath, false, []string{})
	if err == nil || !errors.Is(err, errUtils.ErrComputeRelativePath) {
		t.Errorf("Expected ErrComputeRelativePath, got %v", err)
	}
}

// Unix-specific test moved to copy_glob_unix_test.go:
// - TestCopyFile_FailCreate

// TestShouldIncludePath_NoPatterns tests that files are included when no patterns specified.
func TestShouldIncludePath_NoPatterns(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// No patterns means include everything.
	if !shouldIncludePath(info, "docs/readme.txt", []string{}) {
		t.Errorf("Expected path to be included when no patterns specified")
	}
}

// TestShouldIncludePath_Directory tests that directories are always included.
func TestShouldIncludePath_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat dir: %v", err)
	}

	// Directories are always included regardless of patterns.
	if !shouldIncludePath(info, filepath.Base(tmpDir), []string{"**/*.txt"}) {
		t.Errorf("Expected directory to be included")
	}
}

// TestShouldIncludePath_NoMatch tests exclusion when file doesn't match any pattern.
func TestShouldIncludePath_NoMatch(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// File doesn't match the pattern, should be excluded.
	if shouldIncludePath(info, "app/test.log", []string{"**/*.txt"}) {
		t.Errorf("Expected path not to be included when it doesn't match pattern")
	}
}

// TestShouldSkipPrefixEntry_DirectoryWithTrailingSlash tests directory exclusion in prefix mode.
func TestShouldSkipPrefixEntry_DirectoryWithTrailingSlash(t *testing.T) {
	tmpDir := t.TempDir()

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat dir: %v", err)
	}

	dirName := filepath.Base(tmpDir)
	// Directory should be excluded when pattern has trailing slash.
	if !shouldSkipPrefixEntry(info, dirName, []string{dirName + "/"}) {
		t.Errorf("Expected directory to be excluded with trailing slash pattern")
	}
}

// Unix-specific test moved to copy_glob_unix_test.go:
// - TestShouldSkipPrefixEntry_File

// TestShouldSkipPrefixEntry_NoExclusion tests that files are not excluded without patterns.
func TestShouldSkipPrefixEntry_NoExclusion(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// No patterns means nothing is excluded.
	if shouldSkipPrefixEntry(info, "test.txt", []string{}) {
		t.Errorf("Expected file not to be excluded with no patterns")
	}
}

// TestGetMatchesForPattern_RecursiveNoMatch tests recursive pattern with no matches.
func TestGetMatchesForPattern_RecursiveNoMatch(t *testing.T) {
	patches := gomonkey.ApplyFunc(u.GetGlobMatches, func(pattern string) ([]string, error) {
		return []string{}, nil
	})
	defer patches.Reset()

	srcDir := "/dummy/src"
	pattern := "dir/*/**"
	matches, err := getMatchesForPattern(srcDir, pattern)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("Expected no matches for recursive pattern, got %v", matches)
	}
}

// TestGetLocalFinalTarget_Directory tests target is a directory without extension.
func TestGetLocalFinalTarget_Directory(t *testing.T) {
	srcDir := t.TempDir()

	targetPath := t.TempDir()

	finalTarget, err := getLocalFinalTarget(srcDir, targetPath)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	expectedPath := filepath.Join(targetPath, SanitizeFileName(filepath.Base(srcDir)))
	if finalTarget != expectedPath {
		t.Errorf("Expected %q, got %q", expectedPath, finalTarget)
	}
}

// TestGetLocalFinalTarget_FileExtension tests target with file extension.
func TestGetLocalFinalTarget_FileExtension(t *testing.T) {
	srcDir := t.TempDir()

	tmpDir := t.TempDir()

	targetPath := filepath.Join(tmpDir, "output.txt")
	finalTarget, err := getLocalFinalTarget(srcDir, targetPath)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if finalTarget != targetPath {
		t.Errorf("Expected %q, got %q", targetPath, finalTarget)
	}
}

// TestGetNonLocalFinalTarget tests non-local file target creation.
func TestGetNonLocalFinalTarget(t *testing.T) {
	tmpDir := t.TempDir()

	targetPath := filepath.Join(tmpDir, "newdir")
	finalTarget, err := getNonLocalFinalTarget(targetPath)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if finalTarget != targetPath {
		t.Errorf("Expected %q, got %q", targetPath, finalTarget)
	}
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Errorf("Expected directory to be created")
	}
}

// TestComponentOrMixinsCopy_FileToFile tests file-to-file copy with existing directory at dest.
func TestComponentOrMixinsCopy_FileToFile_ExistingDir(t *testing.T) {
	srcDir := t.TempDir()

	dstDir := t.TempDir()

	// Create source file.
	srcFile := filepath.Join(srcDir, "source.txt")
	if err := os.WriteFile(srcFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Create a directory at the destination path.
	dstFile := filepath.Join(dstDir, "dest.txt")
	if err := os.Mkdir(dstFile, 0o755); err != nil {
		t.Fatalf("Failed to create directory at dest: %v", err)
	}

	// ComponentOrMixinsCopy should remove the directory and copy the file.
	if err := ComponentOrMixinsCopy(srcFile, dstFile); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify it's now a file, not a directory.
	info, err := os.Stat(dstFile)
	if err != nil {
		t.Errorf("Failed to stat dest: %v", err)
	}
	if info.IsDir() {
		t.Errorf("Expected dest to be a file, not a directory")
	}
}

// TestCopyToTargetWithPatterns_InclusionOnly tests copy with only inclusion patterns.
func TestCopyToTargetWithPatterns_InclusionOnly(t *testing.T) {
	srcDir := t.TempDir()

	dstDir := t.TempDir()

	// Create test files.
	if err := os.WriteFile(filepath.Join(srcDir, "match.md"), []byte("md"), 0o600); err != nil {
		t.Fatalf("Failed to write md file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "skip.txt"), []byte("txt"), 0o600); err != nil {
		t.Fatalf("Failed to write txt file: %v", err)
	}

	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{"**/*.md"},
		ExcludedPaths: []string{}, // No exclusions
	}

	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, false); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}

	// Only .md file should be copied.
	if _, err := os.Stat(filepath.Join(dstDir, "match.md")); os.IsNotExist(err) {
		t.Errorf("Expected match.md to exist")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "skip.txt")); err == nil {
		t.Errorf("Expected skip.txt not to exist")
	}
}
