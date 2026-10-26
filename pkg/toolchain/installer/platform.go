package installer

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

// PlatformError represents a platform compatibility error with helpful hints.
type PlatformError struct {
	Tool          string
	CurrentEnv    string
	SupportedEnvs []string
	Hints         []string
}

// Error implements the error interface.
//
//nolint:lintroller // Trivial getter implementing error interface - no perf tracking needed.
func (e *PlatformError) Error() string {
	return fmt.Sprintf("tool %s does not support %s", e.Tool, e.CurrentEnv)
}

// CheckPlatformSupport checks if the current platform is supported by the tool.
// Returns nil if supported, or a PlatformError with helpful hints if not.
// Supports Rosetta 2 fallback (darwin/arm64 → darwin/amd64) and Windows ARM emulation.
func CheckPlatformSupport(tool *registry.Tool) *PlatformError {
	defer perf.Track(nil, "installer.CheckPlatformSupport")()

	return checkPlatformSupportForEnv(tool, runtime.GOOS, runtime.GOARCH)
}

// checkPlatformSupportForEnv is the testable inner implementation of CheckPlatformSupport.
// It accepts explicit OS/arch parameters so all branches can be exercised in tests.
func checkPlatformSupportForEnv(tool *registry.Tool, currentOS, currentArch string) *PlatformError {
	// If no supported_envs specified, assume all platforms are supported.
	if len(tool.SupportedEnvs) == 0 {
		return nil
	}

	// Check if current platform is directly supported.
	if hasPlatformMatch(tool.SupportedEnvs, currentOS, currentArch) {
		return nil
	}

	// Check emulation fallbacks (Rosetta 2, Windows ARM).
	if hasEmulationFallback(tool, currentOS, currentArch) {
		return nil
	}

	// Platform not supported - build error with hints.
	return &PlatformError{
		Tool:          fmt.Sprintf("%s/%s", tool.RepoOwner, tool.RepoName),
		CurrentEnv:    fmt.Sprintf("%s/%s", currentOS, currentArch),
		SupportedEnvs: tool.SupportedEnvs,
		Hints:         buildPlatformHints(currentOS, currentArch, tool.SupportedEnvs),
	}
}

// hasPlatformMatch checks if any supported env matches the given OS/arch.
func hasPlatformMatch(supportedEnvs []string, targetOS, targetArch string) bool {
	for _, env := range supportedEnvs {
		if isPlatformMatch(env, targetOS, targetArch) {
			return true
		}
	}
	return false
}

// hasEmulationFallback checks if the tool supports the platform via emulation.
// Rosetta 2: darwin/arm64 can run darwin/amd64 binaries.
// Windows ARM: windows/arm64 can run windows/amd64 binaries.
func hasEmulationFallback(tool *registry.Tool, currentOS, currentArch string) bool {
	if tool.Rosetta2 && currentOS == "darwin" && currentArch == "arm64" {
		return hasPlatformMatch(tool.SupportedEnvs, "darwin", "amd64")
	}
	if tool.WindowsArmEmulation && currentOS == "windows" && currentArch == "arm64" {
		return hasPlatformMatch(tool.SupportedEnvs, "windows", "amd64")
	}
	return false
}

// isPlatformMatch checks if a supported_env entry matches the current platform.
// Supported formats (following Aqua registry conventions):
//   - "darwin" - matches any darwin architecture
//   - "linux" - matches any linux architecture
//   - "windows" - matches any windows architecture
//   - "amd64" - matches any OS with amd64 architecture
//   - "arm64" - matches any OS with arm64 architecture
//   - "darwin/amd64" - matches specific OS/arch
//   - "linux/arm64" - matches specific OS/arch
func isPlatformMatch(env, currentOS, currentArch string) bool {
	env = strings.ToLower(strings.TrimSpace(env))

	// "all" matches any platform (upstream aquaproj/aqua convention).
	if env == "all" {
		return true
	}

	// Check for exact OS/arch match.
	if strings.Contains(env, "/") {
		parts := strings.Split(env, "/")
		if len(parts) == 2 {
			return parts[0] == currentOS && parts[1] == currentArch
		}
	}

	// Check for arch-only match (any OS with this architecture).
	// Aqua registry uses entries like "amd64" to mean "any OS with amd64".
	if isKnownArch(env) {
		return env == currentArch
	}

	// Check for OS-only match (any architecture).
	return env == currentOS
}

// isKnownArch returns true if the string is a known Go architecture name.
func isKnownArch(s string) bool {
	knownArchs := map[string]bool{
		"amd64":   true,
		"arm64":   true,
		"386":     true,
		"arm":     true,
		"ppc64":   true,
		"ppc64le": true,
		"mips":    true,
		"mipsle":  true,
		"mips64":  true,
		"s390x":   true,
		"riscv64": true,
	}
	return knownArchs[s]
}

// buildPlatformHints generates helpful hints based on the current platform.
func buildPlatformHints(currentOS, currentArch string, supportedEnvs []string) []string {
	hints := []string{
		fmt.Sprintf("This tool only supports: %s", strings.Join(supportedEnvs, ", ")),
	}

	// Add platform-specific suggestions.
	hints = appendWindowsHints(hints, currentOS, supportedEnvs)
	hints = appendDarwinArm64Hints(hints, currentOS, currentArch, supportedEnvs)
	hints = appendLinuxArm64Hints(hints, currentOS, currentArch, supportedEnvs)

	return hints
}

// appendWindowsHints adds Windows-specific hints (WSL suggestion).
func appendWindowsHints(hints []string, currentOS string, supportedEnvs []string) []string {
	if currentOS != "windows" {
		return hints
	}
	if containsEnv(supportedEnvs, "linux") {
		hints = append(hints,
			"Consider using WSL (Windows Subsystem for Linux) to run this tool",
			"Install WSL: https://docs.microsoft.com/en-us/windows/wsl/install",
		)
	}
	return hints
}

// appendDarwinArm64Hints adds macOS arm64-specific hints (Rosetta/Docker suggestions).
func appendDarwinArm64Hints(hints []string, currentOS, currentArch string, supportedEnvs []string) []string {
	if currentOS != "darwin" || currentArch != "arm64" {
		return hints
	}

	// Check if darwin/amd64 is supported but not darwin/arm64.
	darwinSupported := containsEnv(supportedEnvs, "darwin/amd64") || containsEnv(supportedEnvs, "darwin")
	arm64Supported := containsEnv(supportedEnvs, "darwin/arm64")
	if darwinSupported && !arm64Supported {
		hints = append(hints,
			"Try installing the amd64 version and running under Rosetta 2",
			"Run: softwareupdate --install-rosetta",
		)
	}

	// Check if only Linux is supported.
	if !containsEnv(supportedEnvs, "darwin") && containsEnv(supportedEnvs, "linux") {
		hints = append(hints,
			"Consider using Docker to run this Linux-only tool on macOS",
		)
	}

	return hints
}

// appendLinuxArm64Hints adds Linux arm64-specific hints (QEMU suggestion).
func appendLinuxArm64Hints(hints []string, currentOS, currentArch string, supportedEnvs []string) []string {
	if currentOS != "linux" || currentArch != "arm64" {
		return hints
	}
	if containsEnv(supportedEnvs, "linux/amd64") {
		hints = append(hints,
			"This tool may only support amd64 architecture",
			"Consider using an emulation layer like qemu-user",
		)
	}
	return hints
}

// containsEnv checks if the supported envs list contains a specific environment.
func containsEnv(supportedEnvs []string, target string) bool {
	target = strings.ToLower(target)
	for _, env := range supportedEnvs {
		env = strings.ToLower(strings.TrimSpace(env))
		if env == target {
			return true
		}
		// Also match if target is OS-only and env starts with that OS.
		if !strings.Contains(target, "/") && strings.HasPrefix(env, target) {
			return true
		}
	}
	return false
}

// FormatPlatformError formats a PlatformError into a user-friendly string.
func FormatPlatformError(err *PlatformError) string {
	defer perf.Track(nil, "installer.FormatPlatformError")()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool `%s` does not support your platform (%s)\n\n", err.Tool, err.CurrentEnv))
	for _, hint := range err.Hints {
		sb.WriteString(fmt.Sprintf("💡 %s\n", hint))
	}
	return sb.String()
}
