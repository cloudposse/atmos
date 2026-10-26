package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBuildRegistryURL(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		region    string
		expected  string
	}{
		{
			name:      "basic ECR URL",
			accountID: "123456789012",
			region:    "us-east-1",
			expected:  "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			name:      "different region",
			accountID: "999999999999",
			region:    "eu-west-1",
			expected:  "999999999999.dkr.ecr.eu-west-1.amazonaws.com",
		},
		{
			name:      "gov-cloud region",
			accountID: "123456789012",
			region:    "us-gov-west-1",
			expected:  "123456789012.dkr.ecr.us-gov-west-1.amazonaws.com",
		},
		{
			name:      "ap region",
			accountID: "123456789012",
			region:    "ap-southeast-1",
			expected:  "123456789012.dkr.ecr.ap-southeast-1.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildRegistryURL(tt.accountID, tt.region)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRegistryURL(t *testing.T) {
	tests := []struct {
		name              string
		registryURL       string
		expectedAccountID string
		expectedRegion    string
		expectError       bool
	}{
		{
			name:              "standard ECR URL",
			registryURL:       "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			expectedAccountID: "123456789012",
			expectedRegion:    "us-east-1",
			expectError:       false,
		},
		{
			name:              "ECR URL with https prefix",
			registryURL:       "https://123456789012.dkr.ecr.us-west-2.amazonaws.com",
			expectedAccountID: "123456789012",
			expectedRegion:    "us-west-2",
			expectError:       false,
		},
		{
			name:              "gov-cloud region",
			registryURL:       "123456789012.dkr.ecr.us-gov-west-1.amazonaws.com",
			expectedAccountID: "123456789012",
			expectedRegion:    "us-gov-west-1",
			expectError:       false,
		},
		{
			name:              "invalid URL - not ECR format",
			registryURL:       "docker.io/library/nginx",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
		{
			name:              "invalid URL - missing account ID",
			registryURL:       "dkr.ecr.us-east-1.amazonaws.com",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
		{
			name:              "invalid URL - empty string",
			registryURL:       "",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
		{
			name:              "invalid URL - trailing slash not supported",
			registryURL:       "123456789012.dkr.ecr.us-east-1.amazonaws.com/",
			expectedAccountID: "",
			expectedRegion:    "",
			expectError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accountID, region, err := ParseRegistryURL(tt.registryURL)

			if tt.expectError {
				assert.ErrorIs(t, err, errUtils.ErrECRInvalidRegistry)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAccountID, accountID)
				assert.Equal(t, tt.expectedRegion, region)
			}
		})
	}
}

// TestLoadDefaultAWSCredentials_RetrieveFailureHasIdentityHint verifies ambient
// retrieval failures preserve the SDK cause and explain how to use an identity.
func TestLoadDefaultAWSCredentials_RetrieveFailureHasIdentityHint(t *testing.T) {
	// Isolate the SDK's credential chain from developer and CI credentials.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "AWS_") {
			t.Setenv(name, "")
		}
	}
	dir := t.TempDir()
	for _, config := range []struct{ env, file string }{
		{"AWS_CONFIG_FILE", "config"},
		{"AWS_SHARED_CREDENTIALS_FILE", "credentials"},
	} {
		path := filepath.Join(dir, config.file)
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		t.Setenv(config.env, path)
	}

	// A pre-canceled context does not reliably stop the SDK credential cache
	// from consulting IMDS. A local endpoint supplies a deterministic cause.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/latest/api/token", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", server.URL)
	t.Setenv("AWS_EC2_METADATA_V1_DISABLED", "true")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")

	credentials, err := LoadDefaultAWSCredentials(context.Background())
	require.Error(t, err)
	assert.Nil(t, credentials)
	assert.ErrorIs(t, err, errUtils.ErrECRAuthFailed)
	var responseErr *smithyhttp.ResponseError
	require.ErrorAs(t, err, &responseErr, "WithCause must preserve the original SDK response error")
	assert.Equal(t, http.StatusBadRequest, responseErr.HTTPStatusCode())
	assert.EqualValues(t, 1, requests.Load(), "credential retrieval must use only the local IMDS endpoint")

	hints := strings.Join(errors.GetAllHints(err), "\n")
	assert.Contains(t, hints, "atmos aws ecr login --identity")
}

func TestIsECRRegistry(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "valid ECR URL",
			url:      "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			expected: true,
		},
		{
			name:     "valid ECR URL with https prefix",
			url:      "https://123456789012.dkr.ecr.us-east-1.amazonaws.com",
			expected: true,
		},
		{
			name:     "valid ECR URL - different region",
			url:      "999999999999.dkr.ecr.eu-west-1.amazonaws.com",
			expected: true,
		},
		{
			name:     "valid ECR URL - gov-cloud region",
			url:      "123456789012.dkr.ecr.us-gov-west-1.amazonaws.com",
			expected: true,
		},
		{
			name:     "valid ECR URL - ap region",
			url:      "123456789012.dkr.ecr.ap-southeast-1.amazonaws.com",
			expected: true,
		},
		{
			name:     "invalid - Docker Hub",
			url:      "docker.io/library/nginx",
			expected: false,
		},
		{
			name:     "invalid - GitHub Container Registry",
			url:      "ghcr.io/owner/repo",
			expected: false,
		},
		{
			name:     "invalid - empty string",
			url:      "",
			expected: false,
		},
		{
			name:     "invalid - malformed ECR URL (wrong suffix)",
			url:      "123456789012.dkr.ecr.us-east-1.amazonaws.org",
			expected: false,
		},
		{
			name:     "invalid - wrong account ID format (11 digits)",
			url:      "12345678901.dkr.ecr.us-east-1.amazonaws.com",
			expected: false,
		},
		{
			name:     "invalid - wrong account ID format (13 digits)",
			url:      "1234567890123.dkr.ecr.us-east-1.amazonaws.com",
			expected: false,
		},
		{
			name:     "invalid - trailing slash",
			url:      "123456789012.dkr.ecr.us-east-1.amazonaws.com/",
			expected: false,
		},
		{
			name:     "invalid - with path",
			url:      "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-repo",
			expected: false,
		},
		{
			name:     "invalid - ECR public",
			url:      "public.ecr.aws/abcdefgh/my-image",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsECRRegistry(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
