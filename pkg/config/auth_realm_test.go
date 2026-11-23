package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHashRealmFromPath_HasAutoValueFormat pins the coupling the credential store relies on to
// recognize the auto realm by its value alone (pkg/auth/credentials/keyring_legacy_auto_realm.go):
// every realm derived from the atmos.yaml path has realm.HasAutoValueFormat's exact format.
func TestHashRealmFromPath_HasAutoValueFormat(t *testing.T) {
	for _, path := range []string{
		filepath.Join("repo", "atmos.yaml"),
		filepath.Join("Users", "Me", "Infra Repo", "atmos.yaml"),
		"atmos.yaml",
	} {
		value := hashRealmFromPath(path)
		assert.True(t, realm.HasAutoValueFormat(value), "realm %q derived from %q", value, path)
		assert.NoError(t, realm.Validate(value))
	}
}

// TestSetAuthRealm_Sources verifies the realm sources the credential store tells apart: an
// explicit realm is reported as "env" or "config", the derived one as "config-path".
func TestSetAuthRealm_Sources(t *testing.T) {
	t.Setenv(authRealmEnvVar, "")

	derived := &schema.AtmosConfiguration{CliConfigPath: filepath.Join("repo", "atmos.yaml")}
	setAuthRealm(derived)
	assert.Equal(t, "config-path", derived.Auth.RealmSource)
	assert.True(t, realm.HasAutoValueFormat(derived.Auth.Realm))

	configured := &schema.AtmosConfiguration{CliConfigPath: "atmos.yaml", Auth: schema.AuthConfig{Realm: "acme"}}
	setAuthRealm(configured)
	assert.Equal(t, realm.SourceConfig, configured.Auth.RealmSource)

	t.Setenv(authRealmEnvVar, "from-env")
	fromEnv := &schema.AtmosConfiguration{CliConfigPath: "atmos.yaml"}
	setAuthRealm(fromEnv)
	assert.Equal(t, realm.SourceEnv, fromEnv.Auth.RealmSource)
	assert.Equal(t, "from-env", fromEnv.Auth.Realm)
}
