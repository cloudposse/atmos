package aws

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// serveECRAuthorization points the ECR client at a fake endpoint that returns body
// for GetAuthorizationToken and records the signing region of the request.
func serveECRAuthorization(t *testing.T, body string) func() string {
	t.Helper()

	// Keep the developer's shared AWS files and profile out of the SDK config load.
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "")
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	// Inherited FIPS or dual-stack endpoint policies make the SDK reject the custom fake endpoint.
	t.Setenv("AWS_USE_FIPS_ENDPOINT", "false")
	t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "false")

	// The handler runs on the server goroutine, so the observed region is guarded by a mutex.
	var mu sync.Mutex
	signedRegion := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		assert.Equal(t, "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken", r.Header.Get("X-Amz-Target"))
		// Authorization: AWS4-HMAC-SHA256 Credential=AK/<date>/<region>/ecr/aws4_request, ...
		if parts := strings.Split(r.Header.Get("Authorization"), "/"); len(parts) > 2 {
			mu.Lock()
			signedRegion = parts[2]
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_ECR", srv.URL)
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return signedRegion
	}
}

func TestGetAuthorizationToken(t *testing.T) {
	token := base64.StdEncoding.EncodeToString([]byte("AWS:p@ss:with:colons"))

	tests := []struct {
		name         string
		body         string
		credsRegion  string
		region       string
		wantRegion   string
		wantResult   *ECRAuthResult
		wantErrSubst string
	}{
		{
			name: "decodes token, strips scheme, and keeps expiry",
			body: `{"authorizationData":[{"authorizationToken":"` + token +
				`","expiresAt":1893553445,"proxyEndpoint":"https://123456789012.dkr.ecr.eu-west-1.amazonaws.com"}]}`,
			credsRegion: "us-east-1",
			region:      "eu-west-1",
			wantRegion:  "eu-west-1",
			wantResult: &ECRAuthResult{
				Username:  "AWS",
				Password:  "p@ss:with:colons",
				Registry:  "123456789012.dkr.ecr.eu-west-1.amazonaws.com",
				ExpiresAt: time.Unix(1893553445, 0),
			},
		},
		{
			name:        "falls back to the credentials region",
			body:        `{"authorizationData":[{"authorizationToken":"` + token + `"}]}`,
			credsRegion: "ap-southeast-2",
			wantRegion:  "ap-southeast-2",
			wantResult:  &ECRAuthResult{Username: "AWS", Password: "p@ss:with:colons"},
		},
		{
			name:         "no authorization data",
			body:         `{"authorizationData":[]}`,
			region:       "us-east-1",
			wantErrSubst: "no authorization data returned",
		},
		{
			name:         "token is not base64",
			body:         `{"authorizationData":[{"authorizationToken":"!!not-base64!!"}]}`,
			region:       "us-east-1",
			wantErrSubst: "failed to decode token",
		},
		{
			name:         "token has no password separator",
			body:         `{"authorizationData":[{"authorizationToken":"` + base64.StdEncoding.EncodeToString([]byte("AWS")) + `"}]}`,
			region:       "us-east-1",
			wantErrSubst: "invalid token format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signedRegion := serveECRAuthorization(t, tt.body)
			creds := &types.AWSCredentials{AccessKeyID: "AK", SecretAccessKey: "SK", SessionToken: "TK", Region: tt.credsRegion}

			result, err := GetAuthorizationToken(context.Background(), creds, "123456789012", tt.region)

			if tt.wantErrSubst != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrECRAuthFailed)
				assert.ErrorContains(t, err, tt.wantErrSubst)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRegion, signedRegion())
			assert.Equal(t, tt.wantResult.Username, result.Username)
			assert.Equal(t, tt.wantResult.Password, result.Password)
			assert.Equal(t, tt.wantResult.Registry, result.Registry)
			assert.True(t, tt.wantResult.ExpiresAt.Equal(result.ExpiresAt), "expiresAt %s, want %s", result.ExpiresAt, tt.wantResult.ExpiresAt)
		})
	}
}

func TestGetAuthorizationToken_RejectsNonAWSCredentials(t *testing.T) {
	result, err := GetAuthorizationToken(context.Background(), &types.OIDCCredentials{Token: "jwt"}, "123456789012", "us-east-1")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrECRAuthFailed)
}
