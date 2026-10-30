package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/pkg/toolchain/registry"
)

var (
	_ = schema.AtmosConfiguration{Toolchain: schema.Toolchain{}}
	_ = schema.Toolchain{Registries: []schema.ToolchainRegistry{}}
	_ = schema.ToolchainRegistry{Name: "", Type: "", Tools: map[string]any{}}
)

func TestListConfiguredNamedRegistry(t *testing.T) {
	old := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(old) })
	toolchain.SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{Registries: []schema.ToolchainRegistry{{Name: "local", Type: "atmos", Tools: map[string]any{"local/test": map[string]any{"type": "http", "url": "https://example.invalid/tool", "format": "raw"}}}}}})
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	output := &bytes.Buffer{}
	streams := &registryListStreams{output: output}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)

	require.NoError(t, listRegistryTools(context.Background(), "local", &ListOptions{Format: "json", Limit: 50, Sort: "name"}))
	var tools []*toolchainregistry.Tool
	require.NoError(t, json.Unmarshal(output.Bytes(), &tools))
	require.Len(t, tools, 1)
	require.Equal(t, "local", tools[0].RepoOwner)
	require.Equal(t, "test", tools[0].RepoName)
	output.Reset()
	require.NoError(t, listConfiguredRegistries(context.Background(), &ListOptions{Format: "json"}))
	var registries []toolchainregistry.ConfiguredRegistryInfo
	require.NoError(t, json.Unmarshal(output.Bytes(), &registries))
	require.Contains(t, registries, toolchainregistry.ConfiguredRegistryInfo{Name: "local", Type: "atmos"})
	require.ErrorIs(t, listRegistryTools(context.Background(), "missing", &ListOptions{Format: "json", Limit: 50}), toolchainregistry.ErrUnknownRegistry)
}

type registryListStreams struct{ output *bytes.Buffer }

func (s *registryListStreams) Input() io.Reader     { return nil }
func (s *registryListStreams) Output() io.Writer    { return s.output }
func (s *registryListStreams) Error() io.Writer     { return io.Discard }
func (s *registryListStreams) RawOutput() io.Writer { return s.output }
func (s *registryListStreams) RawError() io.Writer  { return io.Discard }
