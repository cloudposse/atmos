package installer

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

func TestMatchesPlatform(t *testing.T) {
	tests := []struct {
		name         string
		overrideGOOS string
		overrideArch string
		goos         string
		goarch       string
		expected     bool
	}{
		{
			name:         "exact match darwin/amd64",
			overrideGOOS: "darwin",
			overrideArch: "amd64",
			goos:         "darwin",
			goarch:       "amd64",
			expected:     true,
		},
		{
			name:         "exact match linux/arm64",
			overrideGOOS: "linux",
			overrideArch: "arm64",
			goos:         "linux",
			goarch:       "arm64",
			expected:     true,
		},
		{
			name:         "wildcard GOOS matches any OS",
			overrideGOOS: "",
			overrideArch: "amd64",
			goos:         "darwin",
			goarch:       "amd64",
			expected:     true,
		},
		{
			name:         "wildcard GOARCH matches any arch",
			overrideGOOS: "linux",
			overrideArch: "",
			goos:         "linux",
			goarch:       "arm64",
			expected:     true,
		},
		{
			name:         "both wildcards match everything",
			overrideGOOS: "",
			overrideArch: "",
			goos:         "windows",
			goarch:       "386",
			expected:     true,
		},
		{
			name:         "GOOS mismatch",
			overrideGOOS: "darwin",
			overrideArch: "amd64",
			goos:         "linux",
			goarch:       "amd64",
			expected:     false,
		},
		{
			name:         "GOARCH mismatch",
			overrideGOOS: "darwin",
			overrideArch: "amd64",
			goos:         "darwin",
			goarch:       "arm64",
			expected:     false,
		},
		{
			name:         "both mismatch",
			overrideGOOS: "darwin",
			overrideArch: "amd64",
			goos:         "linux",
			goarch:       "arm64",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPlatform(tt.overrideGOOS, tt.overrideArch, tt.goos, tt.goarch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyOverride(t *testing.T) {
	tests := []struct {
		name          string
		tool          *registry.Tool
		override      registry.Override
		expectedAsset string
		expectedFmt   string
		expectedFiles []registry.File
		expectedRepl  map[string]string
	}{
		{
			name: "apply asset only",
			tool: &registry.Tool{
				Asset:  "original-asset",
				Format: "tar.gz",
			},
			override: registry.Override{
				Asset: "new-asset.zip",
			},
			expectedAsset: "new-asset.zip",
			expectedFmt:   "tar.gz",
		},
		{
			name: "apply format only",
			tool: &registry.Tool{
				Asset:  "original-asset",
				Format: "tar.gz",
			},
			override: registry.Override{
				Format: "pkg",
			},
			expectedAsset: "original-asset",
			expectedFmt:   "pkg",
		},
		{
			name: "apply files",
			tool: &registry.Tool{
				Asset: "original-asset",
			},
			override: registry.Override{
				Files: []registry.File{
					{Name: "aws", Src: "aws/dist/aws"},
					{Name: "aws_completer", Src: "aws/dist/aws_completer"},
				},
			},
			expectedAsset: "original-asset",
			expectedFiles: []registry.File{
				{Name: "aws", Src: "aws/dist/aws"},
				{Name: "aws_completer", Src: "aws/dist/aws_completer"},
			},
		},
		{
			name: "apply replacements to empty map",
			tool: &registry.Tool{
				Asset: "original-asset",
			},
			override: registry.Override{
				Replacements: map[string]string{
					"amd64": "x86_64",
					"arm64": "aarch64",
				},
			},
			expectedAsset: "original-asset",
			expectedRepl: map[string]string{
				"amd64": "x86_64",
				"arm64": "aarch64",
			},
		},
		{
			name: "merge replacements with existing",
			tool: &registry.Tool{
				Asset: "original-asset",
				Replacements: map[string]string{
					"darwin": "macos",
					"amd64":  "old_value",
				},
			},
			override: registry.Override{
				Replacements: map[string]string{
					"amd64": "x86_64",
				},
			},
			expectedAsset: "original-asset",
			expectedRepl: map[string]string{
				"darwin": "macos",
				"amd64":  "x86_64", // Override takes precedence.
			},
		},
		{
			name: "apply all fields",
			tool: &registry.Tool{
				Asset:  "original-asset",
				Format: "tar.gz",
			},
			override: registry.Override{
				Asset:  "new-asset.pkg",
				Format: "pkg",
				Files: []registry.File{
					{Name: "binary", Src: "payload/binary"},
				},
				Replacements: map[string]string{
					"darwin": "macos",
				},
			},
			expectedAsset: "new-asset.pkg",
			expectedFmt:   "pkg",
			expectedFiles: []registry.File{
				{Name: "binary", Src: "payload/binary"},
			},
			expectedRepl: map[string]string{
				"darwin": "macos",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyOverride(tt.tool, &tt.override)

			assert.Equal(t, tt.expectedAsset, tt.tool.Asset)
			if tt.expectedFmt != "" {
				assert.Equal(t, tt.expectedFmt, tt.tool.Format)
			}
			if tt.expectedFiles != nil {
				assert.Equal(t, tt.expectedFiles, tt.tool.Files)
			}
			if tt.expectedRepl != nil {
				assert.Equal(t, tt.expectedRepl, tt.tool.Replacements)
			}
		})
	}
}

func TestApplyPlatformOverrides_NoOverrides(t *testing.T) {
	tool := &registry.Tool{
		Asset:  "original-asset",
		Format: "tar.gz",
	}

	ApplyPlatformOverrides(tool)

	assert.Equal(t, "original-asset", tool.Asset)
	assert.Equal(t, "tar.gz", tool.Format)
}

func TestApplyPlatformOverrides_FirstMatchWins(t *testing.T) {
	// Create overrides where both could potentially match, but first should win.
	tool := &registry.Tool{
		Asset:  "original-asset",
		Format: "tar.gz",
		Overrides: []registry.Override{
			{
				GOOS:   runtime.GOOS, // Match current OS.
				GOARCH: runtime.GOARCH,
				Asset:  "first-match-asset",
				Format: "first",
			},
			{
				GOOS:   runtime.GOOS, // Also matches current OS.
				GOARCH: "",           // Wildcard arch.
				Asset:  "second-match-asset",
				Format: "second",
			},
		},
	}

	ApplyPlatformOverrides(tool)

	assert.Equal(t, "first-match-asset", tool.Asset)
	assert.Equal(t, "first", tool.Format)
}

func TestApplyPlatformOverrides_NoMatchingOverride(t *testing.T) {
	tool := &registry.Tool{
		Asset:  "original-asset",
		Format: "tar.gz",
		Overrides: []registry.Override{
			{
				GOOS:   "nonexistent-os",
				GOARCH: "nonexistent-arch",
				Asset:  "should-not-apply",
				Format: "should-not-apply",
			},
		},
	}

	ApplyPlatformOverrides(tool)

	assert.Equal(t, "original-asset", tool.Asset)
	assert.Equal(t, "tar.gz", tool.Format)
}

func TestApplyPlatformOverrides_WildcardGOOSMatch(t *testing.T) {
	tool := &registry.Tool{
		Asset:  "original-asset",
		Format: "tar.gz",
		Overrides: []registry.Override{
			{
				GOOS:   "",             // Wildcard OS.
				GOARCH: runtime.GOARCH, // Match current arch.
				Asset:  "wildcard-os-asset",
			},
		},
	}

	ApplyPlatformOverrides(tool)

	assert.Equal(t, "wildcard-os-asset", tool.Asset)
}

func TestApplyPlatformOverrides_WildcardGOARCHMatch(t *testing.T) {
	tool := &registry.Tool{
		Asset:  "original-asset",
		Format: "tar.gz",
		Overrides: []registry.Override{
			{
				GOOS:   runtime.GOOS, // Match current OS.
				GOARCH: "",           // Wildcard arch.
				Asset:  "wildcard-arch-asset",
			},
		},
	}

	ApplyPlatformOverrides(tool)

	assert.Equal(t, "wildcard-arch-asset", tool.Asset)
}
