package emulator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseYAMLFuncArgs(t *testing.T) {
	ref, key, err := parseYAMLFuncArgs("  aws   endpoint ")
	require.NoError(t, err)
	assert.Equal(t, "aws", ref)
	assert.Equal(t, "endpoint", key)

	// Qualified ref with a slash is preserved as a single field.
	ref, key, err = parseYAMLFuncArgs("aws/floci port")
	require.NoError(t, err)
	assert.Equal(t, "aws/floci", ref)
	assert.Equal(t, "port", key)

	for _, bad := range []string{"aws", "", "aws endpoint extra"} {
		_, _, err := parseYAMLFuncArgs(bad)
		require.Error(t, err, "input %q must be rejected", bad)
		assert.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
	}
}

func testEndpoint() *Endpoint {
	return &Endpoint{
		Target:  TargetAWS,
		Host:    "localhost",
		Ports:   map[int]int{4566: 34566},
		Region:  "us-east-1",
		Project: "demo-project",
	}
}

func TestValueForKey_ScalarKeys(t *testing.T) {
	ep := testEndpoint()
	profile := &Profile{Env: map[string]string{"AWS_ACCESS_KEY_ID": "test"}}

	cases := map[string]any{
		"endpoint":              "http://127.0.0.1:34566",
		"url":                   "http://127.0.0.1:34566",
		"host":                  "localhost",
		"port":                  "34566",
		"region":                "us-east-1",
		"project":               "demo-project",
		"env.AWS_ACCESS_KEY_ID": "test",
		"env.MISSING":           "",
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			got, err := valueForKey(ep, profile, "aws", "dev", key)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestValueForKey_UnknownKey(t *testing.T) {
	_, err := valueForKey(testEndpoint(), &Profile{}, "aws", "dev", "bogus")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
}

func TestValueForKey_PortNotBound(t *testing.T) {
	ep := &Endpoint{Target: TargetAWS, Host: "localhost", Ports: map[int]int{}}
	_, err := valueForKey(ep, &Profile{}, "aws", "dev", "port")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
}

func TestValueForKey_KubeconfigMaterializes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)
	t.Setenv("ATMOS_XDG_CACHE_HOME", "")

	const body = "apiVersion: v1\nkind: Config\n"
	profile := &Profile{Kubeconfig: []byte(body)}

	got, err := valueForKey(&Endpoint{Target: TargetKubernetes}, profile, "k3s/local", "plat/dev", "kubeconfig")
	require.NoError(t, err)

	path, ok := got.(string)
	require.True(t, ok)
	// Stack and ref separators are sanitized so the filename stays flat.
	assert.Equal(t, "plat_dev-k3s_local.kubeconfig", filepath.Base(path))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, string(data))
}

func TestValueForKey_KubeconfigMissing(t *testing.T) {
	_, err := valueForKey(&Endpoint{Target: TargetKubernetes}, &Profile{}, "k3s", "dev", "kubeconfig")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
}

func TestResolveYAMLFunc_NoResolverRegistered(t *testing.T) {
	saved := profileResolver
	profileResolver = nil
	t.Cleanup(func() { profileResolver = saved })

	_, err := ResolveYAMLFunc(nil, "aws endpoint", "dev", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorResolverUnavailable)
}

func TestResolveYAMLFunc_DispatchesToResolver(t *testing.T) {
	saved := profileResolver
	t.Cleanup(func() { profileResolver = saved })

	var gotRef, gotStack string
	RegisterProfileResolver(func(_ *schema.AtmosConfiguration, ref, stack string, _ *schema.ConfigAndStacksInfo) (*Endpoint, *Profile, error) {
		gotRef, gotStack = ref, stack
		return testEndpoint(), &Profile{}, nil
	})

	got, err := ResolveYAMLFunc(nil, "aws endpoint", "dev", nil)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:34566", got)
	assert.Equal(t, "aws", gotRef)
	assert.Equal(t, "dev", gotStack)
}
