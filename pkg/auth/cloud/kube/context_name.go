package kube

import (
	"slices"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/cloudposse/atmos/pkg/perf"
)

// realmContextSeparator joins the auth realm and the cluster ID in a default
// context name. Realms cannot contain "/", so the realm prefix is unambiguous.
const realmContextSeparator = "/"

// ContextName returns the kube context name written for a cluster entry: the
// alias when one is set, otherwise the cluster ID prefixed with the auth realm
// (<realm>/<cluster-id>) when the realm is non-empty, otherwise the cluster ID.
// The realm prefix keeps two repositories with different realms that write the
// same cluster from sharing, and repointing, one context.
func ContextName(clusterID, alias, realm string) string {
	defer perf.Track(nil, "kube.ContextName")()

	if alias != "" {
		return alias
	}
	if realm != "" {
		return realm + realmContextSeparator + clusterID
	}
	return clusterID
}

// ContextNames returns ContextName followed by the context name older versions
// wrote for the same entry: the unprefixed cluster ID, when the realm-prefixed
// default name is in use. Cleanup uses it to also remove a context written
// before default context names included the realm.
func ContextNames(clusterID, alias, realm string) []string {
	defer perf.Track(nil, "kube.ContextNames")()

	name := ContextName(clusterID, alias, realm)
	if name != clusterID && alias == "" {
		return []string{name, clusterID}
	}
	return []string{name}
}

// OwnedUserNames returns the exec-plugin user names that belong to the cluster
// entry described by info: the current UserName, the name without the realm
// (when a realm is set), and the info.LegacyUserNames older versions wrote.
// Contexts that point at any other user belong to someone else.
func OwnedUserNames(info *ClusterInfo) []string {
	defer perf.Track(nil, "kube.OwnedUserNames")()

	names := []string{UserName(info)}
	if info.Realm != "" {
		preRealm := *info
		preRealm.Realm = ""
		names = append(names, UserName(&preRealm))
	}
	for _, name := range info.LegacyUserNames {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// unprefixedContextMigration describes the unprefixed default context an older
// version wrote for a cluster entry that now uses a realm-prefixed name.
type unprefixedContextMigration struct {
	// contextName is the old, unprefixed context name (the cluster ID).
	contextName string
	// currentUser is the user the new context points at; it is never removed.
	currentUser string
	// ownedUsers are the user names that mark the old context as this entry's.
	ownedUsers []string
}

// newUnprefixedContextMigration returns the migration for writing info under
// alias, or nil when the context name is not realm-prefixed (an alias is set or
// the realm is empty), so there is no older unprefixed name to migrate.
func newUnprefixedContextMigration(info *ClusterInfo, alias string) *unprefixedContextMigration {
	if alias != "" || info.Realm == "" {
		return nil
	}
	return &unprefixedContextMigration{
		contextName: info.ID,
		currentUser: UserName(info),
		ownedUsers:  OwnedUserNames(info),
	}
}

// apply removes the old unprefixed context from config when it points at one
// of this entry's own users, then removes this entry's older users that no
// context references any more. A context that points at another user is never
// touched. Returns true when config was changed. A nil migration is a no-op.
func (u *unprefixedContextMigration) apply(config *clientcmdapi.Config) bool {
	if u == nil {
		return false
	}
	if !contextOwnedBy(config, u.contextName, u.ownedUsers) {
		return false
	}
	delete(config.Contexts, u.contextName)
	if config.CurrentContext == u.contextName {
		config.CurrentContext = ""
	}
	for _, name := range u.ownedUsers {
		if name != u.currentUser && !userReferenced(config, name) {
			delete(config.AuthInfos, name)
		}
	}
	return true
}
