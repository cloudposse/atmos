package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
)

func TestStore_Name(t *testing.T) {
	store := &Store{}
	assert.Equal(t, "github/artifacts", store.Name())
}

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special chars",
			input:    "simple-key.tfplan",
			expected: "simple-key.tfplan",
		},
		{
			name:     "forward slash",
			input:    "stack/component/sha.tfplan",
			expected: "stack--component--sha.tfplan",
		},
		{
			name:     "backslash",
			input:    "stack\\component\\sha.tfplan",
			expected: "stack--component--sha.tfplan",
		},
		{
			name:     "mixed slashes",
			input:    "stack/component\\sha.tfplan",
			expected: "stack--component--sha.tfplan",
		},
		{
			name:     "multiple consecutive slashes",
			input:    "a//b",
			expected: "a----b",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDesanitizeKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special chars",
			input:    "simple-key.tfplan",
			expected: "simple-key.tfplan",
		},
		{
			name:     "double dash",
			input:    "stack--component--sha.tfplan",
			expected: "stack/component/sha.tfplan",
		},
		{
			name:     "single dash preserved",
			input:    "my-stack--my-component.tfplan",
			expected: "my-stack/my-component.tfplan",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := desanitizeKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeDesanitizeRoundtrip(t *testing.T) {
	keys := []string{
		"stack/component/sha.tfplan",
		"dev/vpc/abc123.tfplan",
		"staging/rds/def456.tfplan",
		"production/eks-cluster/ghi789.tfplan",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			sanitized := sanitizeKey(key)
			desanitized := desanitizeKey(sanitized)
			assert.Equal(t, key, desanitized, "roundtrip failed for key: %s", key)
		})
	}
}

func TestSplitRepoString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "valid owner/repo",
			input:    "owner/repo",
			expected: []string{"owner", "repo"},
		},
		{
			name:     "repo with org",
			input:    "cloudposse/atmos",
			expected: []string{"cloudposse", "atmos"},
		},
		{
			name:     "no slash",
			input:    "noslash",
			expected: []string{"noslash"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "multiple slashes",
			input:    "owner/repo/extra",
			expected: []string{"owner", "repo/extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitRepoString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		prefix   string
		expected bool
	}{
		{
			name:     "has prefix",
			s:        "hello world",
			prefix:   "hello",
			expected: true,
		},
		{
			name:     "no prefix",
			s:        "hello world",
			prefix:   "world",
			expected: false,
		},
		{
			name:     "exact match",
			s:        "hello",
			prefix:   "hello",
			expected: true,
		},
		{
			name:     "empty prefix",
			s:        "hello",
			prefix:   "",
			expected: true,
		},
		{
			name:     "empty string",
			s:        "",
			prefix:   "hello",
			expected: false,
		},
		{
			name:     "both empty",
			s:        "",
			prefix:   "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasPrefix(tt.s, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStore_artifactName(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		store := &Store{prefix: "planfile"}

		tests := []struct {
			name     string
			key      string
			expected string
		}{
			{
				name:     "simple key",
				key:      "test.tfplan",
				expected: "planfile-test.tfplan",
			},
			{
				name:     "key with path",
				key:      "stack/component/sha.tfplan",
				expected: "planfile-stack--component--sha.tfplan",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := store.artifactName(tt.key)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("without prefix", func(t *testing.T) {
		store := &Store{}

		tests := []struct {
			name     string
			key      string
			expected string
		}{
			{
				name:     "simple key",
				key:      "test.tfplan",
				expected: "test.tfplan",
			},
			{
				name:     "key with path",
				key:      "stack/component/sha.tfplan",
				expected: "stack--component--sha.tfplan",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := store.artifactName(tt.key)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestGetRetentionDays(t *testing.T) {
	tests := []struct {
		name     string
		options  map[string]any
		expected int
	}{
		{
			name:     "default when not set",
			options:  map[string]any{},
			expected: defaultRetentionDays,
		},
		{
			name:     "default when nil",
			options:  nil,
			expected: defaultRetentionDays,
		},
		{
			name: "custom value",
			options: map[string]any{
				"retention_days": 30,
			},
			expected: 30,
		},
		{
			name: "zero defaults to default",
			options: map[string]any{
				"retention_days": 0,
			},
			expected: defaultRetentionDays,
		},
		{
			name: "negative defaults to default",
			options: map[string]any{
				"retention_days": -1,
			},
			expected: defaultRetentionDays,
		},
		{
			name: "wrong type defaults to default",
			options: map[string]any{
				"retention_days": "30",
			},
			expected: defaultRetentionDays,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRetentionDays(tt.options)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRepoInfo(t *testing.T) {
	tests := []struct {
		name          string
		options       map[string]any
		envRepo       string
		expectedOwner string
		expectedRepo  string
	}{
		{
			name: "from options",
			options: map[string]any{
				"owner": "myowner",
				"repo":  "myrepo",
			},
			envRepo:       "",
			expectedOwner: "myowner",
			expectedRepo:  "myrepo",
		},
		{
			name:          "from env",
			options:       map[string]any{},
			envRepo:       "envowner/envrepo",
			expectedOwner: "envowner",
			expectedRepo:  "envrepo",
		},
		{
			name: "options override env",
			options: map[string]any{
				"owner": "optowner",
				"repo":  "optrepo",
			},
			envRepo:       "envowner/envrepo",
			expectedOwner: "optowner",
			expectedRepo:  "optrepo",
		},
		{
			name: "partial options with env fallback",
			options: map[string]any{
				"owner": "optowner",
			},
			envRepo:       "envowner/envrepo",
			expectedOwner: "optowner",
			expectedRepo:  "envrepo",
		},
		{
			name:          "empty options and env",
			options:       map[string]any{},
			envRepo:       "",
			expectedOwner: "",
			expectedRepo:  "",
		},
		{
			name:          "invalid env format",
			options:       map[string]any{},
			envRepo:       "noslash",
			expectedOwner: "",
			expectedRepo:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envRepo != "" {
				t.Setenv("GITHUB_REPOSITORY", tt.envRepo)
			} else {
				t.Setenv("GITHUB_REPOSITORY", "")
			}

			owner, repo := getRepoInfo(tt.options)
			assert.Equal(t, tt.expectedOwner, owner)
			assert.Equal(t, tt.expectedRepo, repo)
		})
	}
}

func TestFillRepoFromEnv(t *testing.T) {
	tests := []struct {
		name          string
		owner         string
		repo          string
		envRepo       string
		expectedOwner string
		expectedRepo  string
	}{
		{
			name:          "fills both from env",
			owner:         "",
			repo:          "",
			envRepo:       "envowner/envrepo",
			expectedOwner: "envowner",
			expectedRepo:  "envrepo",
		},
		{
			name:          "keeps existing owner",
			owner:         "myowner",
			repo:          "",
			envRepo:       "envowner/envrepo",
			expectedOwner: "myowner",
			expectedRepo:  "envrepo",
		},
		{
			name:          "keeps existing repo",
			owner:         "",
			repo:          "myrepo",
			envRepo:       "envowner/envrepo",
			expectedOwner: "envowner",
			expectedRepo:  "myrepo",
		},
		{
			name:          "keeps both if set",
			owner:         "myowner",
			repo:          "myrepo",
			envRepo:       "envowner/envrepo",
			expectedOwner: "myowner",
			expectedRepo:  "myrepo",
		},
		{
			name:          "no env set",
			owner:         "",
			repo:          "",
			envRepo:       "",
			expectedOwner: "",
			expectedRepo:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_REPOSITORY", tt.envRepo)

			owner, repo := fillRepoFromEnv(tt.owner, tt.repo)
			assert.Equal(t, tt.expectedOwner, owner)
			assert.Equal(t, tt.expectedRepo, repo)
		})
	}
}

func TestNewStore(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		t.Setenv("ATMOS_CI_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")
		// Override PATH to prevent gh CLI fallback from finding a token.
		t.Setenv("PATH", t.TempDir())

		_, err := NewStore(artifact.StoreOptions{
			Options: map[string]any{
				"owner": "testowner",
				"repo":  "testrepo",
			},
		})
		assert.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
	})

	t.Run("missing owner and repo", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv("GITHUB_REPOSITORY", "")

		_, err := NewStore(artifact.StoreOptions{
			Options: map[string]any{},
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactStoreNotFound)
	})

	t.Run("valid configuration", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")

		store, err := NewStore(artifact.StoreOptions{
			Options: map[string]any{
				"owner":          "testowner",
				"repo":           "testrepo",
				"retention_days": 14,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, store)

		s, ok := store.(*Store)
		require.True(t, ok)
		assert.Equal(t, "testowner", s.owner)
		assert.Equal(t, "testrepo", s.repo)
		assert.Equal(t, 14, s.retentionDays)
		assert.Equal(t, "https://api.github.com", s.baseURL)
		assert.NotNil(t, s.httpClient)
		assert.NotNil(t, s.httpClient.Transport)
	})

	t.Run("with GITHUB_REPOSITORY env", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv("GITHUB_REPOSITORY", "envowner/envrepo")

		store, err := NewStore(artifact.StoreOptions{
			Options: map[string]any{},
		})
		require.NoError(t, err)
		require.NotNil(t, store)

		s, ok := store.(*Store)
		require.True(t, ok)
		assert.Equal(t, "envowner", s.owner)
		assert.Equal(t, "envrepo", s.repo)
	})
}

func TestExtractFromZip(t *testing.T) {
	t.Run("valid zip with archive and metadata", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			archiveFilename: []byte("tar stream content"),
			metadataFilename: func() []byte {
				m := &artifact.Metadata{}
				m.Stack = "test-stack"
				m.Component = "test-component"
				m.SHA = "abc123"
				return marshalJSON(t, m)
			}(),
		})

		reader, metadata, err := extractFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "tar stream content", string(content))

		require.NotNil(t, metadata)
		assert.Equal(t, "test-stack", metadata.Stack)
		assert.Equal(t, "test-component", metadata.Component)
		assert.Equal(t, "abc123", metadata.SHA)
	})

	t.Run("valid zip without metadata", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			archiveFilename: []byte("tar stream content"),
		})

		reader, metadata, err := extractFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "tar stream content", string(content))
		assert.Nil(t, metadata)
	})

	t.Run("zip without archive.tar", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			metadataFilename: []byte("{}"),
		})

		_, _, err := extractFromZip(zipData)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactDownloadFailed)
	})

	t.Run("invalid zip data", func(t *testing.T) {
		_, _, err := extractFromZip([]byte("not a zip"))
		assert.Error(t, err)
	})

	t.Run("zip with invalid metadata JSON", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			archiveFilename:  []byte("tar stream content"),
			metadataFilename: []byte("not valid json"),
		})

		reader, metadata, err := extractFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		// Archive should still be readable.
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "tar stream content", string(content))

		// Metadata should be nil due to JSON parse error.
		assert.Nil(t, metadata)
	})
}

func TestReadZipFile(t *testing.T) {
	zipData := createTestZip(t, map[string][]byte{
		"test.txt": []byte("test content"),
	})

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	require.NoError(t, err)
	require.Len(t, zipReader.File, 1)

	content, err := readZipFile(zipReader.File[0])
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

func TestReadMetadataFile(t *testing.T) {
	t.Run("valid metadata", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			metadataFilename: func() []byte {
				m := &artifact.Metadata{}
				m.Stack = "test"
				m.Component = "comp"
				return marshalJSON(t, m)
			}(),
		})

		zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
		require.NoError(t, err)
		require.Len(t, zipReader.File, 1)

		metadata := readMetadataFile(zipReader.File[0])
		require.NotNil(t, metadata)
		assert.Equal(t, "test", metadata.Stack)
		assert.Equal(t, "comp", metadata.Component)
	})

	t.Run("invalid JSON returns nil", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			metadataFilename: []byte("invalid json"),
		})

		zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
		require.NoError(t, err)
		require.Len(t, zipReader.File, 1)

		metadata := readMetadataFile(zipReader.File[0])
		assert.Nil(t, metadata)
	})
}

// mockUploader implements artifactUploader for testing.
type mockUploader struct {
	createFn   func(ctx context.Context, req *createArtifactRequest) (*createArtifactResponse, error)
	uploadFn   func(ctx context.Context, uploadURL string, data []byte) error
	finalizeFn func(ctx context.Context, req *finalizeArtifactRequest) (*finalizeArtifactResponse, error)
}

func (m *mockUploader) CreateArtifact(ctx context.Context, req *createArtifactRequest) (*createArtifactResponse, error) {
	return m.createFn(ctx, req)
}

func (m *mockUploader) UploadBlob(ctx context.Context, uploadURL string, data []byte) error {
	return m.uploadFn(ctx, uploadURL, data)
}

func (m *mockUploader) FinalizeArtifact(ctx context.Context, req *finalizeArtifactRequest) (*finalizeArtifactResponse, error) {
	return m.finalizeFn(ctx, req)
}

func TestStore_Upload(t *testing.T) {
	t.Run("no uploader returns not implemented", func(t *testing.T) {
		store := &Store{
			owner:         "testowner",
			repo:          "testrepo",
			retentionDays: 7,
			uploader:      nil,
		}

		ctx := context.Background()
		data := strings.NewReader("plan data")
		metadata := &artifact.Metadata{}
		metadata.Stack = "test"
		metadata.Component = "comp"

		err := store.Upload(ctx, "test/key.tfplan", data, int64(data.Len()), metadata)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
	})

	t.Run("successful upload", func(t *testing.T) {
		// Create a fake runtime token with Actions.Results scope.
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-backend-id-123:job-backend-id-456",
		})
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)

		var capturedCreateReq *createArtifactRequest
		var capturedUploadData []byte
		var capturedFinalizeReq *finalizeArtifactRequest

		mock := &mockUploader{
			createFn: func(_ context.Context, req *createArtifactRequest) (*createArtifactResponse, error) {
				capturedCreateReq = req
				return &createArtifactResponse{
					OK:              true,
					SignedUploadURL: "https://blob.example.com/upload?sig=abc",
				}, nil
			},
			uploadFn: func(_ context.Context, _ string, data []byte) error {
				capturedUploadData = data
				return nil
			},
			finalizeFn: func(_ context.Context, req *finalizeArtifactRequest) (*finalizeArtifactResponse, error) {
				capturedFinalizeReq = req
				return &finalizeArtifactResponse{
					OK:         true,
					ArtifactID: 42,
				}, nil
			},
		}

		store := &Store{
			owner:         "testowner",
			repo:          "testrepo",
			prefix:        "planfile",
			retentionDays: 14,
			uploader:      mock,
		}

		ctx := context.Background()
		planContent := []byte("terraform plan binary content")
		metadata := &artifact.Metadata{}
		metadata.Stack = "dev"
		metadata.Component = "vpc"
		metadata.SHA = "abc123"

		err := store.Upload(ctx, "dev/vpc/abc123.tfplan", bytes.NewReader(planContent), int64(len(planContent)), metadata)
		require.NoError(t, err)

		// Verify CreateArtifact request.
		require.NotNil(t, capturedCreateReq)
		assert.Equal(t, "planfile-dev--vpc--abc123.tfplan", capturedCreateReq.Name)
		assert.Equal(t, "run-backend-id-123", capturedCreateReq.WorkflowRunBackendID)
		assert.Equal(t, "job-backend-id-456", capturedCreateReq.WorkflowJobRunBackendID)
		assert.Equal(t, 4, capturedCreateReq.Version)
		assert.Equal(t, "14d", capturedCreateReq.ExpiresAfter)

		// Verify uploaded data is a valid zip containing archive.tar and metadata.
		require.NotEmpty(t, capturedUploadData)
		reader, meta, err := extractFromZip(capturedUploadData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		planBytes, err := io.ReadAll(reader)
		require.NoError(t, err)
		// The archive should contain the exact bytes passed to Upload.
		assert.Equal(t, planContent, planBytes)
		require.NotNil(t, meta)
		assert.Equal(t, "dev", meta.Stack)
		assert.Equal(t, "vpc", meta.Component)

		// Verify FinalizeArtifact request.
		require.NotNil(t, capturedFinalizeReq)
		assert.Equal(t, "planfile-dev--vpc--abc123.tfplan", capturedFinalizeReq.Name)
		assert.Equal(t, int64(len(capturedUploadData)), capturedFinalizeReq.Size)
		assert.Equal(t, "run-backend-id-123", capturedFinalizeReq.WorkflowRunBackendID)
		assert.Equal(t, "job-backend-id-456", capturedFinalizeReq.WorkflowJobRunBackendID)
	})

	t.Run("upload without metadata", func(t *testing.T) {
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-id:job-id",
		})
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)

		var capturedUploadData []byte
		mock := &mockUploader{
			createFn: func(_ context.Context, _ *createArtifactRequest) (*createArtifactResponse, error) {
				return &createArtifactResponse{
					OK:              true,
					SignedUploadURL: "https://blob.example.com/upload",
				}, nil
			},
			uploadFn: func(_ context.Context, _ string, data []byte) error {
				capturedUploadData = data
				return nil
			},
			finalizeFn: func(_ context.Context, _ *finalizeArtifactRequest) (*finalizeArtifactResponse, error) {
				return &finalizeArtifactResponse{OK: true, ArtifactID: 1}, nil
			},
		}

		store := &Store{
			owner:         "testowner",
			repo:          "testrepo",
			retentionDays: 7,
			uploader:      mock,
		}

		ctx := context.Background()
		err := store.Upload(ctx, "test/key.tfplan", strings.NewReader("plan"), 4, nil)
		require.NoError(t, err)

		// Verify the zip contains the archive data and no metadata sidecar.
		reader, meta, err := extractFromZip(capturedUploadData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "plan", string(content))
		// When nil metadata is passed, the store does not generate metadata.
		assert.Nil(t, meta)
	})

	t.Run("create artifact fails", func(t *testing.T) {
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-id:job-id",
		})
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)

		mock := &mockUploader{
			createFn: func(_ context.Context, _ *createArtifactRequest) (*createArtifactResponse, error) {
				return nil, fmt.Errorf("service unavailable")
			},
			uploadFn:   nil,
			finalizeFn: nil,
		}

		store := &Store{
			owner:    "testowner",
			repo:     "testrepo",
			uploader: mock,
		}

		ctx := context.Background()
		err := store.Upload(ctx, "test/key.tfplan", strings.NewReader("plan"), 4, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
		assert.Contains(t, err.Error(), "service unavailable")
	})

	t.Run("empty upload URL", func(t *testing.T) {
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-id:job-id",
		})
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)

		mock := &mockUploader{
			createFn: func(_ context.Context, _ *createArtifactRequest) (*createArtifactResponse, error) {
				return &createArtifactResponse{OK: true, SignedUploadURL: ""}, nil
			},
			uploadFn:   nil,
			finalizeFn: nil,
		}

		store := &Store{
			owner:    "testowner",
			repo:     "testrepo",
			uploader: mock,
		}

		ctx := context.Background()
		err := store.Upload(ctx, "test/key.tfplan", strings.NewReader("plan"), 4, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
		assert.Contains(t, err.Error(), "empty upload URL")
	})

	t.Run("blob upload fails", func(t *testing.T) {
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-id:job-id",
		})
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)

		mock := &mockUploader{
			createFn: func(_ context.Context, _ *createArtifactRequest) (*createArtifactResponse, error) {
				return &createArtifactResponse{OK: true, SignedUploadURL: "https://blob.example.com/upload"}, nil
			},
			uploadFn: func(_ context.Context, _ string, _ []byte) error {
				return fmt.Errorf("blob storage error")
			},
			finalizeFn: nil,
		}

		store := &Store{
			owner:    "testowner",
			repo:     "testrepo",
			uploader: mock,
		}

		ctx := context.Background()
		err := store.Upload(ctx, "test/key.tfplan", strings.NewReader("plan"), 4, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
		assert.Contains(t, err.Error(), "blob storage error")
	})

	t.Run("finalize fails", func(t *testing.T) {
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-id:job-id",
		})
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)

		mock := &mockUploader{
			createFn: func(_ context.Context, _ *createArtifactRequest) (*createArtifactResponse, error) {
				return &createArtifactResponse{OK: true, SignedUploadURL: "https://blob.example.com/upload"}, nil
			},
			uploadFn: func(_ context.Context, _ string, _ []byte) error {
				return nil
			},
			finalizeFn: func(_ context.Context, _ *finalizeArtifactRequest) (*finalizeArtifactResponse, error) {
				return nil, fmt.Errorf("finalize failed")
			},
		}

		store := &Store{
			owner:    "testowner",
			repo:     "testrepo",
			uploader: mock,
		}

		ctx := context.Background()
		err := store.Upload(ctx, "test/key.tfplan", strings.NewReader("plan"), 4, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
		assert.Contains(t, err.Error(), "finalize failed")
	})

	t.Run("invalid runtime token", func(t *testing.T) {
		t.Setenv("ACTIONS_RUNTIME_TOKEN", "not-a-jwt")

		mock := &mockUploader{
			createFn: func(_ context.Context, _ *createArtifactRequest) (*createArtifactResponse, error) { return nil, nil },
			uploadFn: func(_ context.Context, _ string, _ []byte) error { return nil },
			finalizeFn: func(_ context.Context, _ *finalizeArtifactRequest) (*finalizeArtifactResponse, error) {
				return nil, nil
			},
		}

		store := &Store{
			owner:    "testowner",
			repo:     "testrepo",
			uploader: mock,
		}

		ctx := context.Background()
		err := store.Upload(ctx, "test/key.tfplan", strings.NewReader("plan"), 4, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)
	})

	t.Run("zero retention days omits expiration", func(t *testing.T) {
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-id:job-id",
		})
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)

		var capturedCreateReq *createArtifactRequest
		mock := &mockUploader{
			createFn: func(_ context.Context, req *createArtifactRequest) (*createArtifactResponse, error) {
				capturedCreateReq = req
				return &createArtifactResponse{OK: true, SignedUploadURL: "https://blob.example.com/upload"}, nil
			},
			uploadFn: func(_ context.Context, _ string, _ []byte) error { return nil },
			finalizeFn: func(_ context.Context, _ *finalizeArtifactRequest) (*finalizeArtifactResponse, error) {
				return &finalizeArtifactResponse{OK: true, ArtifactID: 1}, nil
			},
		}

		store := &Store{
			owner:         "testowner",
			repo:          "testrepo",
			retentionDays: 0,
			uploader:      mock,
		}

		ctx := context.Background()
		err := store.Upload(ctx, "test/key.tfplan", strings.NewReader("plan"), 4, nil)
		require.NoError(t, err)
		assert.Empty(t, capturedCreateReq.ExpiresAfter)
	})
}

func TestStore_Download(t *testing.T) {
	t.Run("artifact not found", func(t *testing.T) {
		// Create a mock server that returns empty artifact list.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total_count": 0, "artifacts": []}`))
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		_, _, err := store.Download(ctx, "nonexistent/key.tfplan")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	})
}

func TestStore_Delete(t *testing.T) {
	t.Run("artifact not found returns no error", func(t *testing.T) {
		// Create a mock server that returns empty artifact list.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total_count": 0, "artifacts": []}`))
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		err := store.Delete(ctx, "nonexistent/key.tfplan")
		// Delete of non-existent artifact should not error.
		assert.NoError(t, err)
	})
}

func TestStore_List(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total_count": 0, "artifacts": []}`))
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		files, err := store.List(ctx, artifact.Query{All: true})
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("with artifacts", func(t *testing.T) {
		now := time.Now()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 2,
				"artifacts": []map[string]any{
					{
						"id":            1,
						"name":          "planfile-stack1--component1--sha1.tfplan",
						"size_in_bytes": 1024,
						"created_at":    now.Format(time.RFC3339),
					},
					{
						"id":            2,
						"name":          "planfile-stack2--component2--sha2.tfplan",
						"size_in_bytes": 2048,
						"created_at":    now.Add(-time.Hour).Format(time.RFC3339),
					},
					{
						"id":            3,
						"name":          "other-artifact",
						"size_in_bytes": 512,
						"created_at":    now.Format(time.RFC3339),
					},
				},
			}
			respBytes, _ := json.Marshal(response)
			_, _ = w.Write(respBytes)
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		files, err := store.List(ctx, artifact.Query{All: true})
		require.NoError(t, err)
		// Should only include planfile-* artifacts (prefix filtering).
		assert.Len(t, files, 2)

		// Should be sorted by last modified (newest first).
		assert.Equal(t, "stack1/component1/sha1.tfplan", files[0].Name)
		assert.Equal(t, "stack2/component2/sha2.tfplan", files[1].Name)
	})

	t.Run("with pagination", func(t *testing.T) {
		now := time.Now()
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")

			if callCount == 1 {
				// First page: include Link header pointing to next page.
				w.Header().Set("Link", fmt.Sprintf(`<%s/repos/testowner/testrepo/actions/artifacts?per_page=100&page=2>; rel="next"`, r.Host))
				response := map[string]any{
					"total_count": 2,
					"artifacts": []map[string]any{
						{
							"id":            1,
							"name":          "planfile-stack1--comp1--sha1.tfplan",
							"size_in_bytes": 100,
							"created_at":    now.Format(time.RFC3339),
						},
					},
				}
				respBytes, _ := json.Marshal(response)
				_, _ = w.Write(respBytes)
			} else {
				// Second page: no Link header (last page).
				response := map[string]any{
					"total_count": 2,
					"artifacts": []map[string]any{
						{
							"id":            2,
							"name":          "planfile-stack2--comp2--sha2.tfplan",
							"size_in_bytes": 200,
							"created_at":    now.Add(-time.Hour).Format(time.RFC3339),
						},
					},
				}
				respBytes, _ := json.Marshal(response)
				_, _ = w.Write(respBytes)
			}
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		files, err := store.List(ctx, artifact.Query{All: true})
		require.NoError(t, err)
		assert.Len(t, files, 2)
		assert.Equal(t, 2, callCount, "should have made 2 API calls for pagination")

		// Should be sorted by last modified (newest first).
		assert.Equal(t, "stack1/comp1/sha1.tfplan", files[0].Name)
		assert.Equal(t, "stack2/comp2/sha2.tfplan", files[1].Name)
	})

	t.Run("with prefix filter", func(t *testing.T) {
		now := time.Now()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 2,
				"artifacts": []map[string]any{
					{
						"id":            1,
						"name":          "planfile-stack1--component1--sha1.tfplan",
						"size_in_bytes": 1024,
						"created_at":    now.Format(time.RFC3339),
					},
					{
						"id":            2,
						"name":          "planfile-stack2--component2--sha2.tfplan",
						"size_in_bytes": 2048,
						"created_at":    now.Format(time.RFC3339),
					},
				},
			}
			respBytes, _ := json.Marshal(response)
			_, _ = w.Write(respBytes)
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		files, err := store.List(ctx, artifact.Query{Stacks: []string{"stack1"}})
		require.NoError(t, err)
		assert.Len(t, files, 1)
		assert.Equal(t, "stack1/component1/sha1.tfplan", files[0].Name)
	})
}

func TestStore_Exists(t *testing.T) {
	t.Run("artifact exists", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 1,
				"artifacts": []map[string]any{
					{
						"id":   1,
						"name": "planfile-stack--component--sha.tfplan",
					},
				},
			}
			respBytes, _ := json.Marshal(response)
			_, _ = w.Write(respBytes)
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		exists, err := store.Exists(ctx, "stack/component/sha.tfplan")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("artifact does not exist", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total_count": 0, "artifacts": []}`))
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		exists, err := store.Exists(ctx, "nonexistent/key.tfplan")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestStore_GetMetadata(t *testing.T) {
	t.Run("artifact found", func(t *testing.T) {
		now := time.Now()
		expires := now.Add(7 * 24 * time.Hour)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := map[string]any{
				"total_count": 1,
				"artifacts": []map[string]any{
					{
						"id":         1,
						"name":       "planfile-stack--component--sha.tfplan",
						"created_at": now.Format(time.RFC3339),
						"expires_at": expires.Format(time.RFC3339),
					},
				},
			}
			respBytes, _ := json.Marshal(response)
			_, _ = w.Write(respBytes)
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		metadata, err := store.GetMetadata(ctx, "stack/component/sha.tfplan")
		require.NoError(t, err)
		require.NotNil(t, metadata)
		assert.NotNil(t, metadata.ExpiresAt)
	})

	t.Run("artifact not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total_count": 0, "artifacts": []}`))
		}))
		defer server.Close()

		store := &Store{
			httpClient: server.Client(),
			baseURL:    server.URL,
			prefix:     "planfile",
			owner:      "testowner",
			repo:       "testrepo",
		}

		ctx := context.Background()
		_, err := store.GetMetadata(ctx, "nonexistent/key.tfplan")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	})
}

func TestParseNextPage(t *testing.T) {
	tests := []struct {
		name     string
		link     string
		expected int
	}{
		{
			name:     "empty header",
			link:     "",
			expected: 0,
		},
		{
			name:     "with next page",
			link:     `<https://api.github.com/repos/owner/repo/actions/artifacts?per_page=100&page=2>; rel="next", <https://api.github.com/repos/owner/repo/actions/artifacts?per_page=100&page=5>; rel="last"`,
			expected: 2,
		},
		{
			name:     "no next rel",
			link:     `<https://api.github.com/repos/owner/repo/actions/artifacts?per_page=100&page=5>; rel="last"`,
			expected: 0,
		},
		{
			name:     "page 3",
			link:     `<https://api.github.com/repos/owner/repo/actions/artifacts?per_page=100&page=3>; rel="next"`,
			expected: 3,
		},
		{
			name:     "page param first in query",
			link:     `<https://api.github.com/repos/owner/repo/actions/artifacts?page=4&per_page=100>; rel="next"`,
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNextPage(tt.link)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions for tests.

func createTestZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for name, content := range files {
		w, err := zipWriter.Create(name)
		require.NoError(t, err)
		_, err = w.Write(content)
		require.NoError(t, err)
	}

	require.NoError(t, zipWriter.Close())
	return buf.Bytes()
}

func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

// createFakeJWT creates a fake JWT token with the given claims payload.
func createFakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)

	signature := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))

	return header + "." + encodedPayload + "." + signature
}

func TestGetBackendIDsFromToken(t *testing.T) {
	t.Run("valid token with Actions.Results scope", func(t *testing.T) {
		token := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:workflow-run-id-123:workflow-job-id-456",
		})

		ids, err := getBackendIDsFromToken(token)
		require.NoError(t, err)
		assert.Equal(t, "workflow-run-id-123", ids.WorkflowRunBackendID)
		assert.Equal(t, "workflow-job-id-456", ids.WorkflowJobRunBackendID)
	})

	t.Run("token with multiple scopes", func(t *testing.T) {
		token := createFakeJWT(t, map[string]any{
			"scp": "Actions.GenericRead:some:thing Actions.Results:run-id:job-id Actions.Upload:write",
		})

		ids, err := getBackendIDsFromToken(token)
		require.NoError(t, err)
		assert.Equal(t, "run-id", ids.WorkflowRunBackendID)
		assert.Equal(t, "job-id", ids.WorkflowJobRunBackendID)
	})

	t.Run("token missing Actions.Results scope", func(t *testing.T) {
		token := createFakeJWT(t, map[string]any{
			"scp": "Actions.GenericRead:some:thing",
		})

		_, err := getBackendIDsFromToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Actions.Results scope not found")
	})

	t.Run("invalid JWT format", func(t *testing.T) {
		_, err := getBackendIDsFromToken("not-a-jwt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JWT format")
	})

	t.Run("invalid base64 payload", func(t *testing.T) {
		_, err := getBackendIDsFromToken("header.!!!invalid-base64!!!.signature")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode JWT payload")
	})

	t.Run("invalid JSON payload", func(t *testing.T) {
		payload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
		token := "header." + payload + ".signature"
		_, err := getBackendIDsFromToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse JWT claims")
	})

	t.Run("malformed Actions.Results scope", func(t *testing.T) {
		token := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:only-one-part",
		})

		_, err := getBackendIDsFromToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid Actions.Results scope format")
	})
}

func TestCreateArtifactZip(t *testing.T) {
	t.Run("with data and metadata", func(t *testing.T) {
		dataContent := []byte("plan file data")
		metadata := &artifact.Metadata{}
		metadata.Stack = "prod"
		metadata.Component = "vpc"
		metadata.SHA = "abc123"

		zipData, err := createArtifactZip(bytes.NewReader(dataContent), metadata)
		require.NoError(t, err)
		require.NotEmpty(t, zipData)

		// Verify the zip contents.
		reader, meta, err := extractFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, dataContent, content)

		require.NotNil(t, meta)
		assert.Equal(t, "prod", meta.Stack)
		assert.Equal(t, "vpc", meta.Component)
		assert.Equal(t, "abc123", meta.SHA)
	})

	t.Run("without metadata", func(t *testing.T) {
		dataContent := []byte("plan file content")

		zipData, err := createArtifactZip(bytes.NewReader(dataContent), nil)
		require.NoError(t, err)

		reader, meta, err := extractFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, dataContent, content)
		assert.Nil(t, meta)
	})

	t.Run("empty data", func(t *testing.T) {
		zipData, err := createArtifactZip(bytes.NewReader([]byte{}), nil)
		require.NoError(t, err)

		reader, _, err := extractFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Empty(t, content)
	})
}

func TestRuntimeUploader_CreateArtifact(t *testing.T) {
	t.Run("successful create", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Contains(t, r.URL.Path, "CreateArtifact")
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer test-runtime-token", r.Header.Get("Authorization"))

			var req createArtifactRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "planfile-test", req.Name)
			assert.Equal(t, 4, req.Version)

			w.Header().Set("Content-Type", "application/json")
			resp := createArtifactResponse{
				OK:              true,
				SignedUploadURL: "https://blob.example.com/upload?sig=abc123",
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		uploader := newRuntimeUploader(server.URL, "test-runtime-token")
		ctx := context.Background()

		resp, err := uploader.CreateArtifact(ctx, &createArtifactRequest{
			Version: 4,
			Name:    "planfile-test",
		})
		require.NoError(t, err)
		assert.True(t, resp.OK)
		assert.Equal(t, "https://blob.example.com/upload?sig=abc123", resp.SignedUploadURL)
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "internal error"}`))
		}))
		defer server.Close()

		uploader := newRuntimeUploader(server.URL, "test-token")
		ctx := context.Background()

		_, err := uploader.CreateArtifact(ctx, &createArtifactRequest{Name: "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
	})
}

func TestRuntimeUploader_UploadBlob(t *testing.T) {
	t.Run("successful upload", func(t *testing.T) {
		var receivedData []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))
			assert.Equal(t, "BlockBlob", r.Header.Get("x-ms-blob-type"))

			var err error
			receivedData, err = io.ReadAll(r.Body)
			require.NoError(t, err)

			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		uploader := newRuntimeUploader("https://unused.example.com", "unused-token")
		ctx := context.Background()

		testData := []byte("zip file content here")
		err := uploader.UploadBlob(ctx, server.URL+"/upload", testData)
		require.NoError(t, err)
		assert.Equal(t, testData, receivedData)
	})

	t.Run("upload failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("access denied"))
		}))
		defer server.Close()

		uploader := newRuntimeUploader("https://unused.example.com", "unused-token")
		ctx := context.Background()

		err := uploader.UploadBlob(ctx, server.URL+"/upload", []byte("data"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status 403")
	})
}

func TestRuntimeUploader_FinalizeArtifact(t *testing.T) {
	t.Run("successful finalize", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Contains(t, r.URL.Path, "FinalizeArtifact")
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

			var req finalizeArtifactRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "planfile-test", req.Name)
			assert.Equal(t, int64(1024), req.Size)

			w.Header().Set("Content-Type", "application/json")
			resp := finalizeArtifactResponse{
				OK:         true,
				ArtifactID: 42,
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		uploader := newRuntimeUploader(server.URL, "test-token")
		ctx := context.Background()

		resp, err := uploader.FinalizeArtifact(ctx, &finalizeArtifactRequest{
			Name: "planfile-test",
			Size: 1024,
		})
		require.NoError(t, err)
		assert.True(t, resp.OK)
		assert.Equal(t, int64(42), resp.ArtifactID)
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad request"}`))
		}))
		defer server.Close()

		uploader := newRuntimeUploader(server.URL, "test-token")
		ctx := context.Background()

		_, err := uploader.FinalizeArtifact(ctx, &finalizeArtifactRequest{Name: "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status 400")
	})
}

func TestNewRuntimeUploader(t *testing.T) {
	t.Run("adds trailing slash", func(t *testing.T) {
		u := newRuntimeUploader("https://example.com", "token")
		assert.Equal(t, "https://example.com/", u.baseURL)
	})

	t.Run("preserves existing trailing slash", func(t *testing.T) {
		u := newRuntimeUploader("https://example.com/", "token")
		assert.Equal(t, "https://example.com/", u.baseURL)
	})

	t.Run("sets token", func(t *testing.T) {
		u := newRuntimeUploader("https://example.com", "my-token")
		assert.Equal(t, "my-token", u.token)
	})
}

func TestNewStore_WithRuntimeEnv(t *testing.T) {
	t.Run("creates uploader when runtime env is set", func(t *testing.T) {
		runtimeToken := createFakeJWT(t, map[string]any{
			"scp": "Actions.Results:run-id:job-id",
		})
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv("ACTIONS_RUNTIME_TOKEN", runtimeToken)
		t.Setenv("ACTIONS_RESULTS_URL", "https://results.example.com")

		store, err := NewStore(artifact.StoreOptions{
			Options: map[string]any{
				"owner": "testowner",
				"repo":  "testrepo",
			},
		})
		require.NoError(t, err)
		s, ok := store.(*Store)
		require.True(t, ok)
		assert.NotNil(t, s.uploader)
	})

	t.Run("no uploader without runtime env", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv("ACTIONS_RUNTIME_TOKEN", "")
		t.Setenv("ACTIONS_RESULTS_URL", "")

		store, err := NewStore(artifact.StoreOptions{
			Options: map[string]any{
				"owner": "testowner",
				"repo":  "testrepo",
			},
		})
		require.NoError(t, err)
		s, ok := store.(*Store)
		require.True(t, ok)
		assert.Nil(t, s.uploader)
	})

	t.Run("no uploader with partial runtime env", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv("ACTIONS_RUNTIME_TOKEN", "some-token")
		t.Setenv("ACTIONS_RESULTS_URL", "")

		store, err := NewStore(artifact.StoreOptions{
			Options: map[string]any{
				"owner": "testowner",
				"repo":  "testrepo",
			},
		})
		require.NoError(t, err)
		s, ok := store.(*Store)
		require.True(t, ok)
		assert.Nil(t, s.uploader)
	})
}
