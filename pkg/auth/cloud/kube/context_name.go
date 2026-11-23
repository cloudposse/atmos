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

// ContextNames returns ContextName followed by the default context names that
// older versions wrote for the same entry: the unprefixed cluster ID, when the
// realm-prefixed default name is in use, and <legacy-realm>/<cluster-id> for
// each of legacyRealms (see ClusterInfo.LegacyRealms). With an alias, only the
// alias is returned. Cleanup uses it to also remove contexts written under
// those older names.
func ContextNames(clusterID, alias, realm string, legacyRealms ...string) []string {
	defer perf.Track(nil, "kube.ContextNames")()

	name := ContextName(clusterID, alias, realm)
	names := []string{name}
	if alias != "" {
		return names
	}
	if name != clusterID {
		names = append(names, clusterID)
	}
	for _, legacyRealm := range legacyRealms {
		if legacyName := ContextName(clusterID, "", legacyRealm); !slices.Contains(names, legacyName) {
			names = append(names, legacyName)
		}
	}
	return names
}

// OwnedUserNames returns the exec-plugin user names that belong to the cluster
// entry described by info: the current UserName, the name without the realm
// (when a realm is set), the names under each of info.LegacyRealms, and the
// info.LegacyUserNames older versions wrote. Contexts that point at any other
// user belong to someone else.
func OwnedUserNames(info *ClusterInfo) []string {
	defer perf.Track(nil, "kube.OwnedUserNames")()

	names := []string{UserName(info)}
	appendName := func(name string) {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	// The unhashed form earlier builds wrote for the same entry.
	appendName(UnhashedUserName(info))
	if info.Realm != "" {
		for _, name := range userNameFormsInRealm(info, "") {
			appendName(name)
		}
	}
	for _, legacyRealm := range info.LegacyRealms {
		for _, name := range userNameFormsInRealm(info, legacyRealm) {
			appendName(name)
		}
	}
	for _, name := range info.LegacyUserNames {
		appendName(name)
	}
	return names
}

// userNameFormsInRealm returns the UserNameForms info would have under realm.
func userNameFormsInRealm(info *ClusterInfo, realm string) []string {
	scoped := *info
	scoped.Realm = realm
	return UserNameForms(&scoped)
}

// contextMigration describes the entries older versions wrote for a cluster
// entry that a write replaces: default contexts under older names, and older
// users that no context will reference once the entry is written.
type contextMigration struct {
	// contextName is the context being written; it is replaced by the merge.
	contextName string
	// oldContextNames are the default context names older versions wrote
	// for this entry (see ContextNames), excluding contextName.
	oldContextNames []string
	// currentUser is the user the new context points at; it is never removed.
	currentUser string
	// ownedUsers are the user names that mark a context as this entry's.
	ownedUsers []string
}

// newContextMigration returns the migration for writing info under alias.
func newContextMigration(info *ClusterInfo, alias string) *contextMigration {
	names := ContextNames(info.ID, alias, info.Realm, info.LegacyRealms...)
	return &contextMigration{
		contextName:     names[0],
		oldContextNames: names[1:],
		currentUser:     UserName(info),
		ownedUsers:      OwnedUserNames(info),
	}
}

// apply removes from config each old context name that points at one of this
// entry's own users, then removes this entry's older users that no context
// other than the one being written references (that context is repointed at
// currentUser by the merge). A context or user of anyone else is never
// touched. Returns true when config was changed. A nil migration is a no-op.
func (u *contextMigration) apply(config *clientcmdapi.Config) bool {
	if u == nil {
		return false
	}
	changed := false
	for _, name := range u.oldContextNames {
		if !contextOwnedBy(config, name, u.ownedUsers) {
			continue
		}
		delete(config.Contexts, name)
		if config.CurrentContext == name {
			config.CurrentContext = ""
		}
		changed = true
	}
	for _, name := range u.ownedUsers {
		if name == u.currentUser {
			continue
		}
		if _, exists := config.AuthInfos[name]; !exists || userReferencedOutside(config, name, u.contextName) {
			continue
		}
		delete(config.AuthInfos, name)
		changed = true
	}
	return changed
}

// userReferencedOutside reports whether a context other than contextName uses
// the user.
func userReferencedOutside(config *clientcmdapi.Config, userName, contextName string) bool {
	for name, kubeContext := range config.Contexts {
		if name != contextName && kubeContext != nil && kubeContext.AuthInfo == userName {
			return true
		}
	}
	return false
}

// ownsLegacyContext reports whether current is this entry's own context that
// still points at an older user: it targets the same cluster and namespace as
// next, and its user is one of the entry's owned users other than the current
// one. Writing next replaces it, so it is a migration, not a conflict. A nil
// migration owns nothing.
func (u *contextMigration) ownsLegacyContext(current, next *clientcmdapi.Context) bool {
	if u == nil || current == nil || next == nil {
		return false
	}
	return current.Cluster == next.Cluster &&
		current.Namespace == next.Namespace &&
		current.AuthInfo != u.currentUser &&
		slices.Contains(u.ownedUsers, current.AuthInfo)
}
