package aws

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const (
	// Environment variable that overrides the host platform the Playwright driver detects.
	playwrightHostPlatformOverrideEnv = "PLAYWRIGHT_HOST_PLATFORM_OVERRIDE"
	// Path of the file the Playwright driver reads the Linux distribution from.
	linuxOSReleasePath = "/etc/os-release"
	// LAST_STABLE_MACOS_MAJOR_VERSION in playwright-core 1.64.
	playwrightLastStableMacOSMajor = 26
	// Platform Playwright falls back to on an unrecognized Linux distribution.
	playwrightUbuntuDefault = "ubuntu24.04"

	// Darwin kernel major versions at which the macOS version mapping changes.
	darwinMajorMojave   = 18 // macOS 10.14.
	darwinMajorCatalina = 19 // macOS 10.15.
	darwinMajorBigSur   = 20 // macOS 11, the first release with Apple Silicon.
	darwinMajorTahoe    = 25 // macOS 26, where macOS versions follow Darwin major + 1.
	// First macOS major version number that follows the 10.x series.
	macOSBigSurMajor = 11

	// Ubuntu LTS releases Playwright publishes builds for, and the number of Ubuntu major versions each covers.
	ubuntuFirstLTS = 18
	ubuntuLastLTS  = 26
	ubuntuLTSSpan  = 2

	// Linux Mint major versions that map to an Ubuntu LTS.
	linuxMintMajorFocal = 20 // Based on Ubuntu 20.04.
	linuxMintMajorJammy = 21 // Based on Ubuntu 22.04.

	// Dash separating name parts, for example a platform name and its architecture suffix.
	dashSeparator = "-"
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

	platform := "mac" + macVersionForDarwinMajor(major)
	if major >= darwinMajorBigSur && strings.Contains(cpuBrand, "Apple") {
		platform += dashSeparator + "arm64"
	}
	return platform
}

// macVersionForDarwinMajor maps a Darwin kernel major version to the macOS version Playwright uses.
func macVersionForDarwinMajor(major int) string {
	switch {
	case major < darwinMajorMojave:
		return "10.13"
	case major == darwinMajorMojave:
		return "10.14"
	case major == darwinMajorCatalina:
		return "10.15"
	case major < darwinMajorTahoe:
		return strconv.Itoa(macOSBigSurMajor + major - darwinMajorBigSur)
	default:
		return strconv.Itoa(min(major+1, playwrightLastStableMacOSMajor))
	}
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
	suffix := dashSeparator + arch

	fields := parseOSRelease(osRelease)
	id, version := fields["id"], fields["version_id"]
	major, majorOK := leadingInt(version)

	switch id {
	case "ubuntu", "pop", "neon", "tuxedo":
		return ubuntuHostPlatform(version, major, majorOK) + suffix
	case "linuxmint":
		return mintHostPlatform(major, majorOK) + suffix
	case "debian", "raspbian":
		return debianHostPlatform(version) + suffix
	}
	return playwrightUbuntuDefault + suffix
}

// mintHostPlatform maps a Linux Mint major version to the Ubuntu platform Playwright uses.
func mintHostPlatform(major int, majorOK bool) string {
	switch {
	case majorOK && major <= linuxMintMajorFocal:
		return "ubuntu20.04"
	case majorOK && major == linuxMintMajorJammy:
		return "ubuntu22.04"
	default:
		return playwrightUbuntuDefault
	}
}

// debianHostPlatform maps a Debian VERSION_ID to the Playwright platform, falling back to the Ubuntu default.
func debianHostPlatform(version string) string {
	switch version {
	case "11", "12", "13":
		return "debian" + version
	case "":
		// Debian testing and unstable have no VERSION_ID; Playwright uses the latest release.
		return "debian13"
	default:
		return playwrightUbuntuDefault
	}
}

// ubuntuHostPlatform maps an Ubuntu-based VERSION_ID to the Ubuntu LTS platform Playwright uses.
// Like JavaScript's parseInt, a version without a leading number matches no range.
func ubuntuHostPlatform(version string, major int, majorOK bool) string {
	if majorOK {
		for lts := ubuntuFirstLTS; lts <= ubuntuLastLTS; lts += ubuntuLTSSpan {
			if major < lts+ubuntuLTSSpan {
				return fmt.Sprintf("ubuntu%d.04", lts)
			}
		}
	}
	return "ubuntu" + version
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
