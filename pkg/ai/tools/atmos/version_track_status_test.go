package atmos

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// versionTrackTestDatasource is a fake datasource registered once for this
// package's status/update tool tests, avoiding any real network call --
// mirrors pkg/version/manager's own fake-resolver test pattern.
const versionTrackTestDatasource = "fake-ai-tool-test"

type versionTrackFakeResolver struct{}

func (versionTrackFakeResolver) Names() []string { return []string{versionTrackTestDatasource} }

func (versionTrackFakeResolver) Versions(_ context.Context, _ *resolver.Request) ([]resolver.Candidate, error) {
	now := time.Now()
	return []resolver.Candidate{
		{Version: "1.0.0", ReleasedAt: &now},
		{Version: "1.1.0", ReleasedAt: &now},
	}, nil
}

func (versionTrackFakeResolver) Pin(_ context.Context, _ *resolver.Request, version string) (string, error) {
	return "digest-" + version, nil
}

func init() {
	resolver.Register(versionTrackFakeResolver{})
}

// versionTrackFakeConfig builds an in-memory AtmosConfiguration with one
// track/entry backed by the fake resolver -- no atmos.yaml file or network
// access needed for status/update, unlike the file-editing CRUD tools.
func versionTrackFakeConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	return &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Version: schema.Version{
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"thing": {
							Datasource: versionTrackTestDatasource,
							Package:    "thing",
							Desired:    "latest",
						},
					},
				},
			},
		},
	}
}

func TestNewVersionTrackStatusTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewVersionTrackStatusTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestVersionTrackStatusTool_Name(t *testing.T) {
	tool := NewVersionTrackStatusTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_version_track_status", tool.Name())
}

func TestVersionTrackStatusTool_Description(t *testing.T) {
	tool := NewVersionTrackStatusTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestVersionTrackStatusTool_Parameters(t *testing.T) {
	tool := NewVersionTrackStatusTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, paramTrack, params[0].Name)
	assert.Equal(t, paramGroup, params[1].Name)
}

func TestVersionTrackStatusTool_RequiresPermission(t *testing.T) {
	tool := NewVersionTrackStatusTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestVersionTrackStatusTool_IsRestricted(t *testing.T) {
	tool := NewVersionTrackStatusTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestVersionTrackStatusTool_Execute(t *testing.T) {
	atmosConfig := versionTrackFakeConfig(t)
	tool := NewVersionTrackStatusTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramTrack: "prod",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Contains(t, result.Output, "thing")
	assert.Equal(t, "prod", result.Data[paramTrack])
}
