package sops

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/secrets/providers"

	"filippo.io/age"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSopsProvider_SupportsScope proves the SOPS provider accepts the empty (instance default),
// stack, and instance scopes, and rejects an unknown scope value.
func TestSopsProvider_SupportsScope(t *testing.T) {
	p, _ := newAgeProvider(t)

	assert.True(t, p.SupportsScope(""), "empty scope (instance default) must always be supported")
	assert.True(t, p.SupportsScope(providers.ScopeStack))
	assert.True(t, p.SupportsScope(providers.ScopeInstance))
	assert.False(t, p.SupportsScope(providers.Scope("environment")), "an unknown scope is not supported")
}

// TestSopsProvider_KeyGroups_NoRecipientsNoSopsConfig proves that creating a fresh file with
// neither `spec.age_recipients` nor a discoverable .sops.yaml creation rule surfaces
// ErrSopsRecipients (the FindConfigFile-not-found branch of keyGroups).
func TestSopsProvider_KeyGroups_NoRecipientsNoSopsConfig(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "absent.txt"))
	t.Setenv("SOPS_AGE_KEY", "")

	// Place the encrypted file in an isolated temp dir with no .sops.yaml anywhere up the tree.
	dir := t.TempDir()
	file := filepath.Join(dir, "dev.enc.yaml")
	p := ageProviderWith(t, file, "", "") // no recipients, no key file.

	// Set must create the file, which requires resolving recipients first.
	err := p.Set(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}, "v")
	require.ErrorIs(t, err, ErrSopsRecipients)
}

// TestSopsProvider_KeyGroups_InvalidRecipients proves malformed `spec.age_recipients` is reported
// as ErrSopsRecipients (the MasterKeysFromRecipients failure branch of keyGroups).
func TestSopsProvider_KeyGroups_InvalidRecipients(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "dev.enc.yaml")
	p := ageProviderWith(t, file, "not-a-valid-age-recipient", "")

	err := p.Set(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}, "v")
	require.ErrorIs(t, err, ErrSopsRecipients)
}

// TestSopsProvider_KeyGroups_MalformedSopsConfig proves a `.sops.yaml` that cannot be parsed as a
// SOPS config surfaces ErrSopsRecipients (the LoadCreationRuleForFile failure branch of keyGroups).
func TestSopsProvider_KeyGroups_MalformedSopsConfig(t *testing.T) {
	dir := t.TempDir()
	// A malformed .sops.yaml: present (so FindConfigFile succeeds) but not valid SOPS config.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sops.yaml"), []byte(":\n\t- not valid yaml ::"), 0o600))

	file := filepath.Join(dir, "dev.enc.yaml")
	p := ageProviderWith(t, file, "", "")

	err := p.Set(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}, "v")
	require.ErrorIs(t, err, ErrSopsRecipients)
}

// TestSopsProvider_KeyGroups_NoMatchingCreationRule proves a valid `.sops.yaml` whose creation
// rules do not match the file surfaces ErrSopsRecipients (the empty-KeyGroups branch of keyGroups).
func TestSopsProvider_KeyGroups_NoMatchingCreationRule(t *testing.T) {
	dir := t.TempDir()
	// Valid SOPS config, but its creation rule only matches *.json, not our *.enc.yaml file.
	sopsConfig := "creation_rules:\n  - path_regex: \\.json$\n    age: age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsx7d6q\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".sops.yaml"), []byte(sopsConfig), 0o600))

	file := filepath.Join(dir, "dev.enc.yaml")
	p := ageProviderWith(t, file, "", "")

	err := p.Set(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}, "v")
	require.ErrorIs(t, err, ErrSopsRecipients)
}

// TestSopsProvider_DecryptDoc_CorruptFile proves that a non-SOPS / corrupt file surfaces
// ErrSopsDecrypt at LoadEncryptedFile time (before any key is even consulted).
func TestSopsProvider_DecryptDoc_CorruptFile(t *testing.T) {
	identity, keyFile, file := ageKeyFileFixture(t)

	// Write garbage that is not a SOPS document where the provider expects the encrypted file.
	require.NoError(t, os.WriteFile(file, []byte("this: is\nnot: a sops file\n"), 0o600))

	reader := ageProviderWith(t, file, identity.Recipient().String(), keyFile)
	_, err := reader.Get(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"})
	require.ErrorIs(t, err, ErrSopsDecrypt)
}

// TestSopsProvider_EditFile_CorruptFile proves the edit (Set) path on an existing-but-corrupt file
// surfaces ErrSopsDecrypt from LoadEncryptedFile (the mutation path, not the read path).
func TestSopsProvider_EditFile_CorruptFile(t *testing.T) {
	identity, keyFile, file := ageKeyFileFixture(t)

	require.NoError(t, os.WriteFile(file, []byte("garbage: not-sops\n"), 0o600))

	p := ageProviderWith(t, file, identity.Recipient().String(), keyFile)
	// createIfMissing is true for Set, but the file exists, so it takes the load+decrypt path.
	err := p.Set(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}, "v")
	require.ErrorIs(t, err, ErrSopsDecrypt)
}

// TestSopsProvider_EditFile_ReadErrorNotNotExist proves that a ReadFile error which is NOT
// "file does not exist" (here: the path is a directory) surfaces ErrSopsDecrypt rather than the
// friendly file-not-found error. Skipped on Windows where reading a directory may not error.
func TestSopsProvider_EditFile_ReadErrorNotNotExist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory-read semantics differ on Windows")
	}
	identity, keyFile, _ := ageKeyFileFixture(t)

	// Make the configured "file" path actually a directory: os.ReadFile returns a non-NotExist error.
	dir := t.TempDir()
	asDir := filepath.Join(dir, "dev.enc.yaml")
	require.NoError(t, os.Mkdir(asDir, 0o755))

	p := ageProviderWith(t, asDir, identity.Recipient().String(), keyFile)
	err := p.Set(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}, "v")
	require.ErrorIs(t, err, ErrSopsDecrypt)
	require.NotErrorIs(t, err, ErrSecretFileNotFound)
}

// TestSopsProvider_WriteNewFile_MkdirAllFails proves that when the parent of the target file is a
// regular file (so MkdirAll cannot create the directory), writeNewFile surfaces ErrSopsEncrypt.
// Skipped on Windows where the failure mode/permissions differ.
func TestSopsProvider_WriteNewFile_MkdirAllFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("MkdirAll-over-a-file semantics differ on Windows")
	}
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	dir := t.TempDir()
	// Create a regular file that will stand in for an ancestor directory of the target.
	parentAsFile := filepath.Join(dir, "notadir")
	require.NoError(t, os.WriteFile(parentAsFile, []byte("x"), 0o600))

	// The target's directory (notadir/sub) cannot be created because `notadir` is a file =>
	// writeNewFile's MkdirAll fails. Reset calls writeNewFile directly (no prior ReadFile), so the
	// file-does-not-exist read path is bypassed and MkdirAll is genuinely exercised.
	file := filepath.Join(parentAsFile, "sub", "dev.enc.yaml")
	p := ageProviderWith(t, file, identity.Recipient().String(), "")

	resettable, ok := p.(providers.FileResettable)
	require.True(t, ok)
	err = resettable.Reset(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"})
	require.ErrorIs(t, err, ErrSopsEncrypt)
}

// TestSopsProvider_WriteNewFile_WriteFileFails proves that when the target directory is read-only
// (so os.WriteFile cannot create the file), writeNewFile surfaces an error. Skipped on Windows and
// when running as root (where the directory mode is not enforced).
func TestSopsProvider_WriteNewFile_WriteFileFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	require.NoError(t, os.Mkdir(roDir, 0o555)) // read+execute, no write.
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	// The parent (roDir) already exists so MkdirAll is a no-op; WriteFile then fails.
	file := filepath.Join(roDir, "dev.enc.yaml")
	p := ageProviderWith(t, file, identity.Recipient().String(), "")

	resettable, ok := p.(providers.FileResettable)
	require.True(t, ok)
	err = resettable.Reset(providers.Coordinate{Stack: "dev", Component: "api", Key: "K"})
	require.Error(t, err)
}

// TestExpandKeyPath_EnvExpansion proves `$ENV` references are expanded by expandKeyPath.
func TestExpandKeyPath_EnvExpansion(t *testing.T) {
	base := filepath.Join(t.TempDir(), "secret", "place")
	t.Setenv("ATMOS_TEST_KEYPATH", base)
	got, err := expandKeyPath("$ATMOS_TEST_KEYPATH/keys.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.ToSlash(filepath.Join(base, "keys.txt")), filepath.ToSlash(got))
}

// TestExpandKeyPath_TildeExpansion proves a leading `~` is expanded to the home directory.
func TestExpandKeyPath_TildeExpansion(t *testing.T) {
	home, err := homedir.Dir()
	require.NoError(t, err)

	got, err := expandKeyPath(filepath.FromSlash("~/keys.txt"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "keys.txt"), got)
}

// TestExpandKeyPath_Error proves that a malformed `~user` form (unsupported) surfaces an error
// from expandKeyPath rather than a partially-expanded path.
func TestExpandKeyPath_Error(t *testing.T) {
	_, err := expandKeyPath("~nonexistentuser/keys.txt")
	require.Error(t, err)
}

// TestSopsProvider_AgeKeyFileClient_ExpandError proves an unexpandable `spec.age_key_file` (a
// `~user` form) surfaces ErrSopsAgeKeyFile via the expandKeyPath failure branch of ageKeyFileClient.
func TestSopsProvider_AgeKeyFileClient_ExpandError(t *testing.T) {
	identity, _, file := ageKeyFileFixture(t)
	coord := providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}

	writer := ageProviderWith(t, file, identity.Recipient().String(), "")
	require.NoError(t, writer.Set(coord, "v"))

	reader := ageProviderWith(t, file, "", "~nonexistentuser/keys.txt")
	_, err := reader.Get(coord)
	require.ErrorIs(t, err, ErrSopsAgeKeyFile)
}

// TestSopsProvider_KeyClient_EnvFallback proves that with no inline key, no store, and no key file
// configured, keyClient falls through to the default local client, which resolves the key from
// SOPS_AGE_KEY_FILE — i.e. the back-compatible env-fallback branch.
func TestSopsProvider_KeyClient_EnvFallback(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(keyFile, []byte(identity.String()+"\n"), 0o600))
	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)
	t.Setenv("SOPS_AGE_KEY", "")

	file := filepath.Join(dir, "dev.enc.yaml")
	// No age_key_file in the spec => keyClient default branch (NewLocalClient) is used for decrypt.
	p := ageProviderWith(t, file, identity.Recipient().String(), "")
	sp := p.(*sopsProvider)
	require.Empty(t, sp.ageKey)
	require.Empty(t, sp.ageKeyFile)
	require.Empty(t, sp.ageKeyStore)

	coord := providers.Coordinate{Stack: "dev", Component: "api", Key: "K"}
	require.NoError(t, p.Set(coord, "v"))
	got, err := p.Get(coord)
	require.NoError(t, err, "default local client must resolve the key from SOPS_AGE_KEY_FILE")
	assert.Equal(t, "v", got)
}

// TestNewSopsProvider_NotFound proves an unknown provider name (absent from both the section and
// atmos.yaml) is reported as providers.ErrProviderNotFound.
func TestNewSopsProvider_NotFound(t *testing.T) {
	_, err := New(&schema.AtmosConfiguration{}, "missing", nil)
	require.ErrorIs(t, err, providers.ErrProviderNotFound)
}

// TestNewSopsProvider_NoFileSpec proves a provider declared without `spec.file` is now valid: the
// file path is derived in code from each secret's scope under the default `spec.path` (`secrets`).
// Stack-scoped secrets land in `secrets/<stack>.enc.yaml`, instance-scoped in
// `secrets/<stack>.<component>.enc.yaml`, which is collision-safe by construction.
func TestNewSopsProvider_NoFileSpec(t *testing.T) {
	section := map[string]any{
		"dev-sops": map[string]any{"kind": "sops/age", "spec": map[string]any{}},
	}
	prov, err := New(&schema.AtmosConfiguration{}, "dev-sops", section)
	require.NoError(t, err)

	sp, ok := prov.(*sopsProvider)
	require.True(t, ok)

	stackFile, err := sp.FilePath(providers.Coordinate{Stack: "dev", Scope: providers.ScopeStack})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("secrets", "dev"+sopsFileExt), stackFile)

	instanceFile, err := sp.FilePath(providers.Coordinate{Stack: "dev", Component: "api", Scope: providers.ScopeInstance})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("secrets", "dev.api"+sopsFileExt), instanceFile)
}

// TestNewSopsProvider_TopLevelProviders proves the fallback to atmos.yaml's top-level
// `secrets.providers` when the name is absent from the stack/component section.
func TestNewSopsProvider_TopLevelProviders(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.Secrets.Providers = map[string]schema.SecretProviderConfig{
		"top": {
			Kind: "sops/age",
			Spec: map[string]any{"file": filepath.Join(t.TempDir(), "dev.enc.yaml")},
		},
	}
	p, err := New(cfg, "top", nil)
	require.NoError(t, err)
	assert.Equal(t, "sops/age", p.Kind())
}

// TestSopsErrorWrappers_SentinelsAndHints asserts each error-wrapper helper wraps the expected
// sentinel (errors.Is) AND attaches at least one actionable hint (so users get guidance).
func TestSopsErrorWrappers_SentinelsAndHints(t *testing.T) {
	t.Run("fileNotFoundErr", func(t *testing.T) {
		err := fileNotFoundErr("/some/dev.enc.yaml")
		require.ErrorIs(t, err, ErrSecretFileNotFound)
		assert.NotEmpty(t, cockroachErrors.GetAllHints(err), "must carry initialization hints")
	})

	t.Run("ageKeyFileErr", func(t *testing.T) {
		err := ageKeyFileErr("/keys.txt", os.ErrNotExist)
		require.ErrorIs(t, err, ErrSopsAgeKeyFile)
		assert.NotEmpty(t, cockroachErrors.GetAllHints(err), "must carry age-keygen hints")
	})

	t.Run("ageKeyErr", func(t *testing.T) {
		err := ageKeyErr(os.ErrInvalid)
		require.ErrorIs(t, err, ErrSopsAgeKey)
		assert.NotEmpty(t, cockroachErrors.GetAllHints(err), "must carry inline-key hints")
	})

	t.Run("decryptErr with key file mentions the path", func(t *testing.T) {
		p := &sopsProvider{ageKeyFile: "/my/keys.txt"}
		err := p.decryptErr("/dev.enc.yaml", nil, os.ErrInvalid)
		require.ErrorIs(t, err, ErrSopsDecrypt)
		hints := cockroachErrors.GetAllHints(err)
		require.NotEmpty(t, hints)
		joined := strings.Join(hints, "\n")
		assert.Contains(t, joined, "/my/keys.txt", "hint should reference the configured key file")
	})

	t.Run("decryptErr without key file suggests env vars", func(t *testing.T) {
		p := &sopsProvider{}
		err := p.decryptErr("/dev.enc.yaml", nil, os.ErrInvalid)
		require.ErrorIs(t, err, ErrSopsDecrypt)
		hints := cockroachErrors.GetAllHints(err)
		require.NotEmpty(t, hints)
		joined := strings.Join(hints, "\n")
		assert.Contains(t, joined, "SOPS_AGE_KEY", "hint should mention the env-var fallback")
	})
}
