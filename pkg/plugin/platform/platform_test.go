package platform

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSelectCompatibleVariant(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		target, want string
		available    []string
	}{
		{"linux/arm/v7", "linux/arm/v7", []string{"linux/arm/v6", "linux/arm/v7"}},
		{"linux/arm/v7", "linux/arm/v6", []string{"linux/arm", "linux/arm/v6"}},
		{"linux/arm/v5", "", []string{"linux/arm/v7"}},
		{"linux/arm", "", []string{"linux/arm/v7"}},
		{"linux/amd64/v3", "linux/amd64/v2", []string{"linux/amd64", "linux/amd64/v2", "linux/amd64/v4"}},
		{"linux/amd64", "linux/amd64/v1", []string{"linux/amd64/v1"}},
		{"darwin/arm64", "darwin/arm64/v8.0", []string{"darwin/arm64/v8.0", "darwin/arm64/v8.1"}},
		{"linux/arm64/v9.0", "linux/arm64/v8.5", []string{"linux/arm64/v8.5", "linux/arm64/v8.6"}},
		{"linux/arm64/v8.2", "linux/arm64", []string{"linux/arm64", "linux/arm64/v8.3"}},
		{"linux/arm64", "", []string{"linux/arm/v7", "darwin/arm64"}},
		{"linux/amd64", "", []string{"linux/386", "windows/amd64"}},
		{"linux/amd64/v1", "linux/amd64/v1", []string{"linux/amd64", "linux/amd64/v1"}},
	} {
		t.Run(tc.target+"_"+tc.want, func(t *testing.T) {
			t.Parallel()
			available := make(map[string]string)
			for _, key := range tc.available {
				available[key] = "digest"
			}
			got, err := Select(available, tc.target)
			if tc.want == "" {
				require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRejectInvalidPlatforms(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"linux", "linux/arm/v99", "linux/arm64/v8.10", "linux/amd64/v0", "linux/386/v1", "Linux/amd64", "linux/amd64/../a", "linux/amd64/", "linux/arm64/v8.0,lse"} {
		_, err := Parse(value)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, value)
		_, err = Select(map[string]bool{}, value)
		require.Error(t, err)
	}
	_, err := Select(map[string]bool{"bad": true}, "linux/amd64")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestCurrentUsesBuildMetadataNotEnvironment(t *testing.T) {
	for _, tc := range []struct{ arch, key, value, want string }{
		{"arm", "GOARM", "7,hardfloat", "linux/arm/v7"},
		{"arm", "GOARM", "", "linux/arm/v5"},
		{"amd64", "GOAMD64", "v3", "linux/amd64/v3"},
		{"amd64", "GOAMD64", "v1", "linux/amd64"},
		{"arm64", "GOARM64", "v8.2,lse", "linux/arm64/v8.2"},
		{"arm64", "GOARM64", "v8.0", "linux/arm64"},
		{"arm64", "GOARM64", "bogus", "linux/arm64"},
		{"riscv64", "GORISCV64", "rva20u64", "linux/riscv64"},
	} {
		t.Run(tc.arch+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, "hostile-environment")
			require.Equal(t, tc.want, current("linux", tc.arch, []debug.BuildSetting{{Key: tc.key, Value: tc.value}}))
		})
	}
	require.NotEmpty(t, Current())
}

func TestCompilerEnvironment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ target, key, value string }{
		{"linux/arm/v7", "GOARM", "7,softfloat"},
		{"linux/arm", "GOARM", "5,softfloat"},
		{"linux/amd64", "GOAMD64", "v1"},
		{"linux/amd64/v3", "GOAMD64", "v3"},
		{"darwin/arm64", "GOARM64", "v8.0"},
		{"linux/arm64/v9", "GOARM64", "v9.0"},
	} {
		p, err := Parse(tc.target)
		require.NoError(t, err)
		env := p.Environment([]string{"GOOS=bad", "GOARCH=bad", "GOARM=bad", "GOAMD64=bad", "GOARM64=bad", "CGO_ENABLED=1", "KEEP=yes"})
		require.Contains(t, env, "GOOS="+p.OS)
		require.Contains(t, env, "GOARCH="+p.Architecture)
		require.Contains(t, env, tc.key+"="+tc.value)
		require.Contains(t, env, "CGO_ENABLED=0")
		require.Contains(t, env, "KEEP=yes")
		require.NotContains(t, env, tc.key+"=bad")
	}
}
