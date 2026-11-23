package auth

import (
	"github.com/cloudposse/atmos/pkg/auth/realm"
)

// integrationRealmSource returns the realm source passed to integrations with
// m.realm.Value: the manager's realm source when the realm was configured
// explicitly (ATMOS_AUTH_REALM or auth.realm), else realm.SourceAuto.
//
// NewAuthManager resolves the realm from AuthConfig.Realm, which pkg/config
// fills with a hash of the atmos.yaml path when no realm is configured, so
// m.realm.Source alone reports that hash as SourceConfig. AuthConfig.RealmSource
// tells the two apart, so kube integrations keep the names earlier versions
// wrote unless the user opted into a realm.
func (m *manager) integrationRealmSource() string {
	configSource := ""
	if m.config != nil {
		configSource = m.config.RealmSource
	}
	if realm.IsExplicit(m.realm, configSource) {
		return m.realm.Source
	}
	return realm.SourceAuto
}

// explicitIntegrationRealm returns the realm value when it was configured
// explicitly, else "". Only an explicit realm changes the kubeconfig names the
// integrations write, so it is the only realm that belongs in a dedupe key.
func (m *manager) explicitIntegrationRealm() string {
	if m.integrationRealmSource() == realm.SourceAuto {
		return ""
	}
	return m.realm.Value
}
