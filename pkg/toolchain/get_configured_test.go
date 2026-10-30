package toolchain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	_ = schema.AtmosConfiguration{Toolchain: schema.Toolchain{}}
	_ = schema.Toolchain{VersionsFile: "", Aliases: map[string]string{}}
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
	require.Len(t, got[0].Versions, 1)
	assert.Equal(t, "alpha/first", got[0].Tool)
	assert.Equal(t, "1.0.0", got[0].Versions[0].Version)
	require.Len(t, got[1].Versions, 1)
	assert.Equal(t, "zeta/second", got[1].Tool)
	assert.Equal(t, "2.0.0", got[1].Versions[0].Version)
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

func TestGetAllConfiguredToolsKeepsAliasVersions(t *testing.T) {
	original := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(original) })
	file := createTempToolVersionsFile(t, "terraform 1.6.0\nhashicorp/terraform 1.8.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		VersionsFile: file,
		Aliases:      map[string]string{"terraform": "hashicorp/terraform"},
	}})
	installVersionForTest(t, "hashicorp", "terraform", "1.6.0")
	mock := NewMockGitHubAPI()
	mock.SetReleases("hashicorp", "terraform", []string{"1.6.0", "1.8.0"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)
	t.Run("plain", func(t *testing.T) {
		output := captureDataOutput(t)
		require.NoError(t, ListToolVersions(false, 10, "", "plain"))
		assert.Equal(t, "hashicorp/terraform 1.8.0\nterraform 1.6.0\n", output.String())
	})
	for _, showAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("all=%t", showAll), func(t *testing.T) {
			output := captureDataOutput(t)
			require.NoError(t, ListToolVersions(showAll, 10, "", "json"))
			if !showAll {
				var got []ToolVersionInfo
				require.NoError(t, json.Unmarshal(output.Bytes(), &got))
				require.Len(t, got, 2)
				assert.Equal(t, []ToolVersionInfo{
					{Tool: "hashicorp/terraform", Version: "1.8.0", Installed: false},
					{Tool: "terraform", Version: "1.6.0", Installed: true},
				}, got)
				return
			}
			var got []ToolVersionList
			require.NoError(t, json.Unmarshal(output.Bytes(), &got))
			require.Len(t, got, 2)
			for i, expected := range []string{"1.8.0", "1.6.0"} {
				assert.Equal(t, []string{"hashicorp/terraform", "terraform"}[i], got[i].Tool)
				assert.Equal(t, expected, got[i].DefaultVersion)
				require.Len(t, got[i].Versions, 2)
				assert.Equal(t, "1.6.0", got[i].Versions[0].Version)
				assert.Equal(t, "1.8.0", got[i].Versions[1].Version)
				for _, version := range got[i].Versions {
					assert.Equal(t, version.Version == expected, version.Default)
					assert.Equal(t, version.Version == "1.6.0", version.Installed)
				}
			}
		})
	}
	t.Run("named tool remains canonical", func(t *testing.T) {
		output := captureDataOutput(t)
		require.NoError(t, ListToolVersions(false, 10, "terraform", "json"))
		var got ToolVersionInfo
		require.NoError(t, json.Unmarshal(output.Bytes(), &got))
		assert.Equal(t, ToolVersionInfo{Tool: "hashicorp/terraform", Version: "1.8.0"}, got)
		output.Reset()
		require.NoError(t, ListToolVersions(false, 10, "terraform", "plain"))
		assert.Equal(t, "1.8.0\n", output.String())
	})
}

func TestGetConfiguredToolsTable(t *testing.T) {
	original := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(original) })
	file := createTempToolVersionsFile(t, "zeta/second 2.0.0\nalpha/first 1.2.0 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: file}})
	installVersionForTest(t, "alpha", "first", "1.2.0")
	output := captureUITestOutput(t, func() {
		require.NoError(t, ListToolVersions(false, 10, "", "table"))
	})
	assert.Contains(t, output, "alpha/first:")
	assert.Contains(t, output, "zeta/second:")
	assert.Less(t, strings.Index(output, "alpha/first:"), strings.Index(output, "zeta/second:"))
	for _, version := range []string{"1.0.0", "1.2.0", "2.0.0"} {
		assert.Contains(t, output, version)
	}
}

func TestGetConfiguredToolsErrors(t *testing.T) {
	original := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(original) })
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	for _, tc := range []struct {
		name, content string
		want          error
	}{
		{"empty", "", ErrNoVersionsFound},
		{"invalid name", "invalid/name/extra 1.0.0\n", ErrInvalidToolSpec},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := createTempToolVersionsFile(t, tc.content)
			SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: file}})
			output := captureDataOutput(t)
			require.ErrorIs(t, ListToolVersions(false, 10, "", "json"), tc.want)
			assert.Empty(t, output.String())
		})
	}
	t.Run("unreadable versions file", func(t *testing.T) {
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: t.TempDir()}})
		output := captureDataOutput(t)
		require.ErrorContains(t, ListToolVersions(false, 10, "", "json"), "failed to load .tool-versions")
		assert.Empty(t, output.String())
	})
	t.Run("release lookup fails", func(t *testing.T) {
		file := createTempToolVersionsFile(t, "alpha/first 1.0.0\n")
		SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: file}})
		mock := NewMockGitHubAPI()
		mock.SetError("alpha", "first", ErrNoVersionsFound)
		SetGitHubAPI(mock)
		t.Cleanup(ResetGitHubAPI)
		output := captureDataOutput(t)
		err := ListToolVersions(true, 10, "", "json")
		require.ErrorIs(t, err, ErrNoVersionsFound)
		assert.Contains(t, err.Error(), "failed to fetch versions from GitHub")
		assert.Empty(t, output.String())
	})
}

type configuredOutputFailure struct{}

func (configuredOutputFailure) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestGetConfiguredToolsOutputFailure(t *testing.T) {
	original := GetAtmosConfig()
	t.Cleanup(func() { SetAtmosConfig(original) })
	file := createTempToolVersionsFile(t, "alpha/first 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: file}})
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: configuredOutputFailure{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	for _, format := range []string{"plain", "json"} {
		require.ErrorIs(t, ListToolVersions(false, 10, "", format), io.ErrClosedPipe)
	}
}
