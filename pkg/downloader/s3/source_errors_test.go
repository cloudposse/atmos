package s3

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fetchFromFakeS3 downloads s3::<server>/<path> through the real SDK client and go-getter.
func fetchFromFakeS3(t *testing.T, handler http.HandlerFunc, path string) error {
	t.Helper()
	root := t.TempDir()
	credentialFile := filepath.Join(root, "credentials")
	require.NoError(t, os.WriteFile(credentialFile, []byte("[source]\naws_access_key_id = id\naws_secret_access_key = synthetic\n"), 0o600))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx := WithAuthContext(t.Context(), &schema.AWSAuthContext{Profile: "source", CredentialsFile: credentialFile, ConfigFile: filepath.Join(root, "absent-config"), Region: "us-east-2"})
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client := &getter.Client{
		Ctx:     timeoutCtx,
		Src:     "s3::" + server.URL + path,
		Dst:     filepath.Join(root, "out"),
		Mode:    getter.ClientModeAny,
		Getters: map[string]getter.Getter{"s3": NewGetter(timeoutCtx)},
	}
	return client.Get()
}

// hintsOf joins every hint attached to err.
func hintsOf(err error) string {
	return strings.Join(cockroachErrors.GetAllHints(err), "\n")
}

func TestS3SourceCrossRegionBucket(t *testing.T) {
	for _, tt := range []struct {
		name         string
		bucketRegion string
		wantHint     string
	}{
		{name: "region header present", bucketRegion: "us-west-2", wantHint: "?region=us-west-2"},
		{name: "region header absent", wantHint: "?region="},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := fetchFromFakeS3(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.bucketRegion != "" {
					w.Header().Set(bucketRegionHeader, tt.bucketRegion)
				}
				w.WriteHeader(http.StatusMovedPermanently)
			}, "/bucket/template.yaml?region=us-east-2")

			require.ErrorIs(t, err, errUtils.ErrS3SourceRegionMismatch)
			assert.Contains(t, err.Error(), "bucket `bucket`")
			assert.Contains(t, err.Error(), "us-east-2", "the region used for the request is reported")
			assert.Contains(t, err.Error(), "/bucket/template.yaml", "the source URI is reported")
			assert.NotContains(t, err.Error(), "?region=", "the query string stays out of the message")
			assert.Contains(t, hintsOf(err), tt.wantHint)
			if tt.bucketRegion != "" {
				assert.Contains(t, err.Error(), tt.bucketRegion, "the bucket's region is reported")
			}
		})
	}
}

func TestS3SourceForbiddenObject(t *testing.T) {
	err := fetchFromFakeS3(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}, "/bucket/missing.yaml?region=us-east-2")

	require.ErrorIs(t, err, errUtils.ErrS3SourceForbidden)
	assert.Contains(t, err.Error(), "missing.yaml")
	assert.Contains(t, err.Error(), "bucket")
	hints := hintsOf(err)
	assert.Contains(t, hints, "s3:ListBucket")
	assert.Contains(t, hints, "does not exist")
}

// TestS3SourceOtherFailuresAreUnchanged is the negative path: statuses that are neither a region
// mismatch nor a 403 must not be relabeled.
func TestS3SourceOtherFailuresAreUnchanged(t *testing.T) {
	err := fetchFromFakeS3(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}, "/bucket/template.yaml?region=us-east-2")

	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrS3SourceForbidden)
	assert.NotErrorIs(t, err, errUtils.ErrS3SourceRegionMismatch)
}

func TestS3SourceRejectsCredentialQueryParameters(t *testing.T) {
	for _, param := range []string{"aws_profile", "aws_access_key_id", "aws_access_key_secret", "aws_access_token"} {
		for _, raw := range []string{
			"s3://bucket/key?region=us-east-2&%s=value",
			"https://s3.us-east-2.amazonaws.com/bucket/key?%s=value",
			"http://localhost:1234/bucket/key?%s=value",
		} {
			t.Run(param+"/"+raw, func(t *testing.T) {
				u, err := url.Parse(fmt.Sprintf(raw, param))
				require.NoError(t, err)

				_, err = parseS3SourceURL(u)

				require.ErrorIs(t, err, errUtils.ErrS3SourceUnsupportedParam)
				assert.Contains(t, err.Error(), param)
				assert.NotContains(t, err.Error(), "value", "credential values never reach the message")
				assert.Contains(t, hintsOf(err), "Atmos identity")
			})
		}
	}

	t.Run("allowed parameters are accepted", func(t *testing.T) {
		u, err := url.Parse("s3://bucket/key?region=us-east-2&version=abc")
		require.NoError(t, err)
		loc, err := parseS3SourceURL(u)
		require.NoError(t, err)
		assert.Equal(t, "abc", loc.version)
	})
}
