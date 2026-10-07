package hooks

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStepVariablesAWSIdentity(t *testing.T) {
	auth := &schema.AWSAuthContext{Profile: "artifact-writer", Region: "us-east-2"}
	ctx := &ExecContext{Hook: &Hook{}, AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: auth}}}
	vars := stepVariables(ctx)
	assert.Same(t, auth, vars.AWSAuthContext)
	assert.Same(t, auth, vars.Clone().AWSAuthContext)
	ctx.Info.AuthContext = nil
	assert.Nil(t, stepVariables(ctx).AWSAuthContext)
}

// TestArchiveS3Hook exercises the documented two-step packaging recipe through
// the hook engine and a real SDK client, including unchanged-upload detection.
func TestArchiveS3Hook(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "handler.js"), []byte("exports.handler = () => 1;"), 0o600))
	credentialsFile := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, os.WriteFile(credentialsFile, []byte("[component]\naws_access_key_id = hook-key\naws_secret_access_key = hook-secret\n"), 0o600))
	var body []byte
	var headers http.Header
	var uploads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Authorization"), "Credential=hook-key/")
		assert.Equal(t, "/artifacts/handler.zip", r.URL.Path)
		if r.Method == http.MethodHead {
			if body == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Header().Set("Content-Type", headers.Get("Content-Type"))
			w.Header().Set("X-Amz-Meta-Atmos-Sha256", headers.Get("X-Amz-Meta-Atmos-Sha256"))
			w.WriteHeader(http.StatusOK)
			return
		}
		uploads++
		var err error
		body, err = io.ReadAll(r.Body)
		assert.NoError(t, err)
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	hook := &Hook{Kind: stepsKindName, With: []any{
		map[string]any{"name": "package", "type": "archive", "source": "src", "destination": "handler.zip", "mtime": "epoch", "working_directory": dir},
		map[string]any{"name": "upload", "type": "aws/s3", "source": "{{ .steps.package.value }}", "destination": "s3://artifacts/handler.zip", "region": "us-east-1"},
	}}
	ctx := stepsExecContext(hook)
	ctx.Info.AuthContext = &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "component", CredentialsFile: credentialsFile, ConfigFile: filepath.Join(t.TempDir(), "config"), EndpointURL: server.URL}}
	for range 2 {
		out, err := (stepsEngine{}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, StatusSuccess, out.Summary.Status)
	}
	require.Equal(t, 1, uploads)
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	require.Len(t, archive.File, 1)
	assert.Equal(t, "handler.js", archive.File[0].Name)
}
