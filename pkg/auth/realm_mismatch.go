package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// realmMismatchWarningOnce ensures we only warn about realm mismatch once per execution.
var realmMismatchWarningOnce sync.Once

// awsDirName is the AWS credential subdirectory name.
const awsDirNameForMismatch = "aws"

// xdgDirPermissions is the permission mode for XDG config directories.
const xdgDirPermissions = 0o700

// emitRealmMismatchWarning checks if credentials exist under a different realm
// and emits a warning if so. This helps users who changed auth.realm or
// ATMOS_AUTH_REALM and don't understand why their cached credentials are gone.
// Only runs on credential-not-found paths (zero happy-path overhead).
func (m *manager) emitRealmMismatchWarning(identityName string) {
	realmMismatchWarningOnce.Do(func() {
		currentRealm := m.realm.Value

		// Check keyring for credentials under alternate realm.
		if alternateRealm := m.checkRealmMismatchKeyring(identityName, currentRealm); alternateRealm != "" {
			logRealmMismatchWarning(currentRealm, alternateRealm)
			return
		}

		// Check file system for credentials under alternate realm.
		if alternateRealm := checkRealmMismatchFiles(currentRealm); alternateRealm != "" {
			logRealmMismatchWarning(currentRealm, alternateRealm)
		}
	})
}

// checkRealmMismatchKeyring probes the keyring with an alternate realm to see
// if credentials exist there. Returns the alternate realm name if found, empty otherwise.
func (m *manager) checkRealmMismatchKeyring(identityName, currentRealm string) string {
	if m.credentialStore == nil {
		return ""
	}

	// If current realm is non-empty, probe with empty realm (backward-compatible path).
	// If current realm is empty, we can't probe keyring for specific realms without listing.
	if currentRealm != "" {
		if _, err := m.credentialStore.Retrieve(identityName, ""); err == nil {
			return "(no realm)"
		}
	}

	return ""
}

// checkRealmMismatchFiles scans the file system for credentials stored under a
// different realm than the current one. Returns a description of the found realm,
// or empty string if no mismatch detected.
func checkRealmMismatchFiles(currentRealm string) string {
	baseDir, err := xdg.GetXDGConfigDir("", xdgDirPermissions)
	if err != nil {
		return ""
	}

	if currentRealm != "" {
		return checkNoRealmCredentials(baseDir)
	}

	return scanForRealmCredentials(baseDir)
}

// checkNoRealmCredentials checks if credentials exist at the empty-realm path.
// Empty realm path: {baseDir}/aws/{provider}/credentials.
func checkNoRealmCredentials(baseDir string) string {
	awsDir := filepath.Join(baseDir, awsDirNameForMismatch)
	if hasCredentialFiles(awsDir) {
		return "(no realm)"
	}
	return ""
}

// scanForRealmCredentials scans for realm subdirectories containing credentials.
// Realm path: {baseDir}/{realm}/aws/{provider}/credentials.
func scanForRealmCredentials(baseDir string) string {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == awsDirNameForMismatch {
			continue
		}
		awsDir := filepath.Join(baseDir, entry.Name(), awsDirNameForMismatch)
		if hasCredentialFiles(awsDir) {
			return entry.Name()
		}
	}
	return ""
}

// hasCredentialFiles checks if the given aws directory contains any credential files.
// It looks for {awsDir}/{provider}/credentials pattern.
func hasCredentialFiles(awsDir string) bool {
	providerDirs, err := os.ReadDir(awsDir)
	if err != nil {
		return false
	}
	for _, provider := range providerDirs {
		if !provider.IsDir() {
			continue
		}
		credFile := filepath.Join(awsDir, provider.Name(), "credentials")
		if _, err := os.Stat(credFile); err == nil {
			return true
		}
	}
	return false
}

// logRealmMismatchWarning emits a warning about credentials existing under a different realm.
func logRealmMismatchWarning(currentRealm, alternateRealm string) {
	currentDisplay := currentRealm
	if currentDisplay == "" {
		currentDisplay = "(no realm)"
	}

	log.Warn(fmt.Sprintf(
		"Credentials found under realm %q but current realm is %q. "+
			"This typically happens after changing %s in config or %s. "+
			"Run 'atmos auth login' to re-authenticate under the current realm.",
		alternateRealm,
		currentDisplay,
		"auth.realm",
		"ATMOS_AUTH_REALM",
	))
}
