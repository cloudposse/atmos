package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
)

func TestStore_Name(t *testing.T) {
	store := &Store{}
	assert.Equal(t, "github-artifacts", store.Name())
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
	store := &Store{}

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

func TestGetGitHubToken(t *testing.T) {
	tests := []struct {
		name        string
		githubToken string
		ghToken     string
		expectedLen int
		expectEmpty bool
	}{
		{
			name:        "GITHUB_TOKEN set",
			githubToken: "github-token-value",
			ghToken:     "",
			expectEmpty: false,
		},
		{
			name:        "GH_TOKEN fallback",
			githubToken: "",
			ghToken:     "gh-token-value",
			expectEmpty: false,
		},
		{
			name:        "GITHUB_TOKEN takes precedence",
			githubToken: "github-token-value",
			ghToken:     "gh-token-value",
			expectEmpty: false,
		},
		{
			name:        "no token",
			githubToken: "",
			ghToken:     "",
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tt.githubToken)
			t.Setenv("GH_TOKEN", tt.ghToken)

			result := getGitHubToken()
			if tt.expectEmpty {
				assert.Empty(t, result)
			} else {
				assert.NotEmpty(t, result)
				// If GITHUB_TOKEN is set, it should be returned.
				if tt.githubToken != "" {
					assert.Equal(t, tt.githubToken, result)
				} else {
					assert.Equal(t, tt.ghToken, result)
				}
			}
		})
	}
}

func TestNewStore(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")

		_, err := NewStore(planfile.StoreOptions{
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

		_, err := NewStore(planfile.StoreOptions{
			Options: map[string]any{},
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileStoreNotFound)
	})

	t.Run("valid configuration", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")

		store, err := NewStore(planfile.StoreOptions{
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
	})

	t.Run("with GITHUB_REPOSITORY env", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		t.Setenv("GITHUB_REPOSITORY", "envowner/envrepo")

		store, err := NewStore(planfile.StoreOptions{
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

func TestExtractPlanFromZip(t *testing.T) {
	t.Run("valid zip with plan and metadata", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			planFilename: []byte("plan content"),
			metadataFilename: marshalJSON(t, &planfile.Metadata{
				Stack:      "test-stack",
				Component:  "test-component",
				SHA:        "abc123",
				HasChanges: true,
			}),
		})

		reader, metadata, err := extractPlanFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "plan content", string(content))

		require.NotNil(t, metadata)
		assert.Equal(t, "test-stack", metadata.Stack)
		assert.Equal(t, "test-component", metadata.Component)
		assert.Equal(t, "abc123", metadata.SHA)
		assert.True(t, metadata.HasChanges)
	})

	t.Run("valid zip without metadata", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			planFilename: []byte("plan content"),
		})

		reader, metadata, err := extractPlanFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "plan content", string(content))
		assert.Nil(t, metadata)
	})

	t.Run("zip without plan file", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			metadataFilename: []byte("{}"),
		})

		_, _, err := extractPlanFromZip(zipData)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileDownloadFailed)
	})

	t.Run("invalid zip data", func(t *testing.T) {
		_, _, err := extractPlanFromZip([]byte("not a zip"))
		assert.Error(t, err)
	})

	t.Run("zip with invalid metadata JSON", func(t *testing.T) {
		zipData := createTestZip(t, map[string][]byte{
			planFilename:     []byte("plan content"),
			metadataFilename: []byte("not valid json"),
		})

		reader, metadata, err := extractPlanFromZip(zipData)
		require.NoError(t, err)
		require.NotNil(t, reader)
		defer reader.Close()

		// Plan should still be readable.
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "plan content", string(content))

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
			metadataFilename: marshalJSON(t, &planfile.Metadata{
				Stack:     "test",
				Component: "comp",
			}),
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

func TestStore_Upload(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")

	store := &Store{
		owner:         "testowner",
		repo:          "testrepo",
		retentionDays: 7,
	}

	ctx := context.Background()
	data := bytes.NewReader([]byte("plan data"))
	metadata := &planfile.Metadata{
		Stack:     "test",
		Component: "comp",
	}

	// Upload currently returns an error indicating it requires the actions toolkit.
	err := store.Upload(ctx, "test/key.tfplan", data, metadata)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlanfileUploadFailed)
}

func TestStore_Download(t *testing.T) {
	t.Run("artifact not found", func(t *testing.T) {
		// Create a mock server that returns empty artifact list.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total_count": 0, "artifacts": []}`))
		}))
		defer server.Close()

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
		}

		ctx := context.Background()
		_, _, err := store.Download(ctx, "nonexistent/key.tfplan")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileNotFound)
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
		}

		ctx := context.Background()
		files, err := store.List(ctx, "")
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
		}

		ctx := context.Background()
		files, err := store.List(ctx, "")
		require.NoError(t, err)
		// Should only include planfile-* artifacts.
		assert.Len(t, files, 2)

		// Should be sorted by last modified (newest first).
		assert.Equal(t, "stack1/component1/sha1.tfplan", files[0].Key)
		assert.Equal(t, "stack2/component2/sha2.tfplan", files[1].Key)
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
		}

		ctx := context.Background()
		files, err := store.List(ctx, "stack1")
		require.NoError(t, err)
		assert.Len(t, files, 1)
		assert.Equal(t, "stack1/component1/sha1.tfplan", files[0].Key)
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
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

		serverURL, _ := url.Parse(server.URL + "/")
		client := github.NewClient(nil)
		client.BaseURL = serverURL

		store := &Store{
			client: client,
			owner:  "testowner",
			repo:   "testrepo",
		}

		ctx := context.Background()
		_, err := store.GetMetadata(ctx, "nonexistent/key.tfplan")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileNotFound)
	})
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
