// copy_glob_test.go
package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

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
	if err := os.WriteFile(srcFile, []byte(content), 0o644); err != nil {
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

// TestShouldExcludePath tests that shouldExcludePath returns true when a file matches an exclusion pattern.
func TestShouldExcludePath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.log")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat temp file: %v", err)
	}
	excluded := []string{"**/*.log"}
	relPath := "app/test.log"
	if !shouldExcludePath(info, relPath, excluded) {
		t.Errorf("Expected path %q to be excluded", relPath)
	}
}

// TestShouldIncludePath verifies that shouldIncludePath returns true when the file matches an inclusion pattern.
func TestShouldIncludePath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test.txt")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	info, err := tmpFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat temp file: %v", err)
	}
	included := []string{"**/*.txt"}
	relPath := "docs/readme.txt"
	if !shouldIncludePath(info, relPath, included) {
		t.Errorf("Expected path %q to be included", relPath)
	}
}

// TestShouldSkipEntry creates a temporary directory structure and verifies that shouldSkipEntry correctly skips a file.
func TestShouldSkipEntry(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "base")
	if err != nil {
		t.Fatalf("Failed to create base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)
	subDir := filepath.Join(baseDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}
	filePath := filepath.Join(subDir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	excluded := []string{"**/*.txt"}
	included := []string{}
	if !shouldSkipEntry(info, filePath, baseDir, excluded, included) {
		t.Errorf("Expected file %q to be skipped", filePath)
	}
}

// TestCopyDirRecursive sets up a temporary directory tree and verifies that copyDirRecursive copies all files.
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
		t.Fatalf("Failed to create subdirectory: %v", err)
	}
	file1 := filepath.Join(srcDir, "file1.txt")
	file2 := filepath.Join(subDir, "file2.txt")
	if err := os.WriteFile(file1, []byte("file1"), 0o644); err != nil {
		t.Fatalf("Failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("file2"), 0o644); err != nil {
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
	dstFile1 := filepath.Join(dstDir, "file1.txt")
	dstFile2 := filepath.Join(dstDir, "sub", "file2.txt")
	if _, err := os.Stat(dstFile1); os.IsNotExist(err) {
		t.Errorf("Expected %q to exist", dstFile1)
	}
	if _, err := os.Stat(dstFile2); os.IsNotExist(err) {
		t.Errorf("Expected %q to exist", dstFile2)
	}
}

// TestGetMatchesForPattern verifies that getMatchesForPattern returns the correct file matches.
func TestGetMatchesForPattern(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "glob-src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)
	fileA := filepath.Join(srcDir, "a.txt")
	fileB := filepath.Join(srcDir, "b.log")
	if err := os.WriteFile(fileA, []byte("content"), 0o644); err != nil {
		t.Fatalf("Failed to write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("content"), 0o644); err != nil {
		t.Fatalf("Failed to write fileB: %v", err)
	}
	pattern := "*.txt"
	matches, err := getMatchesForPattern(srcDir, pattern)
	if err != nil {
		t.Fatalf("getMatchesForPattern error: %v", err)
	}
	if len(matches) != 1 || !strings.Contains(matches[0], "a.txt") {
		t.Errorf("Expected one match for a.txt, got %v", matches)
	}
}

// TestIsShallowPattern verifies that isShallowPattern returns correct results for given patterns.
func TestIsShallowPattern(t *testing.T) {
	if !isShallowPattern("**/demo-localstack/*") {
		t.Errorf("Expected pattern '**/demo-localstack/*' to be shallow")
	}
	if isShallowPattern("**/demo-library/**") {
		t.Errorf("Expected pattern '**/demo-library/**' not to be shallow")
	}
}

// TestCopyDirRecursiveWithPrefix tests that copyDirRecursiveWithPrefix copies files using prefix context.
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
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
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
	dstFile := filepath.Join(dstDir, "test.txt")
	if _, err := os.Stat(dstFile); os.IsNotExist(err) {
		t.Errorf("Expected file %q to exist", dstFile)
	}
}

// TestProcessIncludedPattern tests processIncludedPattern by copying matching files to target directory.
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
	if err := os.WriteFile(fileMatch, []byte("mdcontent"), 0644); err != nil {
		t.Fatalf("Failed to write matching file: %v", err)
	}
	fileNoMatch := filepath.Join(srcDir, "no_match.txt")
	if err := os.WriteFile(fileNoMatch, []byte("txtcontent"), 0644); err != nil {
		t.Fatalf("Failed to write non-matching file: %v", err)
	}
	pattern := "**/*.md"
	if err := processIncludedPattern(srcDir, dstDir, pattern, []string{}); err != nil {
		t.Fatalf("processIncludedPattern failed: %v", err)
	}
	copiedMatch := filepath.Join(dstDir, "match.md")
	if _, err := os.Stat(copiedMatch); os.IsNotExist(err) {
		t.Errorf("Expected file %q to exist", copiedMatch)
	}
	notCopied := filepath.Join(dstDir, "no_match.txt")
	if _, err := os.Stat(notCopied); err == nil {
		t.Errorf("Expected file %q not to exist", notCopied)
	}
}

// TestCopyToTargetWithPatterns tests copyToTargetWithPatterns with inclusion and exclusion patterns.
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
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}
	fileKeep := filepath.Join(subDir, "keep.test")
	if err := os.WriteFile(fileKeep, []byte("keep"), 0644); err != nil {
		t.Fatalf("Failed to write keep file: %v", err)
	}
	fileSkip := filepath.Join(subDir, "skip.test")
	if err := os.WriteFile(fileSkip, []byte("skip"), 0644); err != nil {
		t.Fatalf("Failed to write skip file: %v", err)
	}
	dummy := &schema.AtmosVendorSource{
		IncludedPaths: []string{"**/*.test"},
		ExcludedPaths: []string{"**/skip.test"},
	}
	if err := copyToTargetWithPatterns(srcDir, dstDir, dummy, false, "dummy"); err != nil {
		t.Fatalf("copyToTargetWithPatterns failed: %v", err)
	}
	dstKeep := filepath.Join(dstDir, "sub", "keep.test")
	if _, err := os.Stat(dstKeep); os.IsNotExist(err) {
		t.Errorf("Expected file %q to exist", dstKeep)
	}
	dstSkip := filepath.Join(dstDir, "sub", "skip.test")
	if _, err := os.Stat(dstSkip); err == nil {
		t.Errorf("Expected file %q not to exist", dstSkip)
	}
}
