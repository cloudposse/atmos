package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandPublish(t *testing.T) {
	_ = NewTestKit(t)
	var uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		uploads.Add(1)
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/artifacts/build/output.zip", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "artifact", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("AWS_ENDPOINT_URL_S3", server.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input.zip"), []byte("artifact"), 0o600))
	config := schema.AtmosConfiguration{BasePath: dir, Commands: []schema.Command{{
		Name: "test-publish", WorkingDirectory: dir, Steps: schema.Tasks{{
			Type: "publish", Source: "input.zip", Destination: "output.zip",
			Target: map[string]any{"kind": "aws/s3", "bucket": "artifacts", "prefix": "build", "region": "us-east-1"},
		}},
	}}}
	require.NoError(t, processCustomCommands(config, config.Commands, RootCmd))
	command, _, err := RootCmd.Find([]string{"test-publish"})
	require.NoError(t, err)
	command.Run(command, nil)
	assert.Equal(t, int32(1), uploads.Load())
}
