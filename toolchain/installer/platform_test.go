package installer

import (
	"runtime"
	"strings"
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
		// OS-only matching.
		{"OS only matches same OS", "darwin", "darwin", "arm64", true},
		{"OS only matches different arch", "darwin", "darwin", "amd64", true},
		{"OS only does not match different OS", "darwin", "linux", "amd64", false},
		{"linux matches linux", "linux", "linux", "amd64", true},
		{"windows matches windows", "windows", "windows", "amd64", true},
		// OS/arch exact matching.
		{"OS/arch exact match", "darwin/arm64", "darwin", "arm64", true},
		{"OS/arch different arch", "darwin/amd64", "darwin", "arm64", false},
		{"OS/arch different OS", "linux/amd64", "darwin", "amd64", false},
		{"linux/arm64 matches", "linux/arm64", "linux", "arm64", true},
		// Arch-only matching (Aqua registry format).
		{"arch only amd64 matches any OS with amd64", "amd64", "windows", "amd64", true},
		{"arch only amd64 matches darwin amd64", "amd64", "darwin", "amd64", true},
		{"arch only amd64 matches linux amd64", "amd64", "linux", "amd64", true},
		{"arch only amd64 does not match arm64", "amd64", "darwin", "arm64", false},
		{"arch only arm64 matches any OS with arm64", "arm64", "darwin", "arm64", true},
		{"arch only arm64 does not match amd64", "arm64", "linux", "amd64", false},
		// Edge cases.
		{"handles whitespace", "  darwin  ", "darwin", "arm64", true},
		{"handles uppercase", "DARWIN", "darwin", "arm64", true},
		{"handles uppercase arch", "AMD64", "windows", "amd64", true},
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
		// Arch-only support (like helm's registry entry).
		{"current arch supported (arch-only)", []string{currentArch}, false},
		{"helm-like mixed OS and arch", []string{"darwin", "linux", currentArch}, false},
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
		if strings.Contains(hint, "WSL") {
			foundWSL = true
		}
	}
	assert.True(t, foundWSL, "Should suggest WSL for Windows users when Linux is supported")
}

func TestBuildPlatformHints_DarwinArm64(t *testing.T) {
	hints := buildPlatformHints("darwin", "arm64", []string{"darwin/amd64", "linux"})

	foundRosetta := false
	for _, hint := range hints {
		if strings.Contains(hint, "Rosetta") {
			foundRosetta = true
		}
	}
	assert.True(t, foundRosetta, "Should suggest Rosetta for darwin/arm64 when darwin/amd64 is supported")
}

func TestBuildPlatformHints_LinuxOnlyOnDarwin(t *testing.T) {
	hints := buildPlatformHints("darwin", "arm64", []string{"linux"})

	foundDocker := false
	for _, hint := range hints {
		if strings.Contains(hint, "Docker") {
			foundDocker = true
		}
	}
	assert.True(t, foundDocker, "Should suggest Docker for macOS when only Linux is supported")
}

func TestBuildPlatformHints_LinuxArm64(t *testing.T) {
	hints := buildPlatformHints("linux", "arm64", []string{"linux/amd64"})

	foundQEMU := false
	for _, hint := range hints {
		if strings.Contains(hint, "qemu") || strings.Contains(hint, "amd64") {
			foundQEMU = true
		}
	}
	assert.True(t, foundQEMU, "Should suggest QEMU or mention amd64 for linux/arm64 when only linux/amd64 is supported")
}

func TestBuildPlatformHints_NoPlatformSpecificHints(t *testing.T) {
	// Test when no platform-specific hints apply.
	hints := buildPlatformHints("freebsd", "amd64", []string{"darwin", "linux"})

	// Should still have the base hint about supported platforms.
	assert.NotEmpty(t, hints)
	assert.Contains(t, hints[0], "darwin, linux")

	// Should not have WSL hint (not Windows).
	for _, hint := range hints {
		assert.NotContains(t, hint, "WSL")
	}
}

func TestFormatPlatformError(t *testing.T) {
	tests := []struct {
		name        string
		platformErr *PlatformError
		expectTool  string
		expectEnv   string
		expectHint  string
	}{
		{
			name: "basic format",
			platformErr: &PlatformError{
				Tool:          "owner/repo",
				CurrentEnv:    "darwin/arm64",
				SupportedEnvs: []string{"linux/amd64"},
				Hints:         []string{"This tool only supports: linux/amd64"},
			},
			expectTool: "owner/repo",
			expectEnv:  "darwin/arm64",
			expectHint: "linux/amd64",
		},
		{
			name: "multiple hints",
			platformErr: &PlatformError{
				Tool:          "test/tool",
				CurrentEnv:    "windows/amd64",
				SupportedEnvs: []string{"darwin", "linux"},
				Hints: []string{
					"This tool only supports: darwin, linux",
					"Consider using WSL",
				},
			},
			expectTool: "test/tool",
			expectEnv:  "windows/amd64",
			expectHint: "WSL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := FormatPlatformError(tt.platformErr)

			assert.Contains(t, formatted, tt.expectTool)
			assert.Contains(t, formatted, tt.expectEnv)
			assert.Contains(t, formatted, tt.expectHint)
			// Should contain emoji hint markers.
			assert.Contains(t, formatted, "💡")
		})
	}
}

func TestIsKnownArch(t *testing.T) {
	tests := []struct {
		arch string
		want bool
	}{
		{"amd64", true},
		{"arm64", true},
		{"386", true},
		{"arm", true},
		{"ppc64", true},
		{"ppc64le", true},
		{"mips", true},
		{"mipsle", true},
		{"mips64", true},
		{"s390x", true},
		{"riscv64", true},
		// Not known architectures.
		{"x86_64", false},  // Alias, not Go's name.
		{"aarch64", false}, // Alias for arm64.
		{"darwin", false},  // OS, not arch.
		{"linux", false},   // OS, not arch.
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			got := isKnownArch(tt.arch)
			assert.Equal(t, tt.want, got, "isKnownArch(%q)", tt.arch)
		})
	}
}

func TestPlatformError_Error(t *testing.T) {
	err := &PlatformError{
		Tool:          "owner/repo",
		CurrentEnv:    "windows/arm64",
		SupportedEnvs: []string{"darwin/amd64", "linux/amd64"},
		Hints:         []string{"hint1", "hint2"},
	}

	errMsg := err.Error()
	assert.Contains(t, errMsg, "owner/repo")
	assert.Contains(t, errMsg, "windows/arm64")
	assert.Contains(t, errMsg, "does not support")
}

func TestAppendWindowsHints_NotWindows(t *testing.T) {
	// When not on Windows, should return hints unchanged.
	initialHints := []string{"initial hint"}
	result := appendWindowsHints(initialHints, "darwin", []string{"linux"})
	assert.Equal(t, initialHints, result)
}

func TestAppendWindowsHints_WindowsNoLinuxSupport(t *testing.T) {
	// When on Windows but Linux is not in supported envs, should not add WSL hint.
	initialHints := []string{"initial hint"}
	result := appendWindowsHints(initialHints, "windows", []string{"darwin"})
	assert.Equal(t, initialHints, result)
}

func TestAppendDarwinArm64Hints_NotDarwinArm64(t *testing.T) {
	// When not on darwin/arm64, should return hints unchanged.
	initialHints := []string{"initial hint"}
	// Test linux.
	result := appendDarwinArm64Hints(initialHints, "linux", "arm64", []string{"darwin/amd64"})
	assert.Equal(t, initialHints, result)
	// Test darwin/amd64.
	result = appendDarwinArm64Hints(initialHints, "darwin", "amd64", []string{"linux"})
	assert.Equal(t, initialHints, result)
}

func TestAppendLinuxArm64Hints_NotLinuxArm64(t *testing.T) {
	// When not on linux/arm64, should return hints unchanged.
	initialHints := []string{"initial hint"}
	result := appendLinuxArm64Hints(initialHints, "darwin", "arm64", []string{"linux/amd64"})
	assert.Equal(t, initialHints, result)
}

func TestAppendLinuxArm64Hints_NoAmd64Support(t *testing.T) {
	// When on linux/arm64 but linux/amd64 is not supported, should not add QEMU hint.
	initialHints := []string{"initial hint"}
	result := appendLinuxArm64Hints(initialHints, "linux", "arm64", []string{"darwin/amd64"})
	assert.Equal(t, initialHints, result)
}
