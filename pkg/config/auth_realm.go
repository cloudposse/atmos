package config

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	authRealmEnvVar    = "ATMOS_AUTH_REALM"
	realmHashByteCount = 8 // 16 hex chars.
)

// setAuthRealm determines the auth realm and source for credential isolation.
// Precedence: ENV (ATMOS_AUTH_REALM) > auth.realm in config > hash(config path) > default.
func setAuthRealm(atmosConfig *schema.AtmosConfiguration) {
	if atmosConfig == nil {
		return
	}

	if envRealm := configEnvValue(authRealmEnvVar); envRealm != "" {
		atmosConfig.Auth.Realm = envRealm
		atmosConfig.Auth.RealmSource = realm.SourceEnv
		return
	}

	if atmosConfig.Auth.Realm != "" {
		atmosConfig.Auth.RealmSource = realm.SourceConfig
		return
	}

	if atmosConfig.CliConfigPath != "" {
		atmosConfig.Auth.Realm = hashRealmFromPath(atmosConfig.CliConfigPath)
		atmosConfig.Auth.RealmSource = realm.SourceConfigPath
		return
	}

	atmosConfig.Auth.Realm = "default"
	atmosConfig.Auth.RealmSource = realm.SourceDefault
}

func hashRealmFromPath(path string) string {
	cleaned := filepath.Clean(path)
	normalized := strings.ToLower(filepath.ToSlash(cleaned))
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", hash[:realmHashByteCount])
}
