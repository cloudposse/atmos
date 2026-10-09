package cloudformation

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourceOnlyTemplateInference resolves the single provisioned template without requiring an
// explicit component path setting.
func TestSourceOnlyTemplateInference(t *testing.T) {
	dir := t.TempDir()
	body := "Resources: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "downloaded.yaml"), []byte(body), 0o600))
	stubProvisionAndResolveComponentPath(t, dir, nil)
	section := map[string]any{"stack_name": "source-test", "source": map[string]any{"uri": "https://example.test/downloaded.yaml?version=1"}}
	require.NoError(t, validateComponentConfig(section))
	spec, err := resolveSpecAndTemplate(context.Background(), &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{ComponentSection: section}, OperationRender)
	require.NoError(t, err)
	assert.Equal(t, "downloaded.yaml", spec.TemplatePath)
	assert.Equal(t, body, spec.TemplateBody)
}

// TestSourceOnlyHTTPTemplateAndDeployedReads checks escaped filenames and cache reuse against a
// loopback source, with no downloads for deployed-stack operations.
func TestSourceOnlyHTTPTemplateAndDeployedReads(t *testing.T) {
	dir := t.TempDir()
	var requests atomic.Int64
	body := "Resources: {}\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, body)
		}
	}))
	t.Cleanup(server.Close)
	config := &schema.AtmosConfiguration{BasePath: dir}
	newInfo := func() *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{
			Stack: "dev", ComponentFromArg: "source", FinalComponent: "source",
			ComponentSection: map[string]any{
				"stack_name": "source-test", "component": "source", "atmos_stack": "dev",
				"source":    map[string]any{"uri": server.URL + "/template%20file.yaml?version=1", "ttl": "1h"},
				"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
			},
		}
	}
	for _, op := range []Operation{OperationOutput, OperationDelete, OperationDriftDetect, OperationGetTemplate} {
		spec, err := resolveSpecAndTemplate(t.Context(), config, newInfo(), op)
		require.NoError(t, err)
		assert.Equal(t, "source-test", spec.StackName)
	}
	assert.Zero(t, requests.Load(), "deployed operations must not retrieve sources")
	spec, err := resolveSpecAndTemplate(t.Context(), config, newInfo(), OperationRender)
	require.NoError(t, err)
	assert.Equal(t, body, spec.TemplateBody)
	assert.Equal(t, "template file.yaml", spec.TemplatePath)
	firstRequests := requests.Load()
	require.Positive(t, firstRequests)
	spec, err = resolveSpecAndTemplate(t.Context(), config, newInfo(), OperationRender)
	require.NoError(t, err)
	assert.Equal(t, body, spec.TemplateBody)
	assert.Equal(t, firstRequests, requests.Load(), "fresh cached source should not download")
}
