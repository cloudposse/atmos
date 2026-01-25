package installer

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/toolchain/registry"
)

func TestIsPlatformMatch(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		currentOS   string
		currentArch string
		want        bool
	}{
		{"OS only matches same OS", "darwin", "darwin", "arm64", true},
		{"OS only matches different arch", "darwin", "darwin", "amd64", true},
		{"OS only does not match different OS", "darwin", "linux", "amd64", false},
		{"linux matches linux", "linux", "linux", "amd64", true},
		{"windows matches windows", "windows", "windows", "amd64", true},
		{"OS/arch exact match", "darwin/arm64", "darwin", "arm64", true},
		{"OS/arch different arch", "darwin/amd64", "darwin", "arm64", false},
		{"OS/arch different OS", "linux/amd64", "darwin", "amd64", false},
		{"linux/arm64 matches", "linux/arm64", "linux", "arm64", true},
		{"handles whitespace", "  darwin  ", "darwin", "arm64", true},
		{"handles uppercase", "DARWIN", "darwin", "arm64", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPlatformMatch(tt.env, tt.currentOS, tt.currentArch)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsEnv(t *testing.T) {
	tests := []struct {
		name          string
		supportedEnvs []string
		target        string
		want          bool
	}{
		{"exact match", []string{"darwin", "linux"}, "darwin", true},
		{"no match", []string{"darwin", "linux"}, "windows", false},
		{"OS/arch in list", []string{"darwin/amd64", "linux/amd64"}, "darwin/amd64", true},
		{"OS matches OS/arch prefix", []string{"darwin/amd64", "linux/amd64"}, "darwin", true},
		{"empty list", []string{}, "darwin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsEnv(tt.supportedEnvs, tt.target)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckPlatformSupport(t *testing.T) {
	currentOS := runtime.GOOS
	currentArch := runtime.GOARCH

	tests := []struct {
		name          string
		supportedEnvs []string
		wantErr       bool
	}{
		{"empty supported_envs allows all", []string{}, false},
		{"nil supported_envs allows all", nil, false},
		{"current OS supported", []string{currentOS}, false},
		{"current OS/arch supported", []string{currentOS + "/" + currentArch}, false},
		{"different OS not supported", []string{"unsupportedos"}, true},
		{"multiple envs with current supported", []string{"unsupportedos", currentOS}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &registry.Tool{
				RepoOwner:     "test",
				RepoName:      "tool",
				SupportedEnvs: tt.supportedEnvs,
			}
			err := CheckPlatformSupport(tool)
			if tt.wantErr {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), "does not support")
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestBuildPlatformHints_Windows(t *testing.T) {
	hints := buildPlatformHints("windows", "amd64", []string{"darwin", "linux"})

	assert.Contains(t, hints[0], "darwin, linux")
	foundWSL := false
	for _, hint := range hints {
		if assert.NotEmpty(t, hint) && len(hint) > 0 {
			if contains(hint, "WSL") {
				foundWSL = true
			}
		}
	}
	assert.True(t, foundWSL, "Should suggest WSL for Windows users when Linux is supported")
}

func TestBuildPlatformHints_DarwinArm64(t *testing.T) {
	hints := buildPlatformHints("darwin", "arm64", []string{"darwin/amd64", "linux"})

	foundRosetta := false
	for _, hint := range hints {
		if contains(hint, "Rosetta") {
			foundRosetta = true
		}
	}
	assert.True(t, foundRosetta, "Should suggest Rosetta for darwin/arm64 when darwin/amd64 is supported")
}

func TestBuildPlatformHints_LinuxOnlyOnDarwin(t *testing.T) {
	hints := buildPlatformHints("darwin", "arm64", []string{"linux"})

	foundDocker := false
	for _, hint := range hints {
		if contains(hint, "Docker") {
			foundDocker = true
		}
	}
	assert.True(t, foundDocker, "Should suggest Docker for macOS when only Linux is supported")
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
