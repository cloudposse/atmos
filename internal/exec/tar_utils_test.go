package exec

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-create-dir-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		dirPath string
		wantErr bool
	}{
		{
			name:    "create new directory",
			dirPath: filepath.Join(tmpDir, "newdir"),
			wantErr: false,
		},
		{
			name:    "create nested directory",
			dirPath: filepath.Join(tmpDir, "parent", "child"),
			wantErr: false,
		},
		{
			name:    "directory already exists",
			dirPath: tmpDir,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := createDirectory(tt.dirPath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify directory was created
				info, err := os.Stat(tt.dirPath)
				assert.NoError(t, err)
				assert.True(t, info.IsDir())
			}
		})
	}
}

func TestExtractTarball(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-extract-tar-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a simple tar archive in memory
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Add a file to the archive
	fileContent := []byte("test content")
	hdr := &tar.Header{
		Name: "testfile.txt",
		Mode: 0o644,
		Size: int64(len(fileContent)),
	}
	err = tw.WriteHeader(hdr)
	require.NoError(t, err)
	_, err = tw.Write(fileContent)
	require.NoError(t, err)

	// Add a directory to the archive
	dirHdr := &tar.Header{
		Name:     "testdir/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}
	err = tw.WriteHeader(dirHdr)
	require.NoError(t, err)

	err = tw.Close()
	require.NoError(t, err)

	// Extract the tarball
	err = extractTarball(&buf, tmpDir)
	require.NoError(t, err)

	// Verify file was extracted
	extractedFile := filepath.Join(tmpDir, "testfile.txt")
	content, err := os.ReadFile(extractedFile)
	require.NoError(t, err)
	assert.Equal(t, fileContent, content)

	// Verify directory was created
	extractedDir := filepath.Join(tmpDir, "testdir")
	info, err := os.Stat(extractedDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestProcessTarHeader(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-process-tar-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		header  *tar.Header
		content []byte
		wantErr bool
		errType error
	}{
		{
			name: "regular file",
			header: &tar.Header{
				Name:     "file.txt",
				Mode:     0o644,
				Typeflag: tar.TypeReg,
				Size:     10,
			},
			content: []byte("test data!"),
			wantErr: false,
		},
		{
			name: "directory",
			header: &tar.Header{
				Name:     "mydir/",
				Mode:     0o755,
				Typeflag: tar.TypeDir,
			},
			wantErr: false,
		},
		{
			name: "path traversal attempt with ..",
			header: &tar.Header{
				Name:     "../../../etc/passwd",
				Mode:     0o644,
				Typeflag: tar.TypeReg,
				Size:     4,
			},
			content: []byte("test"),
			wantErr: true,
			errType: ErrInvalidFilePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader io.Reader
			if tt.content != nil {
				reader = bytes.NewReader(tt.content)
			} else {
				reader = bytes.NewReader([]byte{})
			}

			tarReader := tar.NewReader(reader)
			err := processTarHeader(tt.header, tarReader, tmpDir)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUntar(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-untar-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name       string
		setupTar   func() io.Reader
		wantErr    bool
		validateFn func(t *testing.T, extractPath string)
	}{
		{
			name: "valid tar with file",
			setupTar: func() io.Reader {
				var buf bytes.Buffer
				tw := tar.NewWriter(&buf)
				content := []byte("hello world")
				hdr := &tar.Header{
					Name: "hello.txt",
					Mode: 0o644,
					Size: int64(len(content)),
				}
				tw.WriteHeader(hdr)
				tw.Write(content)
				tw.Close()
				return &buf
			},
			wantErr: false,
			validateFn: func(t *testing.T, extractPath string) {
				content, err := os.ReadFile(filepath.Join(extractPath, "hello.txt"))
				require.NoError(t, err)
				assert.Equal(t, "hello world", string(content))
			},
		},
		{
			name: "tar with directory traversal attempt",
			setupTar: func() io.Reader {
				var buf bytes.Buffer
				tw := tar.NewWriter(&buf)
				// This should be skipped due to ".." check
				hdr := &tar.Header{
					Name: "../badfile.txt",
					Mode: 0o644,
					Size: 4,
				}
				tw.WriteHeader(hdr)
				tw.Write([]byte("bad!"))
				tw.Close()
				return &buf
			},
			wantErr: false, // Should not error, just skip the file
			validateFn: func(t *testing.T, extractPath string) {
				// Verify the bad file was NOT created
				badPath := filepath.Join(extractPath, "..", "badfile.txt")
				_, err := os.Stat(badPath)
				assert.True(t, os.IsNotExist(err))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join(tmpDir, tt.name)
			err := os.MkdirAll(testDir, 0o755)
			require.NoError(t, err)

			reader := tt.setupTar()
			err = untar(reader, testDir)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFn != nil {
					tt.validateFn(t, testDir)
				}
			}
		})
	}
}
