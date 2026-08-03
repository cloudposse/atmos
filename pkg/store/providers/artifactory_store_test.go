package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	log "github.com/cloudposse/atmos/pkg/logger"
	storepkg "github.com/cloudposse/atmos/pkg/store"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	al "github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Define log levels for testing using atmos logger constants.
var (
	debugLogLevel = log.DebugLevel
	infoLogLevel  = log.InfoLevel
	warnLogLevel  = log.WarnLevel
	traceLogLevel = log.TraceLevel
)

type MockArtifactoryClient struct {
	mock.Mock
	// downloadData, when non-nil, overrides the bytes written for a successful
	// download (default is `{"test":"value"}`). Used to exercise GetKey's
	// empty-data and non-JSON fallback branches.
	downloadData *[]byte
}

func (m *MockArtifactoryClient) DownloadFiles(params ...services.DownloadParams) (int, int, error) {
	args := m.Called(params[0])
	totalDownloaded := args.Int(0)
	totalFailed := args.Int(1)
	err := args.Error(2)
	// First check: if there's an error, return immediately.
	if err != nil {
		return totalDownloaded, totalFailed, err
	}

	// Second check: if there are failures, return without creating files.
	if totalFailed > 0 {
		return totalDownloaded, totalFailed, nil
	}

	// Third check: if no downloads, return without creating files.
	if totalDownloaded == 0 {
		return totalDownloaded, totalFailed, nil
	}

	// Only proceed with file creation for successful cases
	targetDir := params[0].Target
	filename := filepath.Base(params[0].Pattern)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return 0, 0, err
	}

	data := []byte(`{"test":"value"}`)
	if m.downloadData != nil {
		data = *m.downloadData
	}
	fullPath := filepath.Join(targetDir, filename)
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
		return 0, 0, err
	}

	return totalDownloaded, totalFailed, nil
}

func (m *MockArtifactoryClient) UploadFiles(options artifactory.UploadServiceOptions, params ...services.UploadParams) (int, int, error) {
	args := m.Called(options, params[0])
	return args.Int(0), args.Int(1), args.Error(2)
}

func TestNewArtifactoryStore(t *testing.T) {
	tests := []struct {
		name        string
		options     ArtifactoryStoreOptions
		expectError bool
	}{
		{
			name: "valid configuration with access token",
			options: ArtifactoryStoreOptions{
				AccessToken: aws.String("test-token"),
				Prefix:      aws.String("test-prefix"),
				RepoName:    "test-repo",
				URL:         "http://artifactory.example.com",
			},
			expectError: false,
		},
		{
			name: "missing access token",
			options: ArtifactoryStoreOptions{
				RepoName: "test-repo",
				URL:      "http://artifactory.example.com",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Temporarily unset environment variables
			originalArtToken := os.Getenv("ARTIFACTORY_ACCESS_TOKEN")
			originalJfrogToken := os.Getenv("JFROG_ACCESS_TOKEN")
			defer func() {
				os.Setenv("ARTIFACTORY_ACCESS_TOKEN", originalArtToken)
				os.Setenv("JFROG_ACCESS_TOKEN", originalJfrogToken)
			}()
			_ = os.Unsetenv("ARTIFACTORY_ACCESS_TOKEN")
			_ = os.Unsetenv("JFROG_ACCESS_TOKEN")

			store, err := NewArtifactoryStore(tt.options)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, store)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, store)
			}
		})
	}
}

func TestArtifactoryStore_getKey(t *testing.T) {
	delimiter := "/"
	store := &ArtifactoryStore{
		prefix:         "prefix",
		repoName:       "repo",
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		expected  string
	}{
		{
			name:      "simple path",
			stack:     "dev",
			component: "app",
			key:       "config.json",
			expected:  "repo/prefix/dev/app/config.json",
		},
		{
			name:      "nested component",
			stack:     "dev",
			component: "app/service",
			key:       "config.json",
			expected:  "repo/prefix/dev/app/service/config.json",
		},
		{
			name:      "multi-level stack",
			stack:     "dev/us-west-2",
			component: "app",
			key:       "config.json",
			expected:  "repo/prefix/dev/us-west-2/app/config.json",
		},
		{
			name:      "slice value",
			stack:     "dev",
			component: "app",
			key:       "[]string{\"a\",\"b\"}",
			expected:  "repo/prefix/dev/app/[]string{\"a\",\"b\"}",
		},
		{
			name:      "map value",
			stack:     "dev",
			component: "app",
			key:       "map[string]string{\"key\":\"value\"}",
			expected:  "repo/prefix/dev/app/map[string]string{\"key\":\"value\"}",
		},
		{
			name:      "nested map value",
			stack:     "dev",
			component: "app",
			key:       "map[string]map[string]int{\"outer\":{\"inner\":42}}",
			expected:  "repo/prefix/dev/app/map[string]map[string]int{\"outer\":{\"inner\":42}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := store.getKey(tt.stack, tt.component, tt.key)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestArtifactoryStore_Set(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		expected  string
	}{
		{
			name:      "basic",
			stack:     "dev",
			component: "app",
			key:       "config.json",
			expected:  "repo/prefix/dev/app/config.json",
		},
		{
			name:      "nested component",
			stack:     "dev",
			component: "app/service",
			key:       "config.json",
			expected:  "repo/prefix/dev/app/service/config.json",
		},
		{
			name:      "multi-level stack",
			stack:     "dev/us-west-2",
			component: "app",
			key:       "config.json",
			expected:  "repo/prefix/dev/us-west-2/app/config.json",
		},
		{
			name:      "slice value",
			stack:     "dev",
			component: "app",
			key:       "[]string{\"a\",\"b\"}",
			expected:  "repo/prefix/dev/app/[]string{\"a\",\"b\"}",
		},
		{
			name:      "map value",
			stack:     "dev",
			component: "app",
			key:       "map[string]string{\"key\":\"value\"}",
			expected:  "repo/prefix/dev/app/map[string]string{\"key\":\"value\"}",
		},
		{
			name:      "nested map value",
			stack:     "dev",
			component: "app",
			key:       "map[string]map[string]int{\"outer\":{\"inner\":42}}",
			expected:  "repo/prefix/dev/app/map[string]map[string]int{\"outer\":{\"inner\":42}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockArtifactoryClient)
			delimiter := "/"
			store := &ArtifactoryStore{
				prefix:         "prefix",
				repoName:       "repo",
				rtManager:      mockClient,
				stackDelimiter: &delimiter,
			}

			mockClient.On("UploadFiles",
				mock.MatchedBy(func(options artifactory.UploadServiceOptions) bool {
					return options.FailFast == true
				}),
				mock.MatchedBy(func(params services.UploadParams) bool {
					return params.Target == tt.expected && params.Flat == true
				})).Return(1, 0, nil)

			err := store.Set(tt.stack, tt.component, tt.key, []byte("test data"))
			assert.NoError(t, err)
			mockClient.AssertExpectations(t)
		})
	}
}

func TestArtifactoryStore_GetWithMockErrors(t *testing.T) {
	mockClient := new(MockArtifactoryClient)
	delimiter := "/"
	store := &ArtifactoryStore{
		prefix:         "prefix",
		repoName:       "repo",
		rtManager:      mockClient,
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name        string
		stack       string
		component   string
		key         string
		mockSetup   func()
		expectError bool
		errorMsg    string
	}{
		{
			name:      "download error",
			stack:     "dev",
			component: "app",
			key:       "config.json",
			mockSetup: func() {
				mockClient.On("DownloadFiles", mock.MatchedBy(func(params services.DownloadParams) bool {
					return params.Pattern == "repo/prefix/dev/app/config.json"
				})).Return(0, 1, fmt.Errorf("download failed")) //nolint
			},
			expectError: true,
			errorMsg:    "download failed",
		},
		{
			name:      "no files downloaded",
			stack:     "dev",
			component: "app",
			key:       "config.json",
			mockSetup: func() {
				mockClient.On("DownloadFiles", mock.Anything).Return(0, 0, nil)
			},
			expectError: true,
			errorMsg:    "no files downloaded",
		},
		{
			name:      "successful download",
			stack:     "dev",
			component: "app",
			key:       "config.json",
			mockSetup: func() {
				mockClient.On("DownloadFiles", mock.Anything).Return(1, 0, nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any previous mock expectations
			mockClient.ExpectedCalls = nil
			mockClient.Calls = nil

			tt.mockSetup()
			result, err := store.Get(tt.stack, tt.component, tt.key)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestArtifactoryStore_GetKey(t *testing.T) {
	mockClient := new(MockArtifactoryClient)
	delimiter := "/"
	store := &ArtifactoryStore{
		prefix:         "prefix",
		repoName:       "repo",
		rtManager:      mockClient,
		stackDelimiter: &delimiter,
	}

	tests := []struct {
		name        string
		key         string
		mockSetup   func()
		expectError bool
		// errIs, when non-nil, is matched against the returned error with errors.Is.
		errIs error
		// errMsg, when non-empty, is matched against the returned error with Contains.
		errMsg   string
		expected interface{}
	}{
		{
			name:        "empty key",
			key:         "",
			mockSetup:   func() {},
			expectError: true,
			errIs:       storepkg.ErrEmptyKey,
		},
		{
			name: "download error",
			key:  "config.json",
			mockSetup: func() {
				mockClient.On("DownloadFiles", mock.MatchedBy(func(params services.DownloadParams) bool {
					return params.Pattern == "repo/prefix/config.json"
				})).Return(0, 1, fmt.Errorf("download failed"))
			},
			expectError: true,
			errMsg:      "download failed",
		},
		{
			name: "successful download appends json extension and parses JSON",
			// No ".json" suffix; GetKey appends it before building the repo path.
			key: "config",
			mockSetup: func() {
				mockClient.On("DownloadFiles", mock.MatchedBy(func(params services.DownloadParams) bool {
					return params.Pattern == "repo/prefix/config.json"
				})).Return(1, 0, nil)
			},
			expectError: false,
			// MockArtifactoryClient writes `{"test":"value"}` on a successful download.
			expected: map[string]interface{}{"test": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any previous mock expectations.
			mockClient.ExpectedCalls = nil
			mockClient.Calls = nil

			tt.mockSetup()
			result, err := store.GetKey(tt.key)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestArtifactoryStore_LoggingConfiguration(t *testing.T) {
	// Save the original function and restore it after the test
	origCreateNoopLogger := createNoopLogger
	defer func() {
		createNoopLogger = origCreateNoopLogger
	}()

	// Save original log level and restore after test
	originalLogLevel := log.GetLevel()
	defer log.SetLevel(originalLogLevel)

	tests := []struct {
		name             string
		atmosLogLevel    log.Level
		expectNoopLogger bool
	}{
		{
			name:             "Debug level uses standard logger",
			atmosLogLevel:    debugLogLevel,
			expectNoopLogger: false,
		},
		{
			name:             "Trace level uses standard logger",
			atmosLogLevel:    traceLogLevel,
			expectNoopLogger: false,
		},
		{
			name:             "Info level uses noopLogger",
			atmosLogLevel:    infoLogLevel,
			expectNoopLogger: true,
		},
		{
			name:             "Warn level uses noopLogger",
			atmosLogLevel:    warnLogLevel,
			expectNoopLogger: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track if noopLogger was created
			noopLoggerCreated := false

			// Override the factory function
			createNoopLogger = func() al.Log {
				noopLoggerCreated = true
				return origCreateNoopLogger()
			}

			// Set the test log level
			log.SetLevel(tt.atmosLogLevel)

			// Create store which should trigger logging setup
			_, err := NewArtifactoryStore(ArtifactoryStoreOptions{
				AccessToken: aws.String("test-token"),
				RepoName:    "test-repo",
				URL:         "http://example.com",
			})
			assert.NoError(t, err)

			// Verify if noopLogger was created as expected
			assert.Equal(t, tt.expectNoopLogger, noopLoggerCreated,
				"For log level %s, noopLogger created: %v, expected: %v",
				tt.atmosLogLevel, noopLoggerCreated, tt.expectNoopLogger)
		})
	}
}

func TestGetAccessKey(t *testing.T) {
	t.Run("explicit access token", func(t *testing.T) {
		tok, err := getAccessKey(&ArtifactoryStoreOptions{AccessToken: aws.String("explicit")})
		assert.NoError(t, err)
		assert.Equal(t, "explicit", tok)
	})

	t.Run("ARTIFACTORY_ACCESS_TOKEN env", func(t *testing.T) {
		t.Setenv("JFROG_ACCESS_TOKEN", "")
		t.Setenv("ARTIFACTORY_ACCESS_TOKEN", "art-env")
		tok, err := getAccessKey(&ArtifactoryStoreOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "art-env", tok)
	})

	t.Run("JFROG_ACCESS_TOKEN env", func(t *testing.T) {
		t.Setenv("ARTIFACTORY_ACCESS_TOKEN", "")
		t.Setenv("JFROG_ACCESS_TOKEN", "jfrog-env")
		tok, err := getAccessKey(&ArtifactoryStoreOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "jfrog-env", tok)
	})

	t.Run("missing token", func(t *testing.T) {
		t.Setenv("ARTIFACTORY_ACCESS_TOKEN", "")
		t.Setenv("JFROG_ACCESS_TOKEN", "")
		_, err := getAccessKey(&ArtifactoryStoreOptions{})
		assert.ErrorIs(t, err, storepkg.ErrMissingArtifactoryToken)
	})
}

func TestArtifactoryStore_getKey_NilDelimiter(t *testing.T) {
	store := &ArtifactoryStore{prefix: "p", repoName: "r"} // stackDelimiter is nil.
	_, err := store.getKey("dev", "app", "k")
	assert.ErrorIs(t, err, storepkg.ErrStackDelimiterNotSet)
}

func TestArtifactoryStore_Get_Validation(t *testing.T) {
	delimiter := "/"
	store := &ArtifactoryStore{prefix: "p", repoName: "r", stackDelimiter: &delimiter}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		want      error
	}{
		{"empty stack", "", "app", "k", storepkg.ErrEmptyStack},
		{"empty component", "dev", "", "k", storepkg.ErrEmptyComponent},
		{"empty key", "dev", "app", "", storepkg.ErrEmptyKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Get(tt.stack, tt.component, tt.key)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestArtifactoryStore_processDownloadedFile(t *testing.T) {
	store := &ArtifactoryStore{}

	t.Run("read error on missing file", func(t *testing.T) {
		_, err := store.processDownloadedFile(t.TempDir(), "nonexistent.json")
		assert.ErrorIs(t, err, storepkg.ErrReadFile)
	})

	t.Run("unmarshal error on non-JSON", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0o600))
		_, err := store.processDownloadedFile(dir, "bad.json")
		assert.ErrorIs(t, err, storepkg.ErrUnmarshalFile)
	})

	t.Run("valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ok.json"), []byte(`{"a":1}`), 0o600))
		result, err := store.processDownloadedFile(dir, "ok.json")
		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"a": float64(1)}, result)
	})
}

func TestArtifactoryStore_Set_Errors(t *testing.T) {
	delimiter := "/"
	newStore := func() (*ArtifactoryStore, *MockArtifactoryClient) {
		mc := new(MockArtifactoryClient)
		return &ArtifactoryStore{prefix: "p", repoName: "r", rtManager: mc, stackDelimiter: &delimiter}, mc
	}

	t.Run("empty stack", func(t *testing.T) {
		s, _ := newStore()
		assert.ErrorIs(t, s.Set("", "app", "k", "v"), storepkg.ErrEmptyStack)
	})

	t.Run("empty component", func(t *testing.T) {
		s, _ := newStore()
		assert.ErrorIs(t, s.Set("dev", "", "k", "v"), storepkg.ErrEmptyComponent)
	})

	t.Run("empty key", func(t *testing.T) {
		s, _ := newStore()
		assert.ErrorIs(t, s.Set("dev", "app", "", "v"), storepkg.ErrEmptyKey)
	})

	t.Run("nil value", func(t *testing.T) {
		s, _ := newStore()
		assert.ErrorIs(t, s.Set("dev", "app", "k", nil), storepkg.ErrNilValue)
	})

	t.Run("getKey error on nil delimiter", func(t *testing.T) {
		mc := new(MockArtifactoryClient)
		s := &ArtifactoryStore{prefix: "p", repoName: "r", rtManager: mc} // nil delimiter.
		assert.ErrorIs(t, s.Set("dev", "app", "k", "v"), storepkg.ErrGetKey)
	})

	t.Run("marshal error", func(t *testing.T) {
		s, _ := newStore()
		// A channel cannot be marshaled to JSON.
		assert.ErrorIs(t, s.Set("dev", "app", "k", make(chan int)), storepkg.ErrMarshalValue)
	})

	t.Run("upload error", func(t *testing.T) {
		s, mc := newStore()
		// A non-[]byte value exercises the json.Marshal branch before upload.
		mc.On("UploadFiles", mock.Anything, mock.Anything).Return(0, 1, fmt.Errorf("upload boom"))
		err := s.Set("dev", "app", "k", "string-value")
		assert.ErrorIs(t, err, storepkg.ErrUploadFile)
		mc.AssertExpectations(t)
	})
}

func TestArtifactoryStore_GetKey_DataVariants(t *testing.T) {
	delimiter := "/"

	t.Run("empty data returns empty string", func(t *testing.T) {
		mc := new(MockArtifactoryClient)
		empty := []byte{}
		mc.downloadData = &empty
		s := &ArtifactoryStore{prefix: "prefix", repoName: "repo", rtManager: mc, stackDelimiter: &delimiter}
		mc.On("DownloadFiles", mock.Anything).Return(1, 0, nil)

		v, err := s.GetKey("config")
		assert.NoError(t, err)
		assert.Equal(t, "", v)
		mc.AssertExpectations(t)
	})

	t.Run("non-JSON data returns string", func(t *testing.T) {
		mc := new(MockArtifactoryClient)
		raw := []byte("plain text")
		mc.downloadData = &raw
		s := &ArtifactoryStore{prefix: "prefix", repoName: "repo", rtManager: mc, stackDelimiter: &delimiter}
		mc.On("DownloadFiles", mock.Anything).Return(1, 0, nil)

		v, err := s.GetKey("config")
		assert.NoError(t, err)
		assert.Equal(t, "plain text", v)
		mc.AssertExpectations(t)
	})
}

func TestBuildArtifactoryStore_ParseError(t *testing.T) {
	// A slice cannot decode into the *string Prefix field.
	_, err := buildArtifactoryStore("n", storepkg.StoreConfig{
		Options: map[string]interface{}{"prefix": []string{"x"}},
	})
	assert.ErrorIs(t, err, storepkg.ErrParseArtifactoryOptions)
}
