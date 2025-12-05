package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	al "github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Define custom log levels for testing.
// Using charmbracelet/log package level constants.
var (
	// Standard levels from the charmbracelet/log package.
	debugLogLevel = log.DebugLevel
	infoLogLevel  = log.InfoLevel
	warnLogLevel  = log.WarnLevel

	// Trace is lower than debug in the charmbracelet/log package.
	traceLogLevel log.Level = -4
)

type MockArtifactoryClient struct {
	mock.Mock
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
				})).Return(0, 1, fmt.Errorf("download failed"))
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
