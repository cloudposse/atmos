package installer

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// Compile-time sentinel: a rename of registry.Tool fields used below would silently
// break the platform-specific URL build path. Fail the build instead.
var _ = registry.Tool{Type: "github_release", Asset: "x"}

// TestBuildAssetURLForPlatform exercises the explicit-target-platform path that
// `atmos toolchain lock` uses to populate every platform the registry advertises from
// a single host. The current-platform `BuildAssetURL` delegates here, so it is also
// covered by the existing tests in asset_test.go.
func TestBuildAssetURLForPlatform(t *testing.T) {
	tests := []struct {
		name            string
		tool            *registry.Tool
		version         string
		goos            string
		goarch          string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "github_release with replacements for linux/amd64 from any host",
			tool: &registry.Tool{
				Type:      "github_release",
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Asset:     "terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
				Replacements: map[string]string{
					"amd64": "amd64", // No-op replacement, but exercises the replacement branch.
				},
				VersionPrefix: "v",
			},
			version: "1.5.7",
			goos:    "linux",
			goarch:  "amd64",
			wantContains: []string{
				"https://github.com/hashicorp/terraform/releases/download/v1.5.7/",
				"terraform_1.5.7_linux_amd64.zip",
			},
		},
		{
			name: "github_release for darwin/arm64 from a linux host",
			tool: &registry.Tool{
				Type:          "github_release",
				RepoOwner:     "hashicorp",
				RepoName:      "terraform",
				Asset:         "terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
				VersionPrefix: "v",
			},
			version: "1.5.7",
			goos:    "darwin",
			goarch:  "arm64",
			wantContains: []string{
				"terraform_1.5.7_darwin_arm64.zip",
			},
		},
		{
			name: "windows target adds .exe to raw binary even from a non-windows host",
			// jq-style raw binary template — no dots in the asset name so filepath.Ext returns "".
			// This mirrors the existing TestBuildAssetURL_WindowsExeExtensionForRawBinary.
			tool: &registry.Tool{
				Type:          "github_release",
				RepoOwner:     "jqlang",
				RepoName:      "jq",
				Asset:         "jq-{{.OS}}-{{.Arch}}",
				VersionPrefix: "jq-",
			},
			version: "1.7.1",
			goos:    "windows",
			goarch:  "amd64",
			wantContains: []string{
				"jq-windows-amd64.exe",
			},
		},
		{
			name: "non-windows target does not add .exe to raw binary",
			tool: &registry.Tool{
				Type:          "github_release",
				RepoOwner:     "jqlang",
				RepoName:      "jq",
				Asset:         "jq-{{.OS}}-{{.Arch}}",
				VersionPrefix: "jq-",
			},
			version: "1.7.1",
			goos:    "linux",
			goarch:  "amd64",
			wantContains: []string{
				"jq-linux-amd64",
			},
			wantNotContains: []string{".exe"},
		},
		{
			name: "rosetta2 fallback applies even when caller asks for darwin/arm64",
			tool: &registry.Tool{
				Type:      "github_release",
				RepoOwner: "x",
				RepoName:  "y",
				Asset:     "y_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
				Rosetta2:  true,
			},
			version: "1.0.0",
			goos:    "darwin",
			goarch:  "arm64",
			wantContains: []string{
				"y_1.0.0_darwin_amd64.tar.gz", // arm64 -> amd64 via Rosetta 2.
			},
		},
		{
			name: "http type with template uses target OS/arch",
			tool: &registry.Tool{
				Type:      "http",
				RepoOwner: "aws",
				RepoName:  "aws-cli",
				Asset:     "https://awscli.amazonaws.com/awscli-exe-{{.OS}}-{{.Arch}}-{{.SemVer}}.zip",
				Replacements: map[string]string{
					"amd64": "x86_64",
				},
			},
			version: "2.15.0",
			goos:    "linux",
			goarch:  "amd64",
			wantContains: []string{
				"awscli-exe-linux-x86_64-2.15.0.zip",
			},
		},
		{
			name: "format override picked by target GOOS, not host GOOS",
			tool: &registry.Tool{
				Type:      "github_release",
				RepoOwner: "x",
				RepoName:  "y",
				Asset:     "y_{{.Version}}_{{.OS}}_{{.Arch}}.{{.Format}}",
				Format:    "tar.gz",
				FormatOverrides: []registry.FormatOverride{
					{GOOS: "windows", Format: "zip"},
				},
			},
			version: "1.0.0",
			goos:    "windows",
			goarch:  "amd64",
			wantContains: []string{
				"y_1.0.0_windows_amd64.zip",
			},
			wantNotContains: []string{".tar.gz"},
		},
		{
			name: "overrides[] block for the target platform is applied",
			tool: &registry.Tool{
				Type:      "github_release",
				RepoOwner: "x",
				RepoName:  "y",
				Asset:     "default_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
				Overrides: []registry.Override{
					{
						GOOS:   "windows",
						GOARCH: "amd64",
						Asset:  "windows_special_{{.Version}}.zip",
					},
				},
			},
			version: "1.0.0",
			goos:    "windows",
			goarch:  "amd64",
			wantContains: []string{
				"windows_special_1.0.0.zip",
			},
		},
	}

	installer := &Installer{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, err := installer.BuildAssetURLForPlatform(tc.tool, tc.version, tc.goos, tc.goarch)
			require.NoError(t, err)
			for _, want := range tc.wantContains {
				assert.Contains(t, url, want)
			}
			for _, dontWant := range tc.wantNotContains {
				assert.NotContains(t, url, dontWant)
			}
		})
	}
}

// TestBuildAssetURLForPlatform_DoesNotMutateInputTool guards the contract that callers
// can invoke this for many platforms in a row without corrupting their tool reference.
// This matters for `atmos toolchain lock` which calls it once per advertised platform.
func TestBuildAssetURLForPlatform_DoesNotMutateInputTool(t *testing.T) {
	tool := &registry.Tool{
		Type:      "github_release",
		RepoOwner: "x",
		RepoName:  "y",
		Asset:     "default_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
		Format:    "tar.gz",
		Overrides: []registry.Override{
			{
				GOOS:   "windows",
				GOARCH: "amd64",
				Asset:  "windows_{{.Version}}.zip",
				Format: "zip",
			},
		},
	}
	installer := &Installer{}

	// Bidirectional isolation check per CLAUDE.md slice/test rule.
	originalAsset := tool.Asset
	originalFormat := tool.Format
	originalOverrideCount := len(tool.Overrides)

	_, err := installer.BuildAssetURLForPlatform(tool, "1.0.0", "windows", "amd64")
	require.NoError(t, err)

	// After the call, the caller's tool reference must be unchanged.
	assert.Equal(t, originalAsset, tool.Asset, "Asset must not be mutated")
	assert.Equal(t, originalFormat, tool.Format, "Format must not be mutated")
	assert.Equal(t, originalOverrideCount, len(tool.Overrides), "Overrides slice must not be mutated")
}

// TestBuildAssetURL_DelegatesToPlatformPath proves the existing BuildAssetURL API still
// works as before (back-compat contract: it just uses runtime.GOOS / runtime.GOARCH).
func TestBuildAssetURL_DelegatesToPlatformPath(t *testing.T) {
	tool := &registry.Tool{
		Type:          "github_release",
		RepoOwner:     "hashicorp",
		RepoName:      "terraform",
		Asset:         "terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
		VersionPrefix: "v",
	}
	installer := &Installer{}

	got, err := installer.BuildAssetURL(tool, "1.5.7")
	require.NoError(t, err)
	want, err := installer.BuildAssetURLForPlatform(tool, "1.5.7", runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err)
	assert.Equal(t, want, got, "BuildAssetURL must delegate to BuildAssetURLForPlatform with runtime values")
}

// TestBuildAssetURLForPlatform_UnsupportedType verifies the error path.
func TestBuildAssetURLForPlatform_UnsupportedType(t *testing.T) {
	tool := &registry.Tool{Type: "unknown"}
	installer := &Installer{}
	_, err := installer.BuildAssetURLForPlatform(tool, "1.0.0", "linux", "amd64")
	require.Error(t, err)
}
