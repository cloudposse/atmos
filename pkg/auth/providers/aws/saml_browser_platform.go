package aws

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

const (
	// playwrightHostPlatformOverrideEnv overrides the host platform the Playwright driver detects.
	playwrightHostPlatformOverrideEnv = "PLAYWRIGHT_HOST_PLATFORM_OVERRIDE"
	// linuxOSReleasePath is the file the Playwright driver reads the Linux distribution from.
	linuxOSReleasePath = "/etc/os-release"
	// playwrightLastStableMacOSMajor is LAST_STABLE_MACOS_MAJOR_VERSION in playwright-core 1.64.
	playwrightLastStableMacOSMajor = 26
	// playwrightUbuntuDefault is the platform Playwright falls back to on an unrecognized Linux distribution.
	playwrightUbuntuDefault = "ubuntu24.04"
)

// darwinHostInfo returns the Darwin kernel release (os.release() in Node.js) and the CPU brand.
// It is a variable so tests can simulate a host; the real implementation is per-OS.
var darwinHostInfo = darwinKernelInfo

// playwrightHostPlatform returns the host platform key the bundled Playwright driver uses to pick a
// revisionOverrides entry from browsers.json (for example mac14-arm64, ubuntu24.04-x64 or win64).
// It mirrors calculatePlatform in playwright-core 1.64 (packages/utils/hostPlatform.ts).
// An empty result means the platform is unknown, so no override applies. A wrong guess is safe:
// the browser is then reported missing and `playwright install`, which skips an installed browser,
// runs once more.
func playwrightHostPlatform() string {
	if override := os.Getenv(playwrightHostPlatformOverrideEnv); override != "" { //nolint:forbidigo // Third-party (Playwright) env var, read the same way the driver reads it.
		return override
	}

	switch runtime.GOOS {
	case "darwin":
		release, cpuBrand, err := darwinHostInfo()
		if err != nil {
			return ""
		}
		return macHostPlatform(release, cpuBrand)
	case "linux":
		osRelease, err := os.ReadFile(linuxOSReleasePath)
		if err != nil {
			osRelease = nil
		}
		return linuxHostPlatform(runtime.GOARCH, string(osRelease))
	case windowsOS:
		return "win64"
	default:
		return ""
	}
}

// macHostPlatform returns the Playwright macOS platform for a Darwin kernel release (for example "23.4.0")
// and CPU brand string. Apple Silicon hosts on macOS 11 and later get an "-arm64" suffix.
func macHostPlatform(kernelRelease, cpuBrand string) string {
	major, ok := leadingInt(kernelRelease)
	if !ok {
		return ""
	}

	var version string
	switch {
	case major < 18:
		version = "10.13"
	case major == 18:
		version = "10.14"
	case major == 19:
		version = "10.15"
	case major < 25:
		version = strconv.Itoa(11 + major - 20)
	default:
		version = strconv.Itoa(min(major+1, playwrightLastStableMacOSMajor))
	}

	platform := "mac" + version
	if major >= 20 && strings.Contains(cpuBrand, "Apple") {
		platform += "-arm64"
	}
	return platform
}

// linuxHostPlatform returns the Playwright Linux platform for a Go architecture and the contents of /etc/os-release.
func linuxHostPlatform(goarch, osRelease string) string {
	var arch string
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return ""
	}
	suffix := "-" + arch

	fields := parseOSRelease(osRelease)
	id, version := fields["id"], fields["version_id"]
	major, majorOK := leadingInt(version)

	switch id {
	case "ubuntu", "pop", "neon", "tuxedo":
		return ubuntuHostPlatform(version, major, majorOK) + suffix
	case "linuxmint":
		switch {
		case majorOK && major <= 20:
			return "ubuntu20.04" + suffix
		case majorOK && major == 21:
			return "ubuntu22.04" + suffix
		default:
			return playwrightUbuntuDefault + suffix
		}
	case "debian", "raspbian":
		switch version {
		case "11", "12", "13":
			return "debian" + version + suffix
		case "":
			// Debian testing and unstable have no VERSION_ID; Playwright uses the latest release.
			return "debian13" + suffix
		}
	}
	return playwrightUbuntuDefault + suffix
}

// ubuntuHostPlatform maps an Ubuntu-based VERSION_ID to the Ubuntu LTS platform Playwright uses.
// Like JavaScript's parseInt, a version without a leading number matches no range.
func ubuntuHostPlatform(version string, major int, majorOK bool) string {
	switch {
	case majorOK && major < 20:
		return "ubuntu18.04"
	case majorOK && major < 22:
		return "ubuntu20.04"
	case majorOK && major < 24:
		return "ubuntu22.04"
	case majorOK && major < 26:
		return "ubuntu24.04"
	case majorOK && major < 28:
		return "ubuntu26.04"
	default:
		return "ubuntu" + version
	}
}

// parseOSRelease parses /etc/os-release the way playwright-core does: lower-cased keys, values
// trimmed and stripped of surrounding double quotes.
func parseOSRelease(text string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		name, value, _ := strings.Cut(line, "=")
		if name == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			value = value[1 : len(value)-1]
		}
		fields[strings.ToLower(name)] = value
	}
	return fields
}

// leadingInt parses the leading decimal digits of s, like JavaScript's parseInt(s, 10).
func leadingInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}
