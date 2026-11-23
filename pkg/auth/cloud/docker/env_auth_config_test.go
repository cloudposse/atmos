package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureWarnings routes the manager's user-facing warnings into a slice.
func captureWarnings(m *ConfigManager) *[]string {
	var warnings []string
	m.warn = func(msg string) { warnings = append(warnings, msg) }
	return &warnings
}

// TestConfigManager_WriteAuth_WarnsWhenDockerAuthConfigSet verifies that a login
// warns when DOCKER_AUTH_CONFIG is set, because docker/cli reads credentials
// from it before config.json, but still succeeds and writes config.json.
func TestConfigManager_WriteAuth_WarnsWhenDockerAuthConfigSet(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		contains []string
	}{
		{
			name:     "lists the registry",
			value:    `{"auths": {"` + testECRRegistry + `": {"auth": "eDp5"}}}`,
			contains: []string{dockerAuthConfigEnv, testECRRegistry, "will not be used"},
		},
		{
			name:     "lists other registries",
			value:    `{"auths": {"other.io": {"auth": "eDp5"}}}`,
			contains: []string{dockerAuthConfigEnv, testECRRegistry, "takes precedence"},
		},
		{
			name:     "not parseable",
			value:    `not json`,
			contains: []string{dockerAuthConfigEnv, testECRRegistry, "cannot use it", "Fix or unset"},
		},
		{
			name:     "entry without auth is rejected by Docker",
			value:    `{"auths": {"` + testECRRegistry + `": {}}}`,
			contains: []string{dockerAuthConfigEnv, "cannot use it", "missing key `auth`"},
		},
		{
			name:     "unknown field is rejected by Docker",
			value:    `{"auths": {"` + testECRRegistry + `": {"auth": "eDp5", "extra": 1}}}`,
			contains: []string{dockerAuthConfigEnv, "cannot use it"},
		},
		{
			name:     "auth that is not base64 is rejected by Docker",
			value:    `{"auths": {"` + testECRRegistry + `": {"auth": "!!!"}}}`,
			contains: []string{dockerAuthConfigEnv, "cannot use it"},
		},
		{
			name:     "auth without a username is rejected by Docker",
			value:    `{"auths": {"` + testECRRegistry + `": {"auth": "OnB3"}}}`,
			contains: []string{dockerAuthConfigEnv, "cannot use it", "base64(username:password)"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager := writeTestConfig(t, `{}`, 0o600)
			warnings := captureWarnings(manager)
			t.Setenv(dockerAuthConfigEnv, tc.value)

			require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

			require.Len(t, *warnings, 1)
			for _, want := range tc.contains {
				assert.Contains(t, (*warnings)[0], want)
			}
			if strings.Contains((*warnings)[0], "cannot use it") {
				assert.NotContains(t, (*warnings)[0], "will not be used", "an invalid value must not claim to shadow the login")
			}
			got := decodeGeneric(t, readConfigFile(t, manager))
			assert.Equal(t, map[string]any{testECRRegistry: map[string]any{"auth": expectedInlineAuth()}}, got["auths"])
		})
	}
}

// TestConfigManager_WriteAuth_NoWarningWithoutDockerAuthConfig verifies the
// negative path: an unset or empty DOCKER_AUTH_CONFIG produces no warning,
// matching docker/cli, which treats an empty value as unset.
func TestConfigManager_WriteAuth_NoWarningWithoutDockerAuthConfig(t *testing.T) {
	manager := writeTestConfig(t, `{}`, 0o600)
	warnings := captureWarnings(manager)
	t.Setenv(dockerAuthConfigEnv, "")

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	assert.Empty(t, *warnings)
}

// TestConfigManager_WriteAuth_NoWarningOnFailure verifies that a failed login
// does not also warn about DOCKER_AUTH_CONFIG.
func TestConfigManager_WriteAuth_NoWarningOnFailure(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop"}`)
	warnings := captureWarnings(manager)
	t.Setenv(dockerAuthConfigEnv, `{"auths": {}}`)
	helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).Return(errFakeHelper)

	require.Error(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	assert.Empty(t, *warnings)
}

// TestConfigManager_Warn_DefaultsToUI verifies that a manager without an
// injected sink still has a warning function.
func TestConfigManager_Warn_DefaultsToUI(t *testing.T) {
	assert.NotNil(t, (&ConfigManager{}).warner())
}
