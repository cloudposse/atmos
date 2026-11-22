package realm

const (
	// SourceConfigPath is the realm source pkg/config records when no realm is
	// configured and it derives one from a hash of the atmos.yaml path.
	SourceConfigPath = "config-path"

	// SourceDefault is the realm source pkg/config records when no realm is
	// configured and there is no atmos.yaml path to derive one from.
	SourceDefault = "default"
)

// IsExplicitSource reports whether a realm with this source was configured
// explicitly, through ATMOS_AUTH_REALM or auth.realm. The sources of a derived
// realm (SourceAuto, SourceConfigPath and SourceDefault) are not explicit. An
// empty source counts as explicit: it means the realm was set directly on the
// auth configuration instead of being resolved by pkg/config.
func IsExplicitSource(source string) bool {
	switch source {
	case SourceAuto, SourceConfigPath, SourceDefault:
		return false
	default:
		return true
	}
}

// IsExplicit reports whether info is a realm the user configured explicitly.
//
// When no realm is configured, pkg/config stores a derived value in auth.realm
// (a hash of the atmos.yaml path, or "default") and records how it got it in
// AuthConfig.RealmSource. GetRealm then reports that value with SourceConfig,
// so configSource (AuthConfig.RealmSource) is what tells a derived realm from
// one set in atmos.yaml. A realm from ATMOS_AUTH_REALM is always explicit, and
// an empty realm never is.
func IsExplicit(info RealmInfo, configSource string) bool {
	switch {
	case info.Value == "" || info.Source == SourceAuto:
		return false
	case info.Source == SourceEnv:
		return true
	default:
		return IsExplicitSource(configSource)
	}
}
