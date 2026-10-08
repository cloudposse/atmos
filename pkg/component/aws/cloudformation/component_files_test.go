package cloudformation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// sourceArchive returns a gzipped tarball holding a single file.
func sourceArchive(t *testing.T, name, contents string) []byte {
	t.Helper()
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	writer := tar.NewWriter(compressed)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(contents))}))
	_, err := io.WriteString(writer, contents)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, compressed.Close())
	return archive.Bytes()
}

// sourceFixture builds a component whose template comes from the given JIT source URI.
func sourceFixture(t *testing.T, uri string) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) {
	t.Helper()
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	config.Components.CloudFormation.BasePath = "components/cloudformation"
	info := &schema.ConfigAndStacksInfo{
		Stack: "dev", ComponentFromArg: "instance", FinalComponent: "modules/service",
		ComponentSection: map[string]any{
			"component": "modules/service", "atmos_component": "instance", "atmos_stack": "dev",
			"stack_name": "source-dev", "path": "original.yaml",
			"source": map[string]any{"uri": uri, "ttl": "1h"},
		},
	}
	return config, info
}

// TestAuthenticatedNestedSourceRenders verifies an HTTP source with credentials is downloaded
// into the nested component directory and supplies the template, on every render.
func TestAuthenticatedNestedSourceRenders(t *testing.T) {
	const template = "Resources: {}\nDescription: original source\n"
	archive := sourceArchive(t, "original.yaml", template)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "test" || password != "synthetic" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		requests.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	uri := strings.Replace(server.URL, "http://", "http://test:synthetic@", 1) + "/source.tar.gz"
	for range 2 {
		config, info := sourceFixture(t, uri)
		spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationApply)
		require.NoError(t, err)
		assert.Equal(t, template, spec.TemplateBody)
		assert.Contains(t, filepath.ToSlash(spec.TemplateAbsPath), "modules/service/original.yaml")
	}
	assert.Positive(t, requests.Load(), "the source must have been fetched")
}

// TestSourceNotFetchedWhenNoLocalTemplateIsNeeded asserts deployed-state-only operations and
// dry runs never contact the source server.
func TestSourceNotFetchedWhenNoLocalTemplateIsNeeded(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tests := []struct {
		name string
		run  func(t *testing.T, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error
	}{
		{name: "delete", run: resolveFor(OperationDelete)},
		{name: "output", run: resolveFor(OperationOutput)},
		{name: "drift detect", run: resolveFor(OperationDriftDetect)},
		{name: "get template", run: resolveFor(OperationGetTemplate)},
		{name: "logs", run: resolveFor(OperationLogs)},
		{name: "changeset execute without policy file", run: resolveFor(OperationChangesetExecute)},
		{name: "dry run apply", run: func(_ *testing.T, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
			info.DryRun = true
			return validateDryRun(config, info, nil, OperationApply)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests.Store(0)
			config, info := sourceFixture(t, server.URL+"/source.tar.gz")
			require.NoError(t, tt.run(t, config, info))
			assert.Zero(t, requests.Load(), "no source request expected")
		})
	}
}

// resolveFor returns a runner that resolves the spec for one operation.
func resolveFor(operation Operation) func(*testing.T, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) error {
	return func(t *testing.T, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
		_, err := resolveSpecAndTemplate(t.Context(), config, info, operation)
		return err
	}
}
