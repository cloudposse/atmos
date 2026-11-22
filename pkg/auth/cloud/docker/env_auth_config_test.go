package docker

import (
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
			contains: []string{dockerAuthConfigEnv, testECRRegistry, "takes precedence"},
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
