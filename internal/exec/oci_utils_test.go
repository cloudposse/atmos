package exec

import (
	"fmt"
	"io"
	"os"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// MockLayer implements v1.Layer for testing.
type MockLayer struct {
	digestVal       v1.Hash
	sizeVal         int64
	uncompressedErr error
	compressedErr   error
}

func (m *MockLayer) Digest() (v1.Hash, error) {
	return m.digestVal, nil
}

func (m *MockLayer) DiffID() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (m *MockLayer) Compressed() (io.ReadCloser, error) {
	return nil, m.compressedErr
}

func (m *MockLayer) Uncompressed() (io.ReadCloser, error) {
	if m.uncompressedErr != nil {
		return nil, m.uncompressedErr
	}
	return nil, nil
}

func (m *MockLayer) Size() (int64, error) {
	return m.sizeVal, nil
}

func (m *MockLayer) MediaType() (types.MediaType, error) {
	return types.DockerLayer, nil
}

// MockLayerWithDigestError implements v1.Layer for testing digest errors.
type MockLayerWithDigestError struct {
	digestErr error
}

func (m *MockLayerWithDigestError) Digest() (v1.Hash, error) {
	return v1.Hash{}, m.digestErr
}

func (m *MockLayerWithDigestError) DiffID() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (m *MockLayerWithDigestError) Compressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *MockLayerWithDigestError) Uncompressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *MockLayerWithDigestError) Size() (int64, error) {
	return 0, nil
}

func (m *MockLayerWithDigestError) MediaType() (types.MediaType, error) {
	return types.DockerLayer, nil
}

// TestBindEnv tests the Viper environment binding function.
func TestBindEnv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVars  []string
		setEnv   map[string]string
		expected string
	}{
		{
			name:     "Single environment variable",
			key:      "test_key",
			envVars:  []string{"TEST_VAR"},
			setEnv:   map[string]string{"TEST_VAR": "test_value"},
			expected: "test_value",
		},
		{
			name:     "Multiple environment variables with fallback",
			key:      "test_key",
			envVars:  []string{"PRIMARY_VAR", "FALLBACK_VAR"},
			setEnv:   map[string]string{"FALLBACK_VAR": "fallback_value"},
			expected: "fallback_value",
		},
		{
			name:     "No environment variables set",
			key:      "test_key",
			envVars:  []string{"MISSING_VAR"},
			setEnv:   map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.setEnv {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			// Create Viper instance
			v := viper.New()
			bindEnv(v, tt.key, tt.envVars...)

			// Test that the value is accessible
			result := v.GetString(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRemoveTempDir tests the removeTempDir function.
func TestRemoveTempDir_OCIUtils(t *testing.T) {
	// Create a temporary directory for testing.
	tempDir := t.TempDir()

	// Ensure directory exists.
	_, err := os.Stat(tempDir)
	assert.NoError(t, err)

	// Remove the directory.
	removeTempDir(tempDir)

	// Verify directory was removed.
	_, err = os.Stat(tempDir)
	assert.True(t, os.IsNotExist(err))
}

// TestRemoveTempDir_NonExistent tests removeTempDir with non-existent directory.
func TestRemoveTempDir_NonExistent(t *testing.T) {
	// This should not panic when removing a non-existent directory.
	// Use defer/recover to verify no panic occurs.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("removeTempDir panicked on non-existent directory: %v", r)
		}
	}()

	removeTempDir("/nonexistent/directory/path")

	// Test passes if no panic occurs.
	assert.True(t, true, "Function executed without panic on non-existent directory")
}

// TestProcessLayer_DigestError tests that processLayer returns nil when digest fails.
func TestProcessLayer_DigestError(t *testing.T) {
	mockLayer := &MockLayerWithDigestError{
		digestErr: fmt.Errorf("digest calculation failed"),
	}

	// processLayer should return nil (not an error) when digest fails.
	err := processLayer(mockLayer, 0, "/tmp/dest")
	assert.NoError(t, err, "processLayer should return nil when digest fails")
}
