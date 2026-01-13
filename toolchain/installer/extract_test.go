package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabriel-vasile/mimetype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/toolchain/registry"
)

func TestIsGzipMime(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		expected bool
	}{
		{
			name:     "application/x-gzip",
			mimeType: "application/x-gzip",
			expected: true,
		},
		{
			name:     "application/gzip",
			mimeType: "application/gzip",
			expected: true,
		},
		{
			name:     "application/zip",
			mimeType: "application/zip",
			expected: false,
		},
		{
			name:     "application/octet-stream",
			mimeType: "application/octet-stream",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp file to detect its mime type.
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test")

			// Write content that will be detected as the appropriate type.
			var content []byte
			switch tt.mimeType {
			case "application/x-gzip", "application/gzip":
				// Create actual gzip content.
				f, err := os.Create(tmpFile)
				require.NoError(t, err)
				gw := gzip.NewWriter(f)
				_, err = gw.Write([]byte("test content"))
				require.NoError(t, err)
				require.NoError(t, gw.Close())
				require.NoError(t, f.Close())
			default:
				content = []byte("test content")
				require.NoError(t, os.WriteFile(tmpFile, content, 0o644))
			}

			mime, err := mimetype.DetectFile(tmpFile)
			require.NoError(t, err)

			// Verify isGzipMime returns expected result for all cases.
			result := isGzipMime(mime)
			assert.Equal(t, tt.expected, result, "isGzipMime(%s) should return %v", tt.name, tt.expected)
		})
	}
}

func TestIsBinaryMime(t *testing.T) {
	t.Run("octet-stream is binary", func(t *testing.T) {
		// Create a file with random binary content that will be detected as octet-stream.
		// Using bytes that don't match any known magic bytes.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "binary")
		// Random binary data that doesn't match known file signatures.
		binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
		require.NoError(t, os.WriteFile(tmpFile, binaryData, 0o755))

		mime, err := mimetype.DetectFile(tmpFile)
		require.NoError(t, err)

		// Verify this is detected as octet-stream (unknown binary).
		assert.Equal(t, "application/octet-stream", mime.String(), "Random binary should be octet-stream")

		// Verify isBinaryMime returns true for octet-stream.
		result := isBinaryMime(mime)
		assert.True(t, result, "octet-stream should be detected as binary")
	})

	t.Run("zip is not binary", func(t *testing.T) {
		// Create a valid zip file.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.zip")
		createTestZipArchive(t, tmpFile, map[string]string{"file.txt": "content"})

		mime, err := mimetype.DetectFile(tmpFile)
		require.NoError(t, err)

		// Verify zip is not treated as binary.
		result := isBinaryMime(mime)
		assert.False(t, result, "Zip file should not be detected as binary, got MIME: %s", mime.String())
	})

	t.Run("gzip is not binary", func(t *testing.T) {
		// Create a gzip file.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.gz")
		f, err := os.Create(tmpFile)
		require.NoError(t, err)
		gw := gzip.NewWriter(f)
		_, err = gw.Write([]byte("test content"))
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		require.NoError(t, f.Close())

		mime, err := mimetype.DetectFile(tmpFile)
		require.NoError(t, err)

		// Verify gzip is not treated as binary.
		result := isBinaryMime(mime)
		assert.False(t, result, "Gzip file should not be detected as binary, got MIME: %s", mime.String())
	})

	t.Run("text is not binary", func(t *testing.T) {
		// Create a plain text file.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(tmpFile, []byte("hello world"), 0o644))

		mime, err := mimetype.DetectFile(tmpFile)
		require.NoError(t, err)

		// Verify text is not treated as binary.
		result := isBinaryMime(mime)
		assert.False(t, result, "Text file should not be detected as binary, got MIME: %s", mime.String())
	})
}

func TestIsTarGzFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{
			name:     "tar.gz extension",
			filename: "file.tar.gz",
			expected: true,
		},
		{
			name:     "tgz extension",
			filename: "file.tgz",
			expected: true,
		},
		{
			name:     "gz extension only",
			filename: "file.gz",
			expected: false,
		},
		{
			name:     "zip extension",
			filename: "file.zip",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file to get a valid mime type.
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test")
			require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0o644))

			mime, err := mimetype.DetectFile(tmpFile)
			require.NoError(t, err)

			result := isTarGzFile(tt.filename, mime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveBinaryName(t *testing.T) {
	tests := []struct {
		name     string
		tool     *registry.Tool
		expected string
	}{
		{
			name: "BinaryName takes precedence",
			tool: &registry.Tool{
				BinaryName: "custom-binary",
				Name:       "tool-name",
				RepoName:   "repo-name",
			},
			expected: "custom-binary",
		},
		{
			name: "Name as fallback",
			tool: &registry.Tool{
				Name:     "tool-name",
				RepoName: "repo-name",
			},
			expected: "tool-name",
		},
		{
			name: "RepoName as last resort",
			tool: &registry.Tool{
				RepoName: "repo-name",
			},
			expected: "repo-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveBinaryName(tt.tool)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		dest        string
		expectError bool
	}{
		{
			name:        "valid path",
			filename:    "subdir/file.txt",
			dest:        "/tmp/extract",
			expectError: false,
		},
		{
			name:        "path traversal attempt",
			filename:    "../../../etc/passwd",
			dest:        "/tmp/extract",
			expectError: true,
		},
		{
			name:        "simple filename",
			filename:    "file.txt",
			dest:        "/tmp/extract",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validatePath(tt.filename, tt.dest)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Use filepath.FromSlash to convert forward slashes to OS-appropriate separators.
				assert.Contains(t, result, filepath.FromSlash(tt.filename))
			}
		})
	}
}

func TestIsSafePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		dest     string
		expected bool
	}{
		{
			name:     "safe path within dest",
			path:     "/tmp/extract/subdir/file",
			dest:     "/tmp/extract",
			expected: true,
		},
		{
			name:     "path outside dest",
			path:     "/etc/passwd",
			dest:     "/tmp/extract",
			expected: false,
		},
		{
			name:     "path traversal",
			path:     "/tmp/extract/../../../etc/passwd",
			dest:     "/tmp/extract",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSafePath(tt.path, tt.dest)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindBinaryInDir(t *testing.T) {
	t.Run("finds binary in root", func(t *testing.T) {
		tmpDir := t.TempDir()
		binaryPath := filepath.Join(tmpDir, "mybinary")
		require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh"), 0o755))

		found, err := findBinaryInDir(tmpDir, "mybinary")
		assert.NoError(t, err)
		assert.Equal(t, binaryPath, found)
	})

	t.Run("finds binary in subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0o755))

		binaryPath := filepath.Join(subDir, "mybinary")
		require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh"), 0o755))

		found, err := findBinaryInDir(tmpDir, "mybinary")
		assert.NoError(t, err)
		assert.Equal(t, binaryPath, found)
	})

	t.Run("finds .exe binary on Windows", func(t *testing.T) {
		tmpDir := t.TempDir()
		binaryPath := filepath.Join(tmpDir, "mybinary.exe")
		require.NoError(t, os.WriteFile(binaryPath, []byte("MZ"), 0o755))

		found, err := findBinaryInDir(tmpDir, "mybinary")
		assert.NoError(t, err)
		assert.Equal(t, binaryPath, found)
	})

	t.Run("returns error when binary not found", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := findBinaryInDir(tmpDir, "nonexistent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})
}

func TestInstallExtractedBinary(t *testing.T) {
	t.Run("moves binary to destination", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "source", "binary")
		dstPath := filepath.Join(tmpDir, "dest", "installed")

		require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0o755))
		require.NoError(t, os.WriteFile(srcPath, []byte("binary content"), 0o755))

		err := installExtractedBinary(srcPath, dstPath)
		assert.NoError(t, err)

		// Verify destination exists.
		_, err = os.Stat(dstPath)
		assert.NoError(t, err)

		// Verify content.
		content, err := os.ReadFile(dstPath)
		assert.NoError(t, err)
		assert.Equal(t, "binary content", string(content))
	})
}

func TestUnzip(t *testing.T) {
	t.Run("extracts zip file", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "test.zip")
		destDir := filepath.Join(tmpDir, "extracted")

		// Create a test zip file.
		createTestZipArchive(t, zipPath, map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
		})

		err := Unzip(zipPath, destDir)
		assert.NoError(t, err)

		// Verify extracted files.
		content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content1", string(content1))

		content2, err := os.ReadFile(filepath.Join(destDir, "file2.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content2", string(content2))
	})

	t.Run("extracts zip with subdirectories", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "test.zip")
		destDir := filepath.Join(tmpDir, "extracted")

		createTestZipArchive(t, zipPath, map[string]string{
			"subdir/file.txt": "nested content",
		})

		err := Unzip(zipPath, destDir)
		assert.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(destDir, "subdir", "file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "nested content", string(content))
	})
}

func TestCopyFileFallback(t *testing.T) {
	t.Run("copies file content", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		require.NoError(t, os.WriteFile(src, []byte("test content"), 0o644))

		err := copyFileFallback(src, dst)
		assert.NoError(t, err)

		content, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, "test content", string(content))

		// Source should still exist.
		_, err = os.Stat(src)
		assert.NoError(t, err)
	})
}

func TestCopyWithLimit(t *testing.T) {
	t.Run("copies content within limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		content := []byte("small content")
		require.NoError(t, os.WriteFile(src, content, 0o644))

		srcFile, err := os.Open(src)
		require.NoError(t, err)
		defer srcFile.Close()

		dstFile, err := os.Create(dst)
		require.NoError(t, err)
		defer dstFile.Close()

		err = copyWithLimit(srcFile, dstFile, "test", 1000)
		assert.NoError(t, err)
	})

	t.Run("returns error when content exceeds limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		content := []byte("this content is longer than the limit")
		require.NoError(t, os.WriteFile(src, content, 0o644))

		srcFile, err := os.Open(src)
		require.NoError(t, err)
		defer srcFile.Close()

		dstFile, err := os.Create(dst)
		require.NoError(t, err)
		defer dstFile.Close()

		err = copyWithLimit(srcFile, dstFile, "test", 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})
}

func TestExtractDir(t *testing.T) {
	t.Run("creates directory with valid mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		targetPath := filepath.Join(tmpDir, "newdir")

		header := &tar.Header{Mode: 0o755}
		err := extractDir(targetPath, header)
		assert.NoError(t, err)

		info, err := os.Stat(targetPath)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("rejects invalid mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		targetPath := filepath.Join(tmpDir, "newdir")

		header := &tar.Header{Mode: -1}
		err := extractDir(targetPath, header)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrFileOperation)
	})
}

// createTestZipArchive creates a zip file with the given files.
func createTestZipArchive(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
}

// createTestTarGzArchive creates a tar.gz file with the given files.
func createTestTarGzArchive(t *testing.T, tarGzPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(tarGzPath)
	require.NoError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err = tw.Write([]byte(content))
		require.NoError(t, err)
	}
}

func TestMoveFile(t *testing.T) {
	t.Run("moves file successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest", "target.txt")

		require.NoError(t, os.WriteFile(src, []byte("content"), 0o644))

		err := MoveFile(src, dst)
		assert.NoError(t, err)

		// Source should not exist.
		_, err = os.Stat(src)
		assert.True(t, os.IsNotExist(err))

		// Destination should exist with correct content.
		content, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, "content", string(content))
	})

	t.Run("creates target directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "deep", "nested", "dir", "target.txt")

		require.NoError(t, os.WriteFile(src, []byte("content"), 0o644))

		err := MoveFile(src, dst)
		assert.NoError(t, err)

		// Verify destination exists.
		_, err = os.Stat(dst)
		assert.NoError(t, err)
	})
}

func TestExtractTarGz_Function(t *testing.T) {
	t.Run("extracts tar.gz file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarGzPath := filepath.Join(tmpDir, "test.tar.gz")
		destDir := filepath.Join(tmpDir, "extracted")

		createTestTarGzArchive(t, tarGzPath, map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
		})

		err := ExtractTarGz(tarGzPath, destDir)
		assert.NoError(t, err)

		// Verify extracted files.
		content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content1", string(content1))

		content2, err := os.ReadFile(filepath.Join(destDir, "file2.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content2", string(content2))
	})

	t.Run("returns error for invalid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidPath := filepath.Join(tmpDir, "nonexistent.tar.gz")
		destDir := filepath.Join(tmpDir, "extracted")

		err := ExtractTarGz(invalidPath, destDir)
		assert.Error(t, err)
	})
}

func TestInstaller_extractZip(t *testing.T) {
	t.Run("extracts zip and finds binary", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "test.zip")
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "mytool")

		// Create zip with binary.
		createTestZipArchive(t, zipPath, map[string]string{
			"mytool": "#!/bin/sh\necho hello",
		})

		installer := &Installer{}
		tool := &registry.Tool{Name: "mytool"}

		err := installer.extractZip(zipPath, binaryPath, tool)
		assert.NoError(t, err)

		// Verify binary was extracted.
		_, err = os.Stat(binaryPath)
		assert.NoError(t, err)
	})

	t.Run("extracts zip with Files config", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "test.zip")
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "primary")

		// Create zip with multiple files.
		createTestZipArchive(t, zipPath, map[string]string{
			"subdir/primary":   "#!/bin/sh\nprimary",
			"subdir/secondary": "#!/bin/sh\nsecondary",
		})

		installer := &Installer{}
		tool := &registry.Tool{
			Files: []registry.File{
				{Name: "primary", Src: "subdir/primary"},
			},
		}

		err := installer.extractZip(zipPath, binaryPath, tool)
		assert.NoError(t, err)

		// Verify primary binary was extracted.
		_, err = os.Stat(binaryPath)
		assert.NoError(t, err)
	})
}

func TestInstaller_extractTarGz(t *testing.T) {
	t.Run("extracts tar.gz and finds binary", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarGzPath := filepath.Join(tmpDir, "test.tar.gz")
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "mytool")

		// Create tar.gz with binary.
		createTestTarGzArchive(t, tarGzPath, map[string]string{
			"mytool": "#!/bin/sh\necho hello",
		})

		installer := &Installer{}
		tool := &registry.Tool{Name: "mytool"}

		err := installer.extractTarGz(tarGzPath, binaryPath, tool)
		assert.NoError(t, err)

		// Verify binary was extracted.
		_, err = os.Stat(binaryPath)
		assert.NoError(t, err)
	})
}

func TestInstaller_extractGzip(t *testing.T) {
	t.Run("extracts gzip-compressed binary", func(t *testing.T) {
		tmpDir := t.TempDir()
		gzPath := filepath.Join(tmpDir, "binary.gz")
		binaryPath := filepath.Join(tmpDir, "binary")

		// Create gzip file.
		f, err := os.Create(gzPath)
		require.NoError(t, err)
		gw := gzip.NewWriter(f)
		_, err = gw.Write([]byte("#!/bin/sh\necho hello"))
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		require.NoError(t, f.Close())

		installer := &Installer{}
		err = installer.extractGzip(gzPath, binaryPath)
		assert.NoError(t, err)

		// Verify binary was extracted.
		content, err := os.ReadFile(binaryPath)
		assert.NoError(t, err)
		assert.Equal(t, "#!/bin/sh\necho hello", string(content))
	})
}

func TestInstaller_copyFile(t *testing.T) {
	t.Run("copies file content", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source")
		dst := filepath.Join(tmpDir, "dest")

		require.NoError(t, os.WriteFile(src, []byte("binary content"), 0o755))

		installer := &Installer{}
		err := installer.copyFile(src, dst)
		assert.NoError(t, err)

		// Verify content was copied.
		content, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, "binary content", string(content))

		// Source should still exist.
		_, err = os.Stat(src)
		assert.NoError(t, err)
	})
}

func TestInstaller_extractByExtension(t *testing.T) {
	t.Run("handles zip extension", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "test.zip")
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "mytool")

		createTestZipArchive(t, zipPath, map[string]string{
			"mytool": "#!/bin/sh\necho hello",
		})

		installer := &Installer{}
		tool := &registry.Tool{Name: "mytool"}

		err := installer.extractByExtension(zipPath, binaryPath, tool)
		assert.NoError(t, err)
	})

	t.Run("handles tar.gz extension", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarGzPath := filepath.Join(tmpDir, "test.tar.gz")
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "mytool")

		createTestTarGzArchive(t, tarGzPath, map[string]string{
			"mytool": "#!/bin/sh\necho hello",
		})

		installer := &Installer{}
		tool := &registry.Tool{Name: "mytool"}

		err := installer.extractByExtension(tarGzPath, binaryPath, tool)
		assert.NoError(t, err)
	})

	t.Run("handles gz extension", func(t *testing.T) {
		tmpDir := t.TempDir()
		gzPath := filepath.Join(tmpDir, "binary.gz")
		binaryPath := filepath.Join(tmpDir, "binary")

		// Create gzip file.
		f, err := os.Create(gzPath)
		require.NoError(t, err)
		gw := gzip.NewWriter(f)
		_, err = gw.Write([]byte("#!/bin/sh\necho hello"))
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		require.NoError(t, f.Close())

		installer := &Installer{}
		tool := &registry.Tool{Name: "binary"}

		err = installer.extractByExtension(gzPath, binaryPath, tool)
		assert.NoError(t, err)
	})

	t.Run("copies unknown extension as binary", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "binary.unknown")
		dst := filepath.Join(tmpDir, "binary")

		require.NoError(t, os.WriteFile(src, []byte("binary content"), 0o755))

		installer := &Installer{}
		tool := &registry.Tool{Name: "binary"}

		err := installer.extractByExtension(src, dst, tool)
		assert.NoError(t, err)

		content, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, "binary content", string(content))
	})
}

func TestInstaller_simpleExtract(t *testing.T) {
	t.Run("extracts zip by magic bytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipPath := filepath.Join(tmpDir, "test.data") // No .zip extension.
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "mytool")

		createTestZipArchive(t, zipPath, map[string]string{
			"mytool": "#!/bin/sh\necho hello",
		})

		installer := &Installer{}
		tool := &registry.Tool{Name: "mytool"}

		err := installer.simpleExtract(zipPath, binaryPath, tool)
		assert.NoError(t, err)

		// Verify binary was extracted.
		_, err = os.Stat(binaryPath)
		assert.NoError(t, err)
	})

	t.Run("extracts gzip by magic bytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		gzPath := filepath.Join(tmpDir, "binary.data") // No .gz extension.
		binaryPath := filepath.Join(tmpDir, "binary")

		// Create gzip file.
		f, err := os.Create(gzPath)
		require.NoError(t, err)
		gw := gzip.NewWriter(f)
		_, err = gw.Write([]byte("#!/bin/sh\necho hello"))
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		require.NoError(t, f.Close())

		installer := &Installer{}
		tool := &registry.Tool{Name: "binary"}

		err = installer.simpleExtract(gzPath, binaryPath, tool)
		assert.NoError(t, err)

		// Verify binary was extracted.
		content, err := os.ReadFile(binaryPath)
		assert.NoError(t, err)
		assert.Equal(t, "#!/bin/sh\necho hello", string(content))
	})

	t.Run("copies binary by magic bytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "binary.data")
		dst := filepath.Join(tmpDir, "binary")

		// Write binary data (octet-stream).
		binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0x04}
		require.NoError(t, os.WriteFile(src, binaryData, 0o755))

		installer := &Installer{}
		tool := &registry.Tool{Name: "binary"}

		err := installer.simpleExtract(src, dst, tool)
		assert.NoError(t, err)

		content, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, binaryData, content)
	})
}

func TestInstaller_extractFilesFromDir(t *testing.T) {
	t.Run("extracts files using Files config", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "src")
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "primary")

		require.NoError(t, os.MkdirAll(srcDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "primary"), []byte("primary content"), 0o755))

		installer := &Installer{}
		tool := &registry.Tool{
			Files: []registry.File{
				{Name: "primary", Src: "primary"},
			},
		}

		err := installer.extractFilesFromDir(srcDir, binaryPath, tool)
		assert.NoError(t, err)

		content, err := os.ReadFile(binaryPath)
		assert.NoError(t, err)
		assert.Equal(t, "primary content", string(content))
	})

	t.Run("returns error for empty Files", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "primary")

		installer := &Installer{}
		tool := &registry.Tool{
			Files: []registry.File{},
		}

		err := installer.extractFilesFromDir(tmpDir, binaryPath, tool)
		assert.Error(t, err)
	})

	t.Run("returns error for missing source file", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		binaryPath := filepath.Join(binDir, "primary")

		installer := &Installer{}
		tool := &registry.Tool{
			Files: []registry.File{
				{Name: "primary", Src: "nonexistent"},
			},
		}

		err := installer.extractFilesFromDir(tmpDir, binaryPath, tool)
		assert.Error(t, err)
	})
}
