package imports_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/cache"
	_ "github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/imports"
)

func TestSourcePolicyChecksCachedRemoteImports(t *testing.T) {
	guard.ResetOrigins()
	t.Cleanup(guard.ResetOrigins)
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: source}
spec:
  policies:
    deny:
      on: source
      action: deny
      rule: "false"
`
	policyFile := filepath.Join(directory, "policy.yaml")
	require.NoError(t, os.WriteFile(policyFile, []byte(manifest), 0o600))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("vars: {value: remote}\n"))
	}))
	defer server.Close()
	fileCache, err := cache.NewFileCache("policy", cache.WithBaseDir(t.TempDir()))
	require.NoError(t, err)
	importer, err := imports.NewRemoteImporter(&schema.AtmosConfiguration{}, imports.WithCache(fileCache))
	require.NoError(t, err)
	uri := server.URL + "/stack.yaml"
	path, err := importer.Download(uri)
	require.NoError(t, err)
	require.Equal(t, "remote", guard.FileOrigin(path).Provenance)
	initialCalls := calls.Load()
	require.Positive(t, initialCalls)
	_, err = importer.Download(uri)
	require.NoError(t, err)
	require.Equal(t, initialCalls, calls.Load())
	require.NoError(t, os.WriteFile(policyFile, []byte(`apiVersion: atmos/v1
kind: Policy
metadata: {name: source}
spec:
  policies:
    deny:
      on: source
      action: deny
      rule: "true"
`), 0o600))
	_, err = importer.Download(uri)
	require.Error(t, err)
	require.Equal(t, initialCalls, calls.Load(), "cached reuse must be denied without another fetch")
}
