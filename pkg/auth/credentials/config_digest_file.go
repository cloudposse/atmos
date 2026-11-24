package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// The configDigestDirSuffix is appended to a file keyring's directory to name the directory
	// of its configuration digest records, so the records stay with that keyring (and a custom
	// keyring path keeps its own records) without appearing as keyring entries.
	configDigestDirSuffix = ".config-digests"
	// The configDigestDirPerm is the permission of the record directory (owner only).
	configDigestDirPerm = 0o700
	// The configDigestFilePerm is the permission of a record file (owner only).
	configDigestFilePerm = 0o600
)

// Configuration digest records describe credentials an identity keeps in its own files (for
// example AWS session credentials). They hold nothing secret, and the identities that write them
// often never use the keyring at all (aws/user with inline keys, aws/credential-process). The
// persistent backends therefore keep them in plain files instead of the keyring: a new keyring
// entry can make macOS show a Keychain prompt (a non-interactive login then hangs), and the file
// keyring asks for its password, for identities that never needed either. A record is only as
// trustworthy as the credential files it describes, which live with the same owner-only
// permissions. The memory backend (process-local, no prompts) keeps them in its keyring.

// systemConfigDigestDir returns the record directory of the system keyring backend, under the
// Atmos XDG cache directory. Losing it only costs one re-authentication of an intermediate chain
// step, so the cache directory is the right home.
func systemConfigDigestDir() string {
	return xdg.LookupXDGCacheDir(filepath.Join("auth", "config-digests"))
}

// fileKeyringConfigDigestDir returns the record directory of a file keyring stored in dir.
func fileKeyringConfigDigestDir(dir string) string {
	return filepath.Clean(dir) + configDigestDirSuffix
}

// configDigestFile returns the record file of alias within realm. The name is a hash of the
// record key, so identity names with path separators or characters a file system rejects
// (such as ':' on Windows) map to one safe file name, and realms stay separated.
func (s *keyringCredentialStore) configDigestFile(alias, realm string) string {
	// codeql[go/weak-sensitive-data-hashing] -- hashes the record key (realm and identity name) to
	// derive a deterministic file name; no password or credential is hashed or verified here.
	sum := sha256.Sum256([]byte(buildConfigDigestKey(alias, realm)))
	return filepath.Join(s.digestDir, hex.EncodeToString(sum[:])+".json")
}

// storeConfigDigestFile writes the record of alias within realm to its file.
func (s *keyringCredentialStore) storeConfigDigestFile(alias, realm string, record ExternalConfigDigest) error {
	// A struct of strings always marshals.
	data, _ := json.Marshal(record)
	if err := os.MkdirAll(s.digestDir, configDigestDirPerm); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to create configuration digest directory: %w", err))
	}
	if err := filesystem.NewOSFileSystem().WriteFileAtomic(s.configDigestFile(alias, realm), data, configDigestFilePerm); err != nil {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to store configuration digest: %w", err))
	}
	return nil
}

// retrieveConfigDigestFile reads the record of alias within realm from its file.
func (s *keyringCredentialStore) retrieveConfigDigestFile(alias, realm string) (ExternalConfigDigest, error) {
	var record ExternalConfigDigest
	data, err := os.ReadFile(s.configDigestFile(alias, realm))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return record, errors.Join(ErrCredentialStore, ErrDataNotFound)
		}
		return record, errors.Join(ErrCredentialStore, fmt.Errorf("failed to retrieve configuration digest: %w", err))
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return ExternalConfigDigest{}, errors.Join(ErrCredentialStore, fmt.Errorf("failed to unmarshal configuration digest: %w", err))
	}
	return record, nil
}

// deleteConfigDigestFile removes the record file of alias within realm. It is idempotent.
func (s *keyringCredentialStore) deleteConfigDigestFile(alias, realm string) error {
	if err := os.Remove(s.configDigestFile(alias, realm)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(ErrCredentialStore, fmt.Errorf("failed to delete configuration digest: %w", err))
	}
	return nil
}
