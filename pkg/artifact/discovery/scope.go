package discovery

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/artifact/discovery/cache"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// binding is one host binding of an artifact repository: the registry host an identity is bound
// to. Only names appear here, never credential values.
type binding struct {
	Repository string `json:"repository"`
	Kind       string `json:"kind"`
	Identity   string `json:"identity"`
}

// ScopeOf returns the cache scope of a configuration: a hash of the authentication configuration,
// the effective authentication realm, and the identity bound to each repository. Credentials
// never enter it. A snapshot written under one scope is invisible under another, so a different
// realm, auth configuration, or identity binding can never reuse another's discovery data.
func ScopeOf(config *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(config, "discovery.ScopeOf")()

	info, err := realm.GetRealm(config.Auth.Realm, config.CliConfigPath)
	if err != nil {
		return "", err
	}
	aliases := make([]string, 0, len(config.Artifacts.Repositories))
	for alias := range config.Artifacts.Repositories {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	bindings := make([]binding, 0, len(aliases))
	for _, alias := range aliases {
		repo := config.Artifacts.Repositories[alias]
		bindings = append(bindings, binding{Repository: alias, Kind: repo.Kind, Identity: repo.Identity})
	}
	return cache.Scope(config.Auth, info.Value, bindings)
}
