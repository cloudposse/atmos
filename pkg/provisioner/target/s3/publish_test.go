package s3

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
)

func TestPublishUploadsThenSkipsMatchingS3Object(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "artifact.txt")
	require.NoError(t, os.WriteFile(source, []byte("artifact content"), 0o600))
	files, err := target.LocalPublishFiles(source, "")
	require.NoError(t, err)
	var mu sync.Mutex
	var body, digest, contentType string
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, "/artifacts/releases/artifact.txt", r.URL.Path)
		switch r.Method {
		case http.MethodHead:
			if digest == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("X-Amz-Meta-Atmos-Sha256", digest)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Header().Set("Content-Type", contentType)
		case http.MethodPut:
			data, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Error(readErr)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			body = string(data)
			digest = r.Header.Get("X-Amz-Meta-Atmos-Sha256")
			contentType = r.Header.Get("Content-Type")
			puts++
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	in := &target.PublishInput{
		TargetConfig: map[string]any{"bucket": "artifacts", "region": "us-east-1", "prefix": "releases"},
		Files:        files,
		Env:          map[string]string{"AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test", "AWS_ENDPOINT_URL_S3": server.URL},
	}
	first, err := (&publisher{}).Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, 1, first.Changed)
	assert.Equal(t, []string{"s3://artifacts/releases/artifact.txt"}, first.Locations)
	assert.Equal(t, "releases/artifact.txt", first.Metadata["key"])
	second, err := (&publisher{}).Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, 1, second.Unchanged)
	assert.Equal(t, 0, second.Changed)
	mu.Lock()
	assert.Equal(t, "artifact content", body)
	assert.Equal(t, 1, puts)
	mu.Unlock()
}

func TestValidatePublishFiles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		file target.PublishFile
	}{
		{"negative size", target.PublishFile{Name: "artifact", Size: -1}},
		{"oversized", target.PublishFile{Name: "artifact", Size: maxPublishObjectSize + 1}},
		{"empty name", target.PublishFile{}},
		{"dot path", target.PublishFile{Name: "."}},
		{"dot component", target.PublishFile{Name: "prefix/./artifact"}},
		{"parent traversal", target.PublishFile{Name: "../artifact"}},
		{"absolute path", target.PublishFile{Name: "/artifact"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (&publisher{}).ValidatePublish(&target.PublishInput{TargetConfig: map[string]any{"bucket": "artifacts", "region": "us-east-1"}, Files: []target.PublishFile{tc.file}})
			require.ErrorIs(t, err, errUtils.ErrPublishSource)
		})
	}
	require.NoError(t, (&publisher{}).ValidatePublish(&target.PublishInput{TargetConfig: map[string]any{"bucket": "artifacts", "region": "us-east-1", "prefix": "releases/"}, Files: []target.PublishFile{{Name: "empty", Size: 0}}}))
}

func TestPublishRejectsInvalidTargetBeforeAWSAccess(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, field string
		value       any
		want        error
	}{
		{"bucket type", "bucket", 123, errUtils.ErrPublishTarget},
		{"missing region", "region", "", errUtils.ErrPublishTarget},
		{"bucket URI", "bucket", "s3://artifacts", errUtils.ErrPublishTarget},
		{"unsafe prefix", "prefix", "../outside", errUtils.ErrPublishSource},
		{"unknown option", "delete", true, errUtils.ErrPublishTarget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			settings := map[string]any{"bucket": "artifacts", "region": "us-east-1"}
			settings[tc.field] = tc.value
			result, err := (&publisher{}).Publish(t.Context(), &target.PublishInput{TargetConfig: settings})
			require.ErrorIs(t, err, tc.want)
			assert.Nil(t, result)
		})
	}
}

func TestPublishFailsClosedOnUnreadableSourceOrDeniedDestination(t *testing.T) {
	t.Parallel()
	for _, missing := range []bool{false, true} {
		name := "access denied"
		if missing {
			name = "missing source"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Equal(t, http.MethodHead, r.Method, "a denied HEAD must not fall back to upload")
				w.WriteHeader(http.StatusForbidden)
			}))
			defer server.Close()
			source := filepath.Join(t.TempDir(), "artifact")
			require.NoError(t, os.WriteFile(source, []byte("artifact content"), 0o600))
			files, err := target.LocalPublishFiles(source, "")
			require.NoError(t, err)
			if missing {
				require.NoError(t, os.Remove(source))
			}
			result, err := (&publisher{}).Publish(t.Context(), &target.PublishInput{
				TargetConfig: map[string]any{"bucket": "artifacts", "region": "us-east-1"},
				Files:        files,
				Env:          map[string]string{"AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test", "AWS_ENDPOINT_URL_S3": server.URL},
			})
			require.ErrorIs(t, err, errUtils.ErrPublishFailed)
			assert.Nil(t, result)
			if missing {
				require.ErrorIs(t, err, os.ErrNotExist)
				assert.Zero(t, requests.Load())
			} else {
				assert.Equal(t, int32(1), requests.Load())
			}
		})
	}
}
