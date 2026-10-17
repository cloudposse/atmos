package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestS3FactoryPreservesScopedAuth exercises public downloader wrappers, factory
// registration, and lazy auth through actual SDK-signed loopback requests.
func TestS3FactoryPreservesScopedAuth(t *testing.T) {
	for _, lazy := range []bool{false, true} {
		t.Run(fmt.Sprint(lazy), func(t *testing.T) {
			root := t.TempDir()
			credentials := filepath.Join(root, "credentials")
			require.NoError(t, os.WriteFile(credentials, []byte("[source]\naws_access_key_id = scoped-source\naws_secret_access_key = synthetic\n"), 0o600))
			var mu sync.Mutex
			var signed []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				signed = append(signed, r.Header.Get("Authorization"))
				mu.Unlock()
				if r.Method != http.MethodHead {
					_, _ = fmt.Fprint(w, "Resources: {}\n")
				}
			}))
			defer server.Close()
			auth := &schema.AWSAuthContext{Profile: "source", CredentialsFile: credentials, ConfigFile: filepath.Join(root, "absent"), Region: "us-east-2"}
			ctx := t.Context()
			calls := 0
			if lazy {
				ctx = WithAWSAuthResolver(ctx, func(context.Context) (*schema.AWSAuthContext, error) { calls++; return auth, nil })
			} else {
				ctx = WithAWSAuthContext(ctx, auth)
			}
			dest := filepath.Join(root, "download")
			_, err := NewGoGetterDownloader(nil).(ContextFileDownloader).FetchWithMetadataContext(ctx, "s3::"+server.URL+"/bucket/template.yaml", dest, ClientModeAny, 10*time.Second)
			require.NoError(t, err)
			data, err := os.ReadFile(filepath.Join(dest, "template.yaml"))
			require.NoError(t, err)
			assert.Equal(t, "Resources: {}\n", string(data))
			if lazy {
				assert.Equal(t, 1, calls)
			}
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, signed, 2)
			for _, header := range signed {
				assert.Contains(t, header, "Credential=scoped-source/")
			}
		})
	}
}
