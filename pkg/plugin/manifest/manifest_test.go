package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/version"
)

const validManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: example/oidc, version: 1.2.3}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2", contracts: {auth.provider: ">=1 <2"}}
  capabilities: {commands: {about: {description: "Show version"}}}
`

func TestParseValidatesPackageAndRuntimeContracts(t *testing.T) {
	t.Parallel()
	parsed, err := Parse([]byte(validManifest))
	require.NoError(t, err)
	require.Equal(t, "example/oidc", parsed.Metadata.Name)
	require.Equal(t, "bin/plugin", parsed.Spec.Runtime.Executable)
	tests := []struct {
		name, from, to string
		expected       error
	}{
		{"name", "example/oidc", "Invalid name", errUtils.ErrPluginInvalidConfig},
		{"version", "1.2.3", "not-semver", errUtils.ErrPluginInvalidConfig},
		{"runtime", "type: native", "type: starlark", errUtils.ErrPluginInvalidConfig},
		{"protocol", "protocol: 1,", "protocol: 2,", errUtils.ErrPluginInvalidConfig},
		{"executable", "bin/plugin", "../escape", errUtils.ErrPluginInvalidConfig},
		{"unsupported contract", `auth.provider: ">=1 <2"`, `auth.provider: ">=2"`, errUtils.ErrPluginUnsupportedExtension},
		{"unsupported protocol", `protocol: ">=1 <2"`, `protocol: ">=2"`, errUtils.ErrPluginUnsupportedExtension},
		{"empty protocol", `protocol: ">=1 <2"`, `protocol: ""`, errUtils.ErrPluginUnsupportedExtension},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value, err := Parse([]byte(strings.Replace(validManifest, test.from, test.to, 1)))
			require.ErrorIs(t, err, test.expected)
			require.Nil(t, value)
		})
	}
	_, err = Parse([]byte("{invalid yaml"))
	require.Error(t, err)
}

func TestAtmosCompatibilityRange(t *testing.T) {
	original := version.Version
	t.Cleanup(func() { version.Version = original })
	for _, test := range []struct {
		name, current, rangeValue string
		expected                  error
	}{
		{"release in range", "v1.233.0", ">=1.200.0 <2", nil},
		{"release below range", "v1.233.0", ">=2", errUtils.ErrPluginUnsupportedExtension},
		{"invalid range", "v1.233.0", "not semver", errUtils.ErrPluginInvalidConfig},
		{"prerelease requires opt in", "v1.233.0-rc.1", ">=1.200.0", errUtils.ErrPluginUnsupportedExtension},
		{"prerelease below final release", "v1.233.0-rc.1", ">=1.233.0", errUtils.ErrPluginUnsupportedExtension},
		{"prerelease range opt in", "v1.233.0-rc.1", ">=1.200.0-0 <2.0.0-0", nil},
		{"exact prerelease", "v1.233.0-rc.1", "=1.233.0-rc.1", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			version.Version = test.current
			err := checkCompatibility(Compatibility{Protocol: ">=1 <2", Atmos: test.rangeValue})
			if test.expected == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.expected)
			}
		})
	}
	version.Version = "development"
	require.ErrorIs(t, checkCompatibility(Compatibility{Protocol: "1", Atmos: ">=1"}), errUtils.ErrPluginUnsupportedExtension)
}

func TestManifestReadsAreConfinedAndBounded(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(validManifest), 0o600))
	value, err := Load(directory)
	require.NoError(t, err)
	require.Equal(t, "1.2.3", value.Metadata.Version)
	for _, path := range []string{"", "../outside", filepath.Join(directory, "plugin.yaml"), "missing"} {
		_, err := ReadFile(directory, path)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	_, err = Load(filepath.Join(directory, "missing"))
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NoError(t, os.Mkdir(filepath.Join(directory, "folder"), 0o700))
	_, err = ReadFile(directory, "folder")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "large.json"), []byte(strings.Repeat("x", maxManifestBytes+1)), 0o600))
	_, err = ReadFile(directory, "large.json")
	require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
}

func TestExecutablePrefersTheRunningPlatformEntry(t *testing.T) {
	t.Parallel()
	definition := &Config{}
	definition.Spec.Runtime.Executable = "bin/plugin"
	definition.Spec.Platforms = map[string]Platform{
		"linux/amd64": {Executable: "bin/plugin-linux"},
		"plan9/arm":   {Executable: "../escape"},
	}
	require.Equal(t, "bin/plugin-linux", Executable(definition, "linux/amd64"))
	require.Equal(t, "bin/plugin", Executable(definition, "darwin/arm64"), "an undeclared platform falls back to the runtime executable")
	require.Equal(t, "bin/plugin", Executable(definition, "plan9/arm"), "an escaping platform entry is ignored")
}

func TestExecutableSelectsCompatibleVariantsWithoutCrossArchitectureFallback(t *testing.T) {
	t.Parallel()
	definition := &Config{}
	definition.Spec.Runtime.Executable = "bin/default"
	definition.Spec.Platforms = map[string]Platform{
		"linux/amd64":    {Executable: "bin/amd64"},
		"linux/amd64/v3": {Executable: "bin/amd64-v3"},
		"linux/arm64":    {Executable: "bin/arm64"},
		"linux/arm/v6":   {Executable: "bin/arm-v6"},
		"linux/arm/v7":   {Executable: "bin/arm-v7"},
	}
	for _, test := range []struct{ platform, executable string }{
		{"linux/amd64", "bin/amd64"},
		{"linux/amd64/v2", "bin/amd64"},
		{"linux/amd64/v3", "bin/amd64-v3"},
		{"linux/amd64/v4", "bin/amd64-v3"},
		{"linux/arm64/v8", "bin/arm64"},
		{"linux/arm/v6", "bin/arm-v6"},
		{"linux/arm/v7", "bin/arm-v7"},
		{"linux/arm/v5", "bin/default"},
		{"linux/386", "bin/default"},
		{"windows/amd64", "bin/default"},
	} {
		t.Run(test.platform, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.executable, Executable(definition, test.platform))
		})
	}
}

func TestParseValidatesEveryPlatformBeforeExecutableSelection(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		platform string
		valid    bool
	}{
		{"linux/arm64", true},
		{"linux/amd64/v3", true},
		{"linux/arm/v7", true},
		{"linux/arm64/v9.1", true},
		{"linux", false},
		{"linux/AMD64", false},
		{"linux/amd64/", false},
		{"linux/amd64/v5", false},
		{"linux/amd64/v3/extra", false},
	} {
		t.Run(test.platform, func(t *testing.T) {
			t.Parallel()
			raw := validManifest + "  platforms:\n    linux/amd64: {executable: bin/linux}\n    " + test.platform + ": {executable: bin/variant}\n"
			parsed, err := Parse([]byte(raw))
			if !test.valid {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				require.Nil(t, parsed, "an invalid entry must not silently force the runtime fallback")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "bin/linux", Executable(parsed, "linux/amd64"))
			require.Equal(t, "bin/variant", Executable(parsed, test.platform))
			require.Equal(t, "bin/plugin", Executable(parsed, "windows/amd64"))
		})
	}
}
