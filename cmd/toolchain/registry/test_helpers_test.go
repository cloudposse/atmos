package registry

import (
	"github.com/cloudposse/atmos/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
)

// toolStatusTestCase represents a test case for tool status checking functions.
type toolStatusTestCase struct {
	name         string
	tool         *toolchainregistry.Tool
	toolVersions *toolchain.ToolVersions
	wantInConfig bool
	wantInstall  bool
}

// getToolStatusTestCases returns common test cases for tool status checking.
func getToolStatusTestCases() []toolStatusTestCase {
	return []toolStatusTestCase{
		{
			name: "tool found by full name",
			tool: &toolchainregistry.Tool{
				RepoOwner:  "hashicorp",
				RepoName:   "terraform",
				BinaryName: "terraform",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"hashicorp/terraform": {"1.5.0"},
				},
			},
			wantInConfig: true,
			wantInstall:  false, // Not installed (no binary found).
		},
		{
			name: "tool found by repo name only",
			tool: &toolchainregistry.Tool{
				RepoOwner:  "hashicorp",
				RepoName:   "terraform",
				BinaryName: "terraform",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
				},
			},
			wantInConfig: true,
			wantInstall:  false,
		},
		{
			name: "tool not in config",
			tool: &toolchainregistry.Tool{
				RepoOwner:  "hashicorp",
				RepoName:   "terraform",
				BinaryName: "terraform",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"other/tool": {"1.0.0"},
				},
			},
			wantInConfig: false,
			wantInstall:  false,
		},
		{
			name: "empty versions list for full name",
			tool: &toolchainregistry.Tool{
				RepoOwner:  "hashicorp",
				RepoName:   "terraform",
				BinaryName: "terraform",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"hashicorp/terraform": {},
				},
			},
			wantInConfig: true,
			wantInstall:  false, // No version to check.
		},
		{
			name: "empty versions list for repo name",
			tool: &toolchainregistry.Tool{
				RepoOwner:  "hashicorp",
				RepoName:   "terraform",
				BinaryName: "terraform",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"terraform": {},
				},
			},
			wantInConfig: true,
			wantInstall:  false, // No version to check.
		},
	}
}
