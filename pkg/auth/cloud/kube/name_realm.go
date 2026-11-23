package kube

// NameRealm returns the realm that kube user and default context names
// include, and the realms whose names still count as the entry's own.
//
// The value is the auth realm and explicit reports whether the user configured it
// (auth.realm or ATMOS_AUTH_REALM). An explicit realm is returned as nameRealm.
// A derived realm (Atmos hashes the atmos.yaml path when no realm is
// configured) must not change names users type, such as
// `kubectl --context <cluster-arn>`, so nameRealm is empty and the derived
// value is returned in legacyRealms instead: unreleased builds prefixed names
// with it, and those entries are cleaned up as this entry's own.
func NameRealm(value string, explicit bool) (nameRealm string, legacyRealms []string) {
	switch {
	case value == "":
		return "", nil
	case explicit:
		return value, nil
	default:
		return "", []string{value}
	}
}

// RealmScope is the realm a user name is written under and the realms older
// versions wrote it under (see ClusterInfo.LegacyRealms).
type RealmScope struct {
	// Realm is the realm included in the user name; empty for no realm.
	Realm string
	// LegacyRealms are additional realms whose names still count as the entry's own.
	LegacyRealms []string
}
