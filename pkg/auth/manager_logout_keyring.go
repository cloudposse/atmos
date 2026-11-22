package auth

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// deleteAmbiguousLegacyKeyringEntry deletes, during a keychain logout, the previous-layout
// keyring key "atmos_<realm>_<name>" when that key is ambiguous (see
// credentials.AmbiguousLegacyRealmStore). Older versions stored this identity's or provider's
// credentials under it and deleted it at logout as their own; keeping it would let a later login
// offer to revive credentials the user just deleted. Because the same key was also used for
// another realm and name split, the deletion is logged as a warning that names the other owners.
//
// The returned errors follow deleteIdentityKeyringEntries: with bestEffort a failure is only
// logged, otherwise it is returned so the logout is reported as partial rather than complete.
func (m *manager) deleteAmbiguousLegacyKeyringEntry(name, logKey string, bestEffort bool) []error {
	deleter, ok := m.credentialStore.(credentials.AmbiguousLegacyRealmDeleter)
	if !ok || m.realm.Value == "" {
		return nil
	}
	entry, err := deleter.DeleteAmbiguousLegacyRealmEntry(name, m.realm.Value)
	if err != nil {
		if bestEffort {
			log.Debug("Failed to delete older keyring entry", logKey, name, logKeyRealm, m.realm.Value, "error", err)
			return nil
		}
		return []error{fmt.Errorf(errFormatWrapTwo, errUtils.ErrKeyringDeletion, name, err)}
	}
	if entry == nil {
		return nil
	}
	log.Warn("Deleted an older keyring entry that may also have belonged to another realm",
		logKey, name, logKeyRealm, m.realm.Value, "key", entry.Key, "other_owners", describeLegacyRealmOwners(entry.OtherOwners))
	return nil
}

// describeLegacyRealmOwners lists the other realm and name pairs of a previous-layout key.
func describeLegacyRealmOwners(owners []credentials.LegacyRealmOwner) string {
	parts := make([]string, 0, len(owners))
	for _, owner := range owners {
		parts = append(parts, fmt.Sprintf("%s in realm %s", owner.Alias, owner.Realm))
	}
	return strings.Join(parts, " or ")
}
