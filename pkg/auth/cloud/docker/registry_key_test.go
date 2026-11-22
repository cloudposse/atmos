package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertToHostname verifies the port of docker/cli's
// credentials.ConvertToHostname used to match registry keys.
func TestConvertToHostname(t *testing.T) {
	tests := map[string]string{
		testECRRegistry:                       testECRRegistry,
		"https://" + testECRRegistry:          testECRRegistry,
		"http://" + testECRRegistry:           testECRRegistry,
		"https://" + testECRRegistry + "/":    testECRRegistry,
		"https://" + testECRRegistry + "/v2/": testECRRegistry,
		testECRRegistry + "/":                 testECRRegistry,
		testECRRegistry + "/v1/":              testECRRegistry,
		"https://myregistry.azurecr.io:443/":  "myregistry.azurecr.io:443",
		"https://user@myregistry.azurecr.io":  "myregistry.azurecr.io",
		"localhost:5000":                      "localhost:5000",
		// The cases below are taken from docker/cli's own TestConvertToHostname.
		"https://[::1]:6555/v2/":      "[::1]:6555",
		"https://::1:6555/v2/":        "[::1]:6555",
		"::1:6556":                    "::1:6556",
		"unix:///var/run/docker.sock": "unix:",
		// A scheme without a host falls through to the path cut, as in docker/cli.
		"https://":                               "https:",
		"":                                       "",
		"public.ecr.aws":                         "public.ecr.aws",
		"https://index.docker.io/v1/":            "index.docker.io",
		"oci://" + testECRRegistry + "/repo/img": testECRRegistry,
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, convertToHostname(in))
		})
	}
}

// TestConfigManager_WriteAuth_NormalizesRegistry verifies that a registry given
// with a scheme or trailing slash is stored under its hostname, as
// `docker login https://host/` does, so Docker finds it when pulling from host.
func TestConfigManager_WriteAuth_NormalizesRegistry(t *testing.T) {
	for _, registry := range []string{"https://" + testECRRegistry, "https://" + testECRRegistry + "/", testECRRegistry + "/"} {
		t.Run(registry, func(t *testing.T) {
			manager := writeTestConfig(t, `{}`, 0o600)

			require.NoError(t, manager.WriteAuth(registry, testECRUser, testECRPassword))

			got := decodeGeneric(t, readConfigFile(t, manager))
			assert.Equal(t, map[string]any{testECRRegistry: map[string]any{"auth": expectedInlineAuth()}}, got["auths"])
		})
	}
}

// TestConfigManager_WriteAuth_NormalizedRegistryUsesHostHelper verifies that the
// credHelpers lookup uses the normalized hostname, as docker/cli does.
func TestConfigManager_WriteAuth_NormalizedRegistryUsesHostHelper(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credHelpers": {"`+testECRRegistry+`": "ecr-login"}}`)
	helper.EXPECT().Store("ecr-login", testECRRegistry, testECRUser, testECRPassword).Return(nil)

	require.NoError(t, manager.WriteAuth("https://"+testECRRegistry+"/", testECRUser, testECRPassword))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{testECRRegistry: map[string]any{}}, got["auths"])
}

// TestConfigManager_WriteAuth_SchemeCredHelperKeyIgnored guards docker/cli
// parity: getConfiguredCredentialStore matches credHelpers keys exactly, so a
// scheme-prefixed key does not route the bare host. Honoring it would store the
// token in a helper Docker never asks for this registry.
func TestConfigManager_WriteAuth_SchemeCredHelperKeyIgnored(t *testing.T) {
	manager, _ := newMockedManager(t, `{"credHelpers": {"https://`+testECRRegistry+`": "ecr-login"}}`)

	// The strict mock fails the test on any helper call.
	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{testECRRegistry: map[string]any{"auth": expectedInlineAuth()}}, got["auths"])
}

// TestConfigManager_RemoveAuth_RemovesLegacyKeyVariants verifies that logout
// removes every auths key Docker would read for the host (docker/cli's file
// store falls back to keys whose ConvertToHostname matches), so no stale
// credential keeps the registry logged in. Other registries are untouched.
func TestConfigManager_RemoveAuth_RemovesLegacyKeyVariants(t *testing.T) {
	content := `{"auths": {` +
		`"https://` + testECRRegistry + `": {"auth": "eDp5"}, ` +
		`"http://` + testECRRegistry + `/": {"auth": "eDp5"}, ` +
		`"` + testECRRegistry + `/v2/": {"auth": "eDp5"}, ` +
		`"https://other.io": {"auth": "eDp5"}, ` +
		`"other.io": {"auth": "eDp5"}}}`
	manager := writeTestConfig(t, content, 0o600)

	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{
		"https://other.io": map[string]any{"auth": "eDp5"},
		"other.io":         map[string]any{"auth": "eDp5"},
	}, got["auths"])
}

// TestConfigManager_RemoveAuth_NormalizesRegistry verifies that a registry given
// with a scheme removes the hostname entry WriteAuth created.
func TestConfigManager_RemoveAuth_NormalizesRegistry(t *testing.T) {
	manager := writeTestConfig(t, `{"auths": {"`+testECRRegistry+`": {"auth": "eDp5"}, "keep.io": {}}}`, 0o600)

	require.NoError(t, manager.RemoveAuth("https://"+testECRRegistry+"/"))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{"keep.io": map[string]any{}}, got["auths"])
}

// TestConfigManager_RemoveAuth_LegacyKeyErasedThroughItsHelper verifies that a
// legacy key is erased from the helper Docker resolves for that exact key,
// with that key as the server URL, as `docker logout` does.
func TestConfigManager_RemoveAuth_LegacyKeyErasedThroughItsHelper(t *testing.T) {
	content := `{"credsStore": "desktop", "credHelpers": {"` + testECRRegistry + `": "ecr-login"}, ` +
		`"auths": {"https://` + testECRRegistry + `": {}, "` + testECRRegistry + `": {}}}`
	manager, helper := newMockedManager(t, content)
	helper.EXPECT().Erase("desktop", "https://"+testECRRegistry).Return(nil)
	helper.EXPECT().Erase("ecr-login", testECRRegistry).Return(nil)

	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Empty(t, got["auths"])
}

// TestHostFromURLFallback covers the branches of the ported fallback that
// net/url-parseable inputs never reach.
func TestHostFromURLFallback(t *testing.T) {
	tests := map[string]string{
		"no-scheme.io/path":    "",
		"https://":             "",
		"https://::1:6555/v2/": "[::1]:6555",
		"https://::1:/v2/":     "::1:",
		"https://::1:ab/v2/":   "::1:ab",
		"https://host.io/v2/":  "host.io",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, hostFromURLFallback(in))
		})
	}
}
