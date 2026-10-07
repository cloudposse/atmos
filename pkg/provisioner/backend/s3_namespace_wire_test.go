package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bucketNamespaceHeader is the HTTP header S3 reads the bucket namespace from.
const bucketNamespaceHeader = "x-amz-bucket-namespace"

// createBucketRecorder wraps an S3 handler and records the namespace header of every
// CreateBucket request (a PUT on a bare bucket path) before delegating to it.
type createBucketRecorder struct {
	next http.Handler

	mu      sync.Mutex
	headers []string
}

func (r *createBucketRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// A bare "/<bucket>" PUT without a query string is CreateBucket; other PUTs target
	// objects or bucket sub-resources (versioning, tagging, and so on).
	if req.Method == http.MethodPut && req.URL.RawQuery == "" && strings.Count(strings.Trim(req.URL.Path, "/"), "/") == 0 {
		r.mu.Lock()
		r.headers = append(r.headers, req.Header.Get(bucketNamespaceHeader))
		r.mu.Unlock()
	}
	r.next.ServeHTTP(w, req)
}

func (r *createBucketRecorder) namespaceHeaders() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.headers...)
}

// TestE2E_CreateS3Backend_SendsBucketNamespaceHeader verifies, through the real default
// S3 client, that the configured namespace reaches the wire as the x-amz-bucket-namespace
// header. The mock-client tests only inspect the SDK input struct, so they cannot catch a
// namespace that is set on the input but never serialized into the request.
//
// An emulator such as Floci cannot stand in for this check: it accepts and ignores the
// header, so a test against it would pass whether or not the namespace was sent.
func TestE2E_CreateS3Backend_SendsBucketNamespaceHeader(t *testing.T) {
	tests := []struct {
		name       string
		opts       []CreateOption
		wantHeader string
	}{
		{
			name:       "configured namespace is sent",
			opts:       []CreateOption{WithBucketNamespace(string(types.BucketNamespaceAccountRegional))},
			wantHeader: string(types.BucketNamespaceAccountRegional),
		},
		// Negative path: nothing is sent when the option is absent, so existing configurations are unchanged.
		{name: "no header without the option", opts: nil, wantHeader: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AWS_ACCESS_KEY_ID", "test")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

			recorder := &createBucketRecorder{next: gofakes3.New(s3mem.New()).Server()}
			server := httptest.NewServer(recorder)
			t.Cleanup(server.Close)

			// gofakes3 doesn't support the encryption, public-access, and tagging APIs.
			SetS3SkipUnsupportedTestOps(true)
			t.Cleanup(func() { SetS3SkipUnsupportedTestOps(false) })

			backendConfig := map[string]any{
				"bucket":         "namespace-wire-test",
				"region":         "us-east-1",
				"endpoints":      map[string]any{"s3": server.URL},
				"use_path_style": true,
			}

			_, err := CreateS3Backend(context.Background(), nil, backendConfig, nil, tt.opts...)
			require.NoError(t, err)

			headers := recorder.namespaceHeaders()
			require.Len(t, headers, 1, "exactly one CreateBucket request is expected")
			assert.Equal(t, tt.wantHeader, headers[0])
		})
	}
}
