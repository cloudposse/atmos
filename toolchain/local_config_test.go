package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetToolWithVersion(t *testing.T) {
	lcm := &LocalConfigManager{
		config: &LocalConfig{
			Tools: map[string]LocalTool{
				"owner1/repo1": {
					Type:       "github_release",
					RepoOwner:  "owner1",
					RepoName:   "repo1",
					Asset:      "asset1",
					Format:     "tar.gz",
					BinaryName: "bin1",
				},
				"owner2/repo2": {
					Type:       "github_release",
					RepoOwner:  "owner2",
					RepoName:   "repo2",
					Asset:      "asset2",
					Format:     "zip",
					BinaryName: "bin2",
					VersionConstraints: []LocalVersionConstraint{
						{Constraint: ">= 2.0.0", Asset: "asset2-2.x", Format: "tar.gz", BinaryName: "bin2-2.x"},
						{Constraint: "< 2.0.0", Asset: "asset2-1.x", Format: "zip", BinaryName: "bin2-1.x"},
					},
				},
			},
		},
	}

	tests := []struct {
		name       string
		owner      string
		repo       string
		version    string
		wantErr    bool
		wantAsset  string
		wantFormat string
		wantBinary string
	}{
		{
			name:  "tool found, no constraints",
			owner: "owner1", repo: "repo1", version: "1.0.0",
			wantErr: false, wantAsset: "asset1", wantFormat: "tar.gz", wantBinary: "bin1",
		},
		{
			name:  "tool found, matching >= constraint",
			owner: "owner2", repo: "repo2", version: "2.1.0",
			wantErr: false, wantAsset: "asset2-2.x", wantFormat: "tar.gz", wantBinary: "bin2-2.x",
		},
		{
			name:  "tool found, matching < constraint",
			owner: "owner2", repo: "repo2", version: "1.5.0",
			wantErr: false, wantAsset: "asset2-1.x", wantFormat: "zip", wantBinary: "bin2-1.x",
		},
		{
			name:  "tool not found",
			owner: "ownerX", repo: "repoX", version: "1.0.0",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := lcm.GetToolWithVersion(tc.owner, tc.repo, tc.version)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantAsset, tool.Asset)
			assert.Equal(t, tc.wantFormat, tool.Format)
			assert.Equal(t, tc.wantBinary, tool.Name)
		})
	}
}

func TestGetToolWithVersion_BinaryNameAndHttpType(t *testing.T) {
	lcm := &LocalConfigManager{
		config: &LocalConfig{
			Tools: map[string]LocalTool{
				"owner1/repo1": {
					Type:       "github_release",
					RepoOwner:  "owner1",
					RepoName:   "repo1",
					Asset:      "asset1",
					Format:     "tar.gz",
					BinaryName: "bin1",
				},
				"owner2/repo2": {
					Type:       "github_release",
					RepoOwner:  "owner2",
					RepoName:   "repo2",
					Asset:      "asset2",
					Format:     "zip",
					BinaryName: "bin2",
					VersionConstraints: []LocalVersionConstraint{
						{Constraint: ">= 2.0.0", Asset: "asset2-2.x", Format: "tar.gz", BinaryName: "bin2-2.x"},
						{Constraint: "< 2.0.0", Asset: "asset2-1.x", Format: "zip", BinaryName: "bin2-1.x"},
					},
				},
				"owner3/repo3": {
					Type:      "http",
					RepoOwner: "owner3",
					RepoName:  "repo3",
					URL:       "https://example.com/repo3.tar.gz",
					Format:    "tar.gz",
				},
			},
		},
	}

	tests := []struct {
		name       string
		owner      string
		repo       string
		version    string
		wantName   string
		wantAsset  string
		wantFormat string
	}{
		{
			name:  "tool-level binary_name respected",
			owner: "owner1", repo: "repo1", version: "1.0.0",
			wantName: "bin1", wantAsset: "asset1", wantFormat: "tar.gz",
		},
		{
			name:  "constraint binary_name overrides tool-level",
			owner: "owner2", repo: "repo2", version: "2.1.0",
			wantName: "bin2-2.x", wantAsset: "asset2-2.x", wantFormat: "tar.gz",
		},
		{
			name:  "http type uses url for asset",
			owner: "owner3", repo: "repo3", version: "1.0.0",
			wantName: "repo3", wantAsset: "https://example.com/repo3.tar.gz", wantFormat: "tar.gz",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := lcm.GetToolWithVersion(tc.owner, tc.repo, tc.version)
			assert.NoError(t, err)
			assert.Equal(t, tc.wantName, tool.Name)
			assert.Equal(t, tc.wantAsset, tool.Asset)
			assert.Equal(t, tc.wantFormat, tool.Format)
		})
	}
}
