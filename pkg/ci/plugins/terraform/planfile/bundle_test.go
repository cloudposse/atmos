package planfile

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBundle_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		planData string
		lockData string
		hasLock  bool
	}{
		{
			name:     "plan only",
			planData: "fake plan binary data",
			hasLock:  false,
		},
		{
			name:     "plan with lock file",
			planData: "fake plan binary data",
			lockData: "provider lock content",
			hasLock:  true,
		},
		{
			name:     "empty plan data",
			planData: "",
			hasLock:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planReader := strings.NewReader(tt.planData)

			var tarBytes []byte
			var sha256Hex string
			var err error

			if tt.hasLock {
				tarBytes, sha256Hex, err = CreateBundle(planReader, strings.NewReader(tt.lockData))
			} else {
				tarBytes, sha256Hex, err = CreateBundle(planReader, nil)
			}
			require.NoError(t, err)
			assert.NotEmpty(t, tarBytes)
			assert.NotEmpty(t, sha256Hex)
			assert.Len(t, sha256Hex, 64) // SHA256 hex is 64 chars.

			// Extract and verify round-trip.
			plan, lockFile, err := ExtractBundle(bytes.NewReader(tarBytes))
			require.NoError(t, err)
			assert.Equal(t, tt.planData, string(plan))

			if tt.hasLock {
				require.NotNil(t, lockFile)
				assert.Equal(t, tt.lockData, string(lockFile))
			} else {
				assert.Nil(t, lockFile)
			}
		})
	}
}

func TestCreateBundle_NilPlan_ReturnsError(t *testing.T) {
	_, _, err := CreateBundle(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan reader must not be nil")
}

func TestCreateBundle_SHA256_Deterministic(t *testing.T) {
	planData := "deterministic plan data"

	_, sha1, err := CreateBundle(strings.NewReader(planData), nil)
	require.NoError(t, err)

	_, sha2, err := CreateBundle(strings.NewReader(planData), nil)
	require.NoError(t, err)

	assert.Equal(t, sha1, sha2, "SHA256 should be deterministic for same input")
}

func TestCreateBundle_SHA256_DifferentWithLock(t *testing.T) {
	planData := "same plan data"

	_, shaWithout, err := CreateBundle(strings.NewReader(planData), nil)
	require.NoError(t, err)

	_, shaWith, err := CreateBundle(strings.NewReader(planData), strings.NewReader("lock content"))
	require.NoError(t, err)

	assert.NotEqual(t, shaWithout, shaWith, "SHA256 should differ when lock file is added")
}

func TestExtractBundle_CorruptedTar_ReturnsError(t *testing.T) {
	_, _, err := ExtractBundle(strings.NewReader("not a tar archive"))
	require.Error(t, err)
}

func TestExtractBundle_MissingPlan_ReturnsError(t *testing.T) {
	// Create a tar with only a lock file (no plan).
	tarBytes := createCustomTar(t, map[string][]byte{
		BundleLockFilename: []byte("lock content"),
	})

	_, _, err := ExtractBundle(bytes.NewReader(tarBytes))
	require.Error(t, err)
	assert.Contains(t, err.Error(), BundlePlanFilename)
}

func TestExtractBundle_EmptyTar_ReturnsError(t *testing.T) {
	// Create an empty but valid tar.
	tarBytes := createCustomTar(t, map[string][]byte{})

	_, _, err := ExtractBundle(bytes.NewReader(tarBytes))
	require.Error(t, err)
	assert.Contains(t, err.Error(), BundlePlanFilename)
}

// createCustomTar creates a tar archive from a map of filename -> content.
func createCustomTar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for name, data := range files {
		err := writeTarEntry(tw, name, data)
		require.NoError(t, err)
	}

	err := tw.Close()
	require.NoError(t, err)

	return buf.Bytes()
}
