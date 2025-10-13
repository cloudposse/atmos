//go:build !windows
// +build !windows

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJoinPath_UnixEdgeCases tests comprehensive Unix/Linux/macOS path edge cases.
func TestJoinPath_UnixEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		providedPath string
		expected     string
		description  string
	}{
		// ============ STANDARD UNIX PATHS ============
		{
			name:         "GitHub Actions Unix - both absolute",
			basePath:     "/home/runner/_work/infrastructure/infrastructure",
			providedPath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			expected:     "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			description:  "Should not duplicate when both paths are absolute",
		},
		{
			name:         "GitHub Actions Unix - relative component",
			basePath:     "/home/runner/_work/infrastructure/infrastructure",
			providedPath: "atmos/components/terraform",
			expected:     "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			description:  "Should join properly with relative path",
		},

		// ============ ROOT AND HOME PATHS ============
		{
			name:         "Root path as base",
			basePath:     "/",
			providedPath: "usr/local/bin",
			expected:     "/usr/local/bin",
			description:  "Root directory joining",
		},
		{
			name:         "Root with absolute provided",
			basePath:     "/",
			providedPath: "/etc/config",
			expected:     "/etc/config",
			description:  "Absolute overrides root",
		},
		{
			name:         "Home directory tilde (not expanded)",
			basePath:     "~/projects",
			providedPath: "components/terraform",
			expected:     "~/projects/components/terraform",
			description:  "Tilde not expanded by filepath",
		},
		{
			name:         "Absolute home path",
			basePath:     "/home/user",
			providedPath: ".config/atmos",
			expected:     "/home/user/.config/atmos",
			description:  "Hidden directory in home",
		},

		// ============ HIDDEN FILES AND DIRECTORIES ============
		{
			name:         "Hidden directory as base",
			basePath:     "/home/user/.local",
			providedPath: "share/atmos",
			expected:     "/home/user/.local/share/atmos",
			description:  "Hidden directory paths",
		},
		{
			name:         "Multiple hidden directories",
			basePath:     "/home/user/.config",
			providedPath: ".atmos/.cache",
			expected:     "/home/user/.config/.atmos/.cache",
			description:  "Multiple dots in path",
		},

		// ============ PATHS WITH SPACES AND SPECIAL CHARS ============
		{
			name:         "Path with spaces",
			basePath:     "/home/user/My Documents",
			providedPath: "Project Files/terraform",
			expected:     "/home/user/My Documents/Project Files/terraform",
			description:  "Spaces in Unix paths",
		},
		{
			name:         "Path with special chars",
			basePath:     "/home/user/project-2024",
			providedPath: "components_v1.2/terraform",
			expected:     "/home/user/project-2024/components_v1.2/terraform",
			description:  "Dashes and underscores",
		},
		{
			name:         "Path with @ symbol",
			basePath:     "/home/user@domain",
			providedPath: "projects/terraform",
			expected:     "/home/user@domain/projects/terraform",
			description:  "@ symbol in path",
		},
		{
			name:         "Path with parentheses",
			basePath:     "/opt/app(beta)",
			providedPath: "config/settings",
			expected:     "/opt/app(beta)/config/settings",
			description:  "Parentheses in path",
		},

		// ============ TRAILING AND MULTIPLE SEPARATORS ============
		{
			name:         "Trailing slash on base",
			basePath:     "/home/user/project/",
			providedPath: "components/terraform",
			expected:     "/home/user/project/components/terraform",
			description:  "Trailing slash handled",
		},
		{
			name:         "Leading slash on relative",
			basePath:     "/home/user/project",
			providedPath: "/components/terraform",
			expected:     "/components/terraform",
			description:  "Leading slash makes it absolute",
		},
		{
			name:         "Multiple slashes",
			basePath:     "/home//user///project",
			providedPath: "components//terraform",
			expected:     "/home/user/project/components/terraform",
			description:  "filepath.Join normalizes multiple slashes",
		},

		// ============ RELATIVE PATH COMPONENTS ============
		{
			name:         "Current directory dot",
			basePath:     "/home/user/project",
			providedPath: "./components/terraform",
			expected:     "/home/user/project/components/terraform",
			description:  "filepath.Join normalizes ./",
		},
		{
			name:         "Parent directory dots",
			basePath:     "/home/user/project/subfolder",
			providedPath: "../components/terraform",
			expected:     "/home/user/project/components/terraform",
			description:  "filepath.Join resolves .. paths",
		},
		{
			name:         "Multiple parent directories",
			basePath:     "/a/b/c/d",
			providedPath: "../../components",
			expected:     "/a/b/components",
			description:  "filepath.Join resolves multiple .. paths",
		},

		// ============ SYMLINK PATHS (PATH ONLY, NOT RESOLVED) ============
		{
			name:         "Common symlink paths",
			basePath:     "/usr/local",
			providedPath: "bin/atmos",
			expected:     "/usr/local/bin/atmos",
			description:  "Common symlink location",
		},
		{
			name:         "Opt directory",
			basePath:     "/opt",
			providedPath: "atmos/components",
			expected:     "/opt/atmos/components",
			description:  "Opt directory path",
		},

		// ============ SYSTEM PATHS ============
		{
			name:         "Proc filesystem",
			basePath:     "/proc/self",
			providedPath: "fd/1",
			expected:     "/proc/self/fd/1",
			description:  "Proc filesystem paths",
		},
		{
			name:         "Dev filesystem",
			basePath:     "/dev",
			providedPath: "null",
			expected:     "/dev/null",
			description:  "Device paths",
		},
		{
			name:         "Sys filesystem",
			basePath:     "/sys/class",
			providedPath: "block/sda",
			expected:     "/sys/class/block/sda",
			description:  "Sys filesystem paths",
		},
		{
			name:         "Tmp directory",
			basePath:     "/tmp",
			providedPath: "atmos-12345/work",
			expected:     "/tmp/atmos-12345/work",
			description:  "Temporary directory paths",
		},

		// ============ ENVIRONMENT VARIABLES (NOT EXPANDED) ============
		{
			name:         "Environment variable in path",
			basePath:     "$HOME/projects",
			providedPath: "components/terraform",
			expected:     "$HOME/projects/components/terraform",
			description:  "Environment variables not expanded",
		},
		{
			name:         "Multiple env vars",
			basePath:     "$HOME/$USER",
			providedPath: "projects",
			expected:     "$HOME/$USER/projects",
			description:  "Multiple variables preserved",
		},
		{
			name:         "Curly brace syntax",
			basePath:     "${HOME}/projects",
			providedPath: "terraform",
			expected:     "${HOME}/projects/terraform",
			description:  "Bash-style variable syntax",
		},

		// ============ DOCKER AND CONTAINER PATHS ============
		{
			name:         "Docker volume mount",
			basePath:     "/var/lib/docker/volumes",
			providedPath: "myvolume/_data",
			expected:     "/var/lib/docker/volumes/myvolume/_data",
			description:  "Docker volume paths",
		},
		{
			name:         "Container path",
			basePath:     "/container/app",
			providedPath: "config/settings.yaml",
			expected:     "/container/app/config/settings.yaml",
			description:  "Container internal paths",
		},

		// ============ NETWORK PATHS (NFS, SMB via CIFS) ============
		{
			name:         "NFS mount point",
			basePath:     "/mnt/nfs/server",
			providedPath: "shared/components",
			expected:     "/mnt/nfs/server/shared/components",
			description:  "NFS mount paths",
		},
		{
			name:         "SMB mount point",
			basePath:     "/mnt/smb/share",
			providedPath: "project/terraform",
			expected:     "/mnt/smb/share/project/terraform",
			description:  "SMB/CIFS mount paths",
		},

		// ============ MACOS SPECIFIC PATHS ============
		{
			name:         "macOS Applications",
			basePath:     "/Applications",
			providedPath: "Atmos.app/Contents",
			expected:     "/Applications/Atmos.app/Contents",
			description:  "macOS application bundle",
		},
		{
			name:         "macOS Library",
			basePath:     "/Users/user/Library",
			providedPath: "Application Support/atmos",
			expected:     "/Users/user/Library/Application Support/atmos",
			description:  "macOS Library paths with spaces",
		},
		{
			name:         "macOS Volumes",
			basePath:     "/Volumes/External Drive",
			providedPath: "projects/terraform",
			expected:     "/Volumes/External Drive/projects/terraform",
			description:  "macOS external volume",
		},

		// ============ EDGE CASES ============
		{
			name:         "Empty provided path",
			basePath:     "/home/user/project",
			providedPath: "",
			expected:     "/home/user/project",
			description:  "Empty provided returns base",
		},
		{
			name:         "Empty base path",
			basePath:     "",
			providedPath: "/absolute/path",
			expected:     "/absolute/path",
			description:  "Empty base with absolute provided",
		},
		{
			name:         "Both paths empty",
			basePath:     "",
			providedPath: "",
			expected:     "",
			description:  "Both empty returns empty string",
		},
		{
			name:         "Same absolute paths",
			basePath:     "/home/user/project",
			providedPath: "/home/user/project",
			expected:     "/home/user/project",
			description:  "Identical paths return as-is",
		},
		{
			name:         "Path with newline (unusual but valid)",
			basePath:     "/home/user",
			providedPath: "file\nname",
			expected:     "/home/user/file\nname",
			description:  "Newline in filename",
		},
		{
			name:         "Path with tab",
			basePath:     "/home/user",
			providedPath: "file\tname",
			expected:     "/home/user/file\tname",
			description:  "Tab in filename",
		},
		{
			name:         "Very long path component",
			basePath:     "/home/user",
			providedPath: "very_long_directory_name_that_exceeds_normal_expectations_but_is_still_valid_in_most_filesystems/terraform",
			expected:     "/home/user/very_long_directory_name_that_exceeds_normal_expectations_but_is_still_valid_in_most_filesystems/terraform",
			description:  "Long directory names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.basePath, tt.providedPath)
			assert.Equal(t, tt.expected, result, tt.description)

			// Additional validation for path duplication
			if tt.basePath != "" && tt.providedPath != "" {
				// Check for the specific GitHub Actions duplication bug
				if tt.basePath == "/home/runner/_work/infrastructure/infrastructure" &&
					tt.providedPath == "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform" {
					assert.NotContains(t, result, "/home/runner/_work/infrastructure/infrastructure/home/runner",
						"Should not duplicate the GitHub Actions path")
				}
			}
		})
	}
}

// TestJoinPath_UnixPathNormalization tests how paths are normalized on Unix.
func TestJoinPath_UnixPathNormalization(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		providedPath string
		description  string
		checkFunc    func(t *testing.T, result string)
	}{
		{
			name:         "Preserve exact separators",
			basePath:     "/home/user/project",
			providedPath: "components/terraform",
			description:  "Should use forward slashes on Unix",
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, "/home/user/project/components/terraform", result)
				assert.Contains(t, result, "/")
				assert.NotContains(t, result, `\`)
			},
		},
		{
			name:         "Case sensitivity",
			basePath:     "/home/User/Project",
			providedPath: "Components/Terraform",
			description:  "Unix is case-sensitive",
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, "/home/User/Project/Components/Terraform", result)
				assert.Contains(t, result, "User")
				assert.Contains(t, result, "Components")
				// These would be different paths on Unix
				assert.NotEqual(t, "/home/user/project/components/terraform", result)
			},
		},
		{
			name:         "Backslash treated as regular character",
			basePath:     "/home/user",
			providedPath: `path\with\backslashes`,
			description:  "Backslashes are valid filename chars on Unix",
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, `/home/user/path\with\backslashes`, result)
				assert.Contains(t, result, `\`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.basePath, tt.providedPath)
			tt.checkFunc(t, result)
		})
	}
}

// TestJoinPath_UnixAbsolutePaths specifically tests Unix absolute path scenarios.
func TestJoinPath_UnixAbsolutePaths(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		providedPath string
		expected     string
	}{
		{
			name:         "GitHub Actions Unix path - components absolute",
			basePath:     "/home/runner/_work/infrastructure/infrastructure",
			providedPath: "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
			expected:     "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
		},
		{
			name:         "GitHub Actions Unix path - components relative",
			basePath:     "/home/runner/_work/infrastructure/infrastructure",
			providedPath: "atmos/components/terraform",
			expected:     "/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform",
		},
		{
			name:         "Root path joining",
			basePath:     "/",
			providedPath: "usr/local/bin",
			expected:     "/usr/local/bin",
		},
		{
			name:         "Home directory path",
			basePath:     "/home/user",
			providedPath: ".config/atmos",
			expected:     "/home/user/.config/atmos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.basePath, tt.providedPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}
