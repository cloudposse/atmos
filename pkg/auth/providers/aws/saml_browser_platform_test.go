package aws

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMacHostPlatform(t *testing.T) {
	tests := []struct {
		release string
		cpu     string
		want    string
	}{
		{release: "17.7.0", cpu: "Intel(R) Core(TM) i7", want: "mac10.13"},
		{release: "18.7.0", want: "mac10.14"},
		{release: "19.6.0", want: "mac10.15"},
		{release: "19.6.0", cpu: "Apple M1", want: "mac10.15"},
		{release: "20.6.0", cpu: "Intel(R) Core(TM) i9", want: "mac11"},
		{release: "20.6.0", cpu: "Apple M1", want: "mac11-arm64"},
		{release: "23.4.0", cpu: "Apple M2 Pro", want: "mac14-arm64"},
		{release: "23.4.0", cpu: "Intel(R) Core(TM) i9", want: "mac14"},
		{release: "24.1.0", cpu: "Apple M4", want: "mac15-arm64"},
		{release: "25.0.0", cpu: "Apple M4", want: "mac26-arm64"},
		// Newer betas are capped at the last stable macOS version Playwright knows.
		{release: "27.0.0", cpu: "Apple M5", want: "mac26-arm64"},
		{release: "", want: ""},
		{release: "garbage", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.release+"/"+tt.cpu, func(t *testing.T) {
			assert.Equal(t, tt.want, macHostPlatform(tt.release, tt.cpu))
		})
	}
}

func TestLinuxHostPlatform(t *testing.T) {
	tests := []struct {
		name      string
		goarch    string
		osRelease string
		want      string
	}{
		{name: "ubuntu 22.04", goarch: "amd64", osRelease: "ID=ubuntu\nVERSION_ID=\"22.04\"\n", want: "ubuntu22.04-x64"},
		{name: "ubuntu 24.04 arm64", goarch: "arm64", osRelease: "ID=ubuntu\nVERSION_ID=\"24.04\"\n", want: "ubuntu24.04-arm64"},
		{name: "ubuntu 18.04", goarch: "amd64", osRelease: "ID=ubuntu\nVERSION_ID=\"18.04\"\n", want: "ubuntu18.04-x64"},
		{name: "ubuntu 21.10", goarch: "amd64", osRelease: "ID=ubuntu\nVERSION_ID=\"21.10\"\n", want: "ubuntu20.04-x64"},
		{name: "ubuntu 26.04", goarch: "amd64", osRelease: "ID=ubuntu\nVERSION_ID=\"26.04\"\n", want: "ubuntu26.04-x64"},
		{name: "ubuntu 28.04", goarch: "amd64", osRelease: "ID=ubuntu\nVERSION_ID=\"28.04\"\n", want: "ubuntu28.04-x64"},
		{name: "pop os", goarch: "amd64", osRelease: "ID=pop\nVERSION_ID=\"22.04\"\n", want: "ubuntu22.04-x64"},
		{name: "linux mint 21", goarch: "amd64", osRelease: "ID=linuxmint\nVERSION_ID=\"21.3\"\n", want: "ubuntu22.04-x64"},
		{name: "linux mint 20", goarch: "amd64", osRelease: "ID=linuxmint\nVERSION_ID=\"20\"\n", want: "ubuntu20.04-x64"},
		{name: "linux mint 22", goarch: "amd64", osRelease: "ID=linuxmint\nVERSION_ID=\"22\"\n", want: "ubuntu24.04-x64"},
		{name: "debian 12", goarch: "arm64", osRelease: "ID=debian\nVERSION_ID=\"12\"\n", want: "debian12-arm64"},
		{name: "debian testing", goarch: "amd64", osRelease: "ID=debian\n", want: "debian13-x64"},
		{name: "debian 10 falls back", goarch: "amd64", osRelease: "ID=debian\nVERSION_ID=\"10\"\n", want: "ubuntu24.04-x64"},
		{name: "unknown distribution", goarch: "amd64", osRelease: "ID=fedora\nVERSION_ID=40\n", want: "ubuntu24.04-x64"},
		{name: "no os-release", goarch: "amd64", want: "ubuntu24.04-x64"},
		{name: "unsupported architecture", goarch: "386", osRelease: "ID=ubuntu\nVERSION_ID=\"22.04\"\n", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, linuxHostPlatform(tt.goarch, tt.osRelease))
		})
	}
}

func TestParseOSRelease(t *testing.T) {
	fields := parseOSRelease("NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"22.04\"\n\n=ignored\nPRETTY_NAME=\"a=b\"\n")
	assert.Equal(t, map[string]string{
		"name":        "Ubuntu",
		"id":          "ubuntu",
		"version_id":  "22.04",
		"pretty_name": "a=b",
	}, fields)
}

func TestPlaywrightHostPlatform(t *testing.T) {
	t.Run("override wins", func(t *testing.T) {
		t.Setenv(playwrightHostPlatformOverrideEnv, "ubuntu20.04-arm64")
		assert.Equal(t, "ubuntu20.04-arm64", playwrightHostPlatform())
	})

	t.Run("detected for the current OS", func(t *testing.T) {
		t.Setenv(playwrightHostPlatformOverrideEnv, "")
		original := darwinHostInfo
		t.Cleanup(func() { darwinHostInfo = original })
		darwinHostInfo = func() (string, string, error) { return "23.4.0", "Apple M2", nil }

		got := playwrightHostPlatform()
		switch runtime.GOOS {
		case "darwin":
			assert.Equal(t, "mac14-arm64", got)
		case windowsOS:
			assert.Equal(t, "win64", got)
		case "linux":
			if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
				assert.NotEmpty(t, got)
			}
		default:
			assert.Empty(t, got)
		}
	})

	t.Run("darwin host information unavailable", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skipf("Darwin host detection only runs on macOS, not %s", runtime.GOOS)
		}
		t.Setenv(playwrightHostPlatformOverrideEnv, "")
		original := darwinHostInfo
		t.Cleanup(func() { darwinHostInfo = original })
		darwinHostInfo = func() (string, string, error) { return "", "", assert.AnError }

		assert.Empty(t, playwrightHostPlatform())
	})
}

// The real per-OS implementation either reports the Darwin kernel or fails off macOS.
func TestDarwinKernelInfo(t *testing.T) {
	release, _, err := darwinKernelInfo()
	if runtime.GOOS != "darwin" {
		assert.Error(t, err)
		return
	}
	assert.NoError(t, err)
	_, ok := leadingInt(release)
	assert.True(t, ok, "kernel release %q starts with a number", release)
}
