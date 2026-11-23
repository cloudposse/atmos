package auth

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// deleteRealmKeyringEntries removes, for `atmos auth logout --all-realms --keychain`, the keyring
// entries of each configured identity in one realm. Besides the current-layout entry it removes
// what the per-identity keychain logout removes: the ambiguous previous-layout key that older
// versions deleted as the identity's own, and the identity's configuration digest record. Every
// step is best effort, as before: entries that do not exist are not errors.
func deleteRealmKeyringEntries(store types.CredentialStore, aliases []string, realm string) {
	sorted := append([]string(nil), aliases...)
	sort.Strings(sorted)

	deleter, canDeleteLegacy := store.(credentials.AmbiguousLegacyRealmDeleter)
	digests, canDeleteDigest := store.(credentials.ConfigDigestStore)

	for _, alias := range sorted {
		if err := store.Delete(alias, realm); err != nil {
			log.Debug("Failed to delete keyring entry", "alias", alias, "realm", realm, "error", err)
		}
		if canDeleteLegacy && realm != "" {
			entry, err := deleter.DeleteAmbiguousLegacyRealmEntry(alias, realm)
			switch {
			case err != nil:
				log.Debug("Failed to delete older keyring entry", "alias", alias, "realm", realm, "error", err)
			case entry != nil:
				log.Warn("Deleted an older keyring entry that may also have belonged to another realm",
					"alias", alias, "realm", realm, "key", entry.Key)
			}
		}
		if canDeleteDigest {
			if err := digests.DeleteExternalConfigDigest(alias, realm); err != nil {
				log.Debug("Failed to delete configuration digest record", "alias", alias, "realm", realm, "error", err)
			}
		}
	}
}
