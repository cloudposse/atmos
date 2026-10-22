package toolchain

import (
	"encoding/json"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllConfiguredTools(t *testing.T) {
	original := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(original) })
	file := createTempToolVersionsFile(t, "zeta/second 2.0.0\nalpha/first 1.2.0 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: file}})
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	for _, format := range []string{"json", "plain"} {
		t.Run(format, func(t *testing.T) {
			output := captureDataOutput(t)
			require.NoError(t, ListToolVersions(false, 10, "", format))
			if format == "plain" {
				assert.Equal(t, "alpha/first 1.2.0\nzeta/second 2.0.0\n", output.String())
				return
			}
			var got []ToolVersionInfo
			require.NoError(t, json.Unmarshal(output.Bytes(), &got))
			require.Len(t, got, 2)
			assert.Equal(t, "alpha/first", got[0].Tool)
			assert.Equal(t, "1.2.0", got[0].Version)
			assert.Equal(t, "zeta/second", got[1].Tool)
		})
	}
}

func TestGetAllConfiguredToolsAvailableVersions(t *testing.T) {
	original := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(original) })
	file := createTempToolVersionsFile(t, "alpha/first 1.0.0\nzeta/second 2.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: file}})
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	mock := NewMockGitHubAPI()
	mock.SetReleases("alpha", "first", []string{"1.0.0", "1.2.0"})
	mock.SetReleases("zeta", "second", []string{"2.0.0", "2.1.0"})
	limited := &configuredGetLimitAPI{MockGitHubAPI: mock}
	SetGitHubAPI(limited)
	t.Cleanup(ResetGitHubAPI)
	output := captureDataOutput(t)
	require.NoError(t, ListToolVersions(true, 1, "", "json"))
	var got []ToolVersionList
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Len(t, got[0].Versions, 1)
	assert.Len(t, got[1].Versions, 1)
	assert.Equal(t, []int{1, 1}, limited.limits)
}

type configuredGetLimitAPI struct {
	*MockGitHubAPI
	limits []int
}

func (m *configuredGetLimitAPI) FetchReleases(owner, repo string, limit int) ([]string, error) {
	m.limits = append(m.limits, limit)
	releases, err := m.MockGitHubAPI.FetchReleases(owner, repo, limit)
	if limit > 0 && len(releases) > limit {
		releases = releases[:limit]
	}
	return releases, err
}
