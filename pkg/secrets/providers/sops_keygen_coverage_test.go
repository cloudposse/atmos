package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// TestHasKey_AgeKeyStore covers HasKey's `ageKeyStore` branch: a StatusStore reports key
// present/absent, and a non-StatusStore store reports false (so keygen runs).
func TestHasKey_AgeKeyStore(t *testing.T) {
	t.Run("StatusStore key present", func(t *testing.T) {
		cfg := newKeychainBackedConfig(t)
		identity, err := age.GenerateX25519Identity()
		require.NoError(t, err)
		// Seed the key at the provider-owned triple (stack=vault, component=age-key, key=vault).
		require.NoError(t, cfg.Stores["keychain"].Set("dev-sops", ageKeyStoreComponent, "dev-sops", identity.String()))

		p := &sopsProvider{name: "dev-sops", kind: "sops/age", ageKeyStore: "keychain", stores: cfg.Stores}
		assert.True(t, p.HasKey(), "StatusStore holding the key reports present")
	})

	t.Run("StatusStore key absent", func(t *testing.T) {
		cfg := newKeychainBackedConfig(t)
		p := &sopsProvider{name: "dev-sops", kind: "sops/age", ageKeyStore: "keychain", stores: cfg.Stores}
		assert.False(t, p.HasKey(), "empty StatusStore reports absent")
	})

	t.Run("non-StatusStore reports false", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		// A plain MockStore does not implement StatusStore, so storeHasKey is best-effort false.
		p := &sopsProvider{name: "dev-sops", kind: "sops/age", ageKeyStore: "app", stores: store.StoreRegistry{"app": store.NewMockStore(ctrl)}}
		assert.False(t, p.HasKey())
	})

	t.Run("store missing reports false", func(t *testing.T) {
		p := &sopsProvider{name: "dev-sops", kind: "sops/age", ageKeyStore: "nope", stores: store.StoreRegistry{}}
		assert.False(t, p.HasKey())
	})
}

// TestHasKey_NonAgeKind covers the early-return where non-age SOPS kinds always report a key
// present (externally managed: KMS/GPG).
func TestHasKey_NonAgeKind(t *testing.T) {
	for _, kind := range []string{"sops/aws-kms", "sops/gcp-kms", "sops/pgp"} {
		p := &sopsProvider{name: "kms", kind: kind}
		assert.Truef(t, p.HasKey(), "kind %q is externally managed", kind)
	}
}

// TestHasKey_InlineKey covers the `ageKey != ""` short-circuit.
func TestHasKey_InlineKey(t *testing.T) {
	p := &sopsProvider{name: "sops", kind: "sops/age", ageKey: "AGE-SECRET-KEY-1AAA"}
	assert.True(t, p.HasKey())
}

// TestHasKey_AgeKeyFile covers the explicit `age_key_file` stat branch (present and absent).
func TestHasKey_AgeKeyFile(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "absent-default.txt"))

	t.Run("present", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "keys.txt")
		require.NoError(t, os.WriteFile(path, []byte("AGE-SECRET-KEY-1AAA\n"), 0o600))
		p := &sopsProvider{name: "sops", kind: "sops/age", ageKeyFile: path}
		assert.True(t, p.HasKey())
	})

	t.Run("absent", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.txt")
		p := &sopsProvider{name: "sops", kind: "sops/age", ageKeyFile: path}
		assert.False(t, p.HasKey())
	})

	t.Run("expand error yields false", func(t *testing.T) {
		// "~otheruser" cannot be expanded; expandKeyPath errors, so HasKey skips this branch.
		p := &sopsProvider{name: "sops", kind: "sops/age", ageKeyFile: "~nonexistent-user-xyz/keys.txt"}
		assert.False(t, p.HasKey())
	})
}

// TestHasKey_DefaultKeysFile covers the default-keys-file stat (via SOPS_AGE_KEY_FILE) when no
// explicit sink is configured: present => true, absent => false.
func TestHasKey_DefaultKeysFile(t *testing.T) {
	t.Run("default present", func(t *testing.T) {
		def := filepath.Join(t.TempDir(), "default-keys.txt")
		require.NoError(t, os.WriteFile(def, []byte("AGE-SECRET-KEY-1AAA\n"), 0o600))
		t.Setenv("SOPS_AGE_KEY_FILE", def)
		p := &sopsProvider{name: "sops", kind: "sops/age"}
		assert.True(t, p.HasKey())
	})

	t.Run("default absent", func(t *testing.T) {
		t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "missing-default.txt"))
		p := &sopsProvider{name: "sops", kind: "sops/age"}
		assert.False(t, p.HasKey())
	})
}

// TestWritePrivateIdentity_StorePath covers writePrivateIdentity's store branch: the identity is
// persisted to the configured store at the provider-owned triple and the returned location names it.
func TestWritePrivateIdentity_StorePath(t *testing.T) {
	cfg := newKeychainBackedConfig(t)
	p := &sopsProvider{name: "dev-sops", kind: "sops/age", ageKeyStore: "keychain", stores: cfg.Stores}

	loc, err := p.writePrivateIdentity("AGE-SECRET-KEY-1ZZZ")
	require.NoError(t, err)
	assert.Contains(t, loc, "keychain")

	got, err := cfg.Stores["keychain"].Get("dev-sops", ageKeyStoreComponent, "dev-sops")
	require.NoError(t, err)
	assert.Equal(t, "AGE-SECRET-KEY-1ZZZ", got)
}

// TestWritePrivateIdentity_StoreNotFound covers the store-write error path: an unconfigured store
// surfaces ErrSopsAgeKey.
func TestWritePrivateIdentity_StoreNotFound(t *testing.T) {
	p := &sopsProvider{name: "dev-sops", kind: "sops/age", ageKeyStore: "missing", stores: store.StoreRegistry{}}
	_, err := p.writePrivateIdentity("AGE-SECRET-KEY-1ZZZ")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsAgeKey)
}

// TestWritePrivateIdentity_KeyFilePath covers writePrivateIdentity's file branch via an explicit
// age_key_file: the identity is appended to that file and the path is returned.
func TestWritePrivateIdentity_KeyFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "keys.txt")
	p := &sopsProvider{name: "sops", kind: "sops/age", ageKeyFile: path}

	loc, err := p.writePrivateIdentity("AGE-SECRET-KEY-1AAA")
	require.NoError(t, err)
	assert.Equal(t, path, loc)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "AGE-SECRET-KEY-1AAA\n", string(data))
}

// TestPrivateKeySink covers privateKeySink: explicit age_key_file (expanded), the default keys
// file, and the expand-error path.
func TestPrivateKeySink(t *testing.T) {
	t.Run("explicit age_key_file expanded", func(t *testing.T) {
		t.Setenv("MY_KEYS_DIR", t.TempDir())
		p := &sopsProvider{name: "sops", kind: "sops/age", ageKeyFile: filepath.Join("$MY_KEYS_DIR", "keys.txt")}
		got, err := p.privateKeySink()
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(os.Getenv("MY_KEYS_DIR"), "keys.txt"), got)
	})

	t.Run("default when unset", func(t *testing.T) {
		def := filepath.Join(t.TempDir(), "sops-keys.txt")
		t.Setenv("SOPS_AGE_KEY_FILE", def)
		p := &sopsProvider{name: "sops", kind: "sops/age"}
		got, err := p.privateKeySink()
		require.NoError(t, err)
		assert.Equal(t, def, got)
	})

	t.Run("expand error", func(t *testing.T) {
		p := &sopsProvider{name: "sops", kind: "sops/age", ageKeyFile: "~nonexistent-user-xyz/keys.txt"}
		_, err := p.privateKeySink()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSopsAgeKeyFile)
	})
}

// TestRecipientSinkPath covers recipientSinkPath: absolute recipients_file passes through, relative
// joins basePath, and the default falls back to `.sops.yaml` at basePath.
func TestRecipientSinkPath(t *testing.T) {
	base := t.TempDir()

	t.Run("explicit absolute", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "custom.sops.yaml")
		p := &sopsProvider{name: "sops", kind: "sops/age", recipientsFile: abs}
		assert.Equal(t, abs, p.recipientSinkPath(base))
	})

	t.Run("explicit relative joins basePath", func(t *testing.T) {
		p := &sopsProvider{name: "sops", kind: "sops/age", recipientsFile: filepath.Join("conf", "custom.yaml")}
		assert.Equal(t, filepath.Join(base, "conf", "custom.yaml"), p.recipientSinkPath(base))
	})

	t.Run("default .sops.yaml at basePath", func(t *testing.T) {
		p := &sopsProvider{name: "sops", kind: "sops/age"}
		assert.Equal(t, filepath.Join(base, defaultSopsConfigName), p.recipientSinkPath(base))
	})
}

// TestWriteRecipient_ReadError covers writeRecipient's non-NotExist ReadFile error: the sink path is
// a directory, so ReadFile fails with something other than os.IsNotExist.
func TestWriteRecipient_ReadError(t *testing.T) {
	dir := t.TempDir() // an existing directory; ReadFile on it is not a NotExist error.
	p := &sopsProvider{name: "sops", kind: "sops/age", file: "secrets/x.enc.yaml"}

	err := p.writeRecipient(dir, "age1aaa")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestWriteRecipient_UpsertError covers the upsert-error path: malformed existing YAML at the sink
// makes upsertSopsCreationRule fail before any write.
func TestWriteRecipient_UpsertError(t *testing.T) {
	sink := filepath.Join(t.TempDir(), ".sops.yaml")
	require.NoError(t, os.WriteFile(sink, []byte("creation_rules: not-a-list\n"), 0o644))
	p := &sopsProvider{name: "sops", kind: "sops/age", file: "secrets/x.enc.yaml"}

	err := p.writeRecipient(sink, "age1aaa")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestWriteRecipient_MkdirError covers the MkdirAll failure: a parent path component is a regular
// file, so creating the sink's directory fails. Unix permission semantics are not needed here, so
// this runs on all platforms.
func TestWriteRecipient_MkdirError(t *testing.T) {
	root := t.TempDir()
	// Make `root/blocker` a file; then the sink dir `root/blocker/sub` cannot be created.
	blocker := filepath.Join(root, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	sink := filepath.Join(blocker, "sub", ".sops.yaml")
	p := &sopsProvider{name: "sops", kind: "sops/age", file: "secrets/x.enc.yaml"}

	err := p.writeRecipient(sink, "age1aaa")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestWriteRecipient_WriteError covers the WriteFile failure via a read-only target directory
// (Unix-only: Windows ignores the directory write bit for this case).
func TestWriteRecipient_WriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500)) // r-x: dir exists (MkdirAll is a no-op) but WriteFile fails.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	sink := filepath.Join(dir, ".sops.yaml")
	p := &sopsProvider{name: "sops", kind: "sops/age", file: "secrets/x.enc.yaml"}

	err := p.writeRecipient(sink, "age1aaa")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestWriteRecipient_Success covers the happy path: a fresh sink gets a creation rule, and a second
// call with the same recipient is idempotent.
func TestWriteRecipient_Success(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "nested", ".sops.yaml")
	p := &sopsProvider{name: "sops", kind: "sops/age", file: filepath.Join("secrets", "{{ .atmos_stack }}.enc.yaml")}

	require.NoError(t, p.writeRecipient(sink, "age1aaa"))
	data, err := os.ReadFile(sink)
	require.NoError(t, err)
	assert.Contains(t, string(data), "age1aaa")
	assert.Contains(t, string(data), "creation_rules:")

	require.NoError(t, p.writeRecipient(sink, "age1aaa"))
	data2, err := os.ReadFile(sink)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(data2), "re-adding the same recipient is idempotent")
}

// TestGenerateKey_StoreWritePath covers GenerateKey when the private sink is a store: the identity
// lands in the store and the recipient in .sops.yaml.
func TestGenerateKey_StoreWritePath(t *testing.T) {
	cfg := newKeychainBackedConfig(t)
	base := t.TempDir()
	p := &sopsProvider{
		name:           "dev-sops",
		kind:           "sops/age",
		file:           filepath.Join("secrets", "{{ .atmos_stack }}.enc.yaml"),
		ageKeyStore:    "keychain",
		recipientsFile: ".sops.yaml",
		stores:         cfg.Stores,
	}

	res, err := p.GenerateKey(base)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(res.Public, "age1"))

	priv := keygenOutputLocation(t, res, "private identity")
	assert.Contains(t, priv, "keychain")

	stored, err := cfg.Stores["keychain"].Get("dev-sops", ageKeyStoreComponent, "dev-sops")
	require.NoError(t, err)
	assert.Equal(t, res.Public, recipientFromStoredKey(t, stored.(string)))

	sb, err := os.ReadFile(filepath.Join(base, ".sops.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(sb), res.Public)
}

// recipientFromStoredKey derives the public recipient from a stored age private identity so we can
// assert the store holds the very key whose recipient keygen reported.
func recipientFromStoredKey(t *testing.T, priv string) string {
	t.Helper()
	id, err := age.ParseX25519Identity(strings.TrimSpace(priv))
	require.NoError(t, err)
	return id.Recipient().String()
}

// TestGenerateKey_PrivateSinkError covers GenerateKey's error propagation from writePrivateIdentity
// when the configured store is missing.
func TestGenerateKey_PrivateSinkError(t *testing.T) {
	p := &sopsProvider{
		name:        "dev-sops",
		kind:        "sops/age",
		file:        "secrets/x.enc.yaml",
		ageKeyStore: "missing",
		stores:      store.StoreRegistry{},
	}
	_, err := p.GenerateKey(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsAgeKey)
}

// TestGenerateKey_RecipientSinkError covers GenerateKey's error propagation from writeRecipient: the
// recipients file already contains malformed YAML (a non-list creation_rules).
func TestGenerateKey_RecipientSinkError(t *testing.T) {
	base := t.TempDir()
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(base, "keys.txt"))
	require.NoError(t, os.WriteFile(filepath.Join(base, ".sops.yaml"), []byte("creation_rules: oops\n"), 0o644))

	p := &sopsProvider{
		name:           "sops",
		kind:           "sops/age",
		file:           "secrets/x.enc.yaml",
		recipientsFile: ".sops.yaml",
	}
	_, err := p.GenerateKey(base)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestDefaultSopsAgeKeysFile covers defaultSopsAgeKeysFile: SOPS_AGE_KEY_FILE override wins, and
// the config-dir fallback (driven via XDG_CONFIG_HOME on darwin) yields `<dir>/sops/age/keys.txt`.
func TestDefaultSopsAgeKeysFile(t *testing.T) {
	t.Run("SOPS_AGE_KEY_FILE override", func(t *testing.T) {
		want := filepath.Join(t.TempDir(), "explicit-keys.txt")
		t.Setenv("SOPS_AGE_KEY_FILE", want)
		got, err := defaultSopsAgeKeysFile()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("config-dir fallback", func(t *testing.T) {
		t.Setenv("SOPS_AGE_KEY_FILE", "")
		// sopsUserConfigDir honors XDG_CONFIG_HOME on darwin; on other platforms os.UserConfigDir
		// reads it too, so this drives the fallback path on every OS we test on.
		cfgDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", cfgDir)
		got, err := defaultSopsAgeKeysFile()
		require.NoError(t, err)
		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
			assert.Equal(t, filepath.Join(cfgDir, "sops", "age", "keys.txt"), got)
		}
		// On all platforms the path ends with the sops keys-file suffix.
		assert.True(t, strings.HasSuffix(filepath.ToSlash(got), "sops/age/keys.txt"), "got %q", got)
	})
}

// TestSopsUserConfigDir covers sopsUserConfigDir: XDG_CONFIG_HOME is honored on darwin, and the
// os.UserConfigDir fallback is exercised on every platform.
func TestSopsUserConfigDir(t *testing.T) {
	t.Run("XDG override", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		got, err := sopsUserConfigDir()
		require.NoError(t, err)
		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
			assert.Equal(t, dir, got)
		} else {
			// On Windows os.UserConfigDir ignores XDG_CONFIG_HOME; just assert it resolves.
			assert.NotEmpty(t, got)
		}
	})

	t.Run("fallback resolves", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		got, err := sopsUserConfigDir()
		// os.UserConfigDir can legitimately error in a stripped env; tolerate that but assert the
		// success case yields a non-empty dir.
		if err == nil {
			assert.NotEmpty(t, got)
		}
	})
}

// TestIdentityPresent covers identityPresent: present, absent, and partial (substring) non-matches.
func TestIdentityPresent(t *testing.T) {
	content := "AGE-SECRET-KEY-1AAA\n# a comment\nAGE-SECRET-KEY-1BBB\n"
	tests := []struct {
		name     string
		identity string
		want     bool
	}{
		{"present first", "AGE-SECRET-KEY-1AAA", true},
		{"present last", "AGE-SECRET-KEY-1BBB", true},
		{"present with surrounding whitespace lines", "AGE-SECRET-KEY-1AAA", true},
		{"absent", "AGE-SECRET-KEY-1CCC", false},
		{"partial substring is not a match", "AGE-SECRET-KEY-1A", false},
		{"empty content", "AGE-SECRET-KEY-1AAA", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := content
			if tt.name == "empty content" {
				c = ""
			}
			assert.Equal(t, tt.want, identityPresent(c, tt.identity))
		})
	}
}

// TestLoadSopsCreationRules_Malformed covers loadSopsCreationRules' parse-error path: invalid YAML.
func TestLoadSopsCreationRules_Malformed(t *testing.T) {
	_, _, err := loadSopsCreationRules([]byte("creation_rules: [unterminated\n"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestLoadSopsCreationRules_MissingKey covers the branch that creates a fresh `creation_rules`
// sequence when the document has none.
func TestLoadSopsCreationRules_MissingKey(t *testing.T) {
	root, rules, err := loadSopsCreationRules([]byte("other: keepme\n"))
	require.NoError(t, err)
	require.NotNil(t, root)
	require.NotNil(t, rules)
	assert.Empty(t, rules.Content, "freshly created creation_rules has no entries")
}

// TestLoadSopsCreationRules_NotASequence covers the branch where `creation_rules` exists but is not
// a list.
func TestLoadSopsCreationRules_NotASequence(t *testing.T) {
	_, _, err := loadSopsCreationRules([]byte("creation_rules: not-a-list\n"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestUpsertSopsCreationRule_MalformedInput covers upsertSopsCreationRule's error propagation from
// loadSopsCreationRules.
func TestUpsertSopsCreationRule_MalformedInput(t *testing.T) {
	_, err := upsertSopsCreationRule([]byte("creation_rules: not-a-list\n"), `x/.*$`, "age1aaa")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// TestAddRecipientToRule covers addRecipientToRule: creating a missing `age` field, appending to an
// existing list, no-op when already present, and replacing an empty `age` value.
func TestAddRecipientToRule(t *testing.T) {
	t.Run("creates missing age field", func(t *testing.T) {
		_, rules, err := loadSopsCreationRules([]byte("creation_rules:\n  - path_regex: x/.*$\n"))
		require.NoError(t, err)
		rule := findCreationRule(rules, "x/.*$")
		require.NotNil(t, rule)
		addRecipientToRule(rule, "age1aaa")
		assert.Equal(t, "age1aaa", mappingValue(rule, "age").Value)
	})

	t.Run("appends to existing list", func(t *testing.T) {
		_, rules, err := loadSopsCreationRules([]byte("creation_rules:\n  - path_regex: x/.*$\n    age: age1aaa\n"))
		require.NoError(t, err)
		rule := findCreationRule(rules, "x/.*$")
		require.NotNil(t, rule)
		addRecipientToRule(rule, "age1bbb")
		assert.Equal(t, "age1aaa,age1bbb", mappingValue(rule, "age").Value)
	})

	t.Run("idempotent when present", func(t *testing.T) {
		_, rules, err := loadSopsCreationRules([]byte("creation_rules:\n  - path_regex: x/.*$\n    age: age1aaa,age1bbb\n"))
		require.NoError(t, err)
		rule := findCreationRule(rules, "x/.*$")
		require.NotNil(t, rule)
		addRecipientToRule(rule, "age1bbb")
		assert.Equal(t, "age1aaa,age1bbb", mappingValue(rule, "age").Value)
	})

	t.Run("replaces empty age value", func(t *testing.T) {
		_, rules, err := loadSopsCreationRules([]byte("creation_rules:\n  - path_regex: x/.*$\n    age: \"\"\n"))
		require.NoError(t, err)
		rule := findCreationRule(rules, "x/.*$")
		require.NotNil(t, rule)
		addRecipientToRule(rule, "age1aaa")
		assert.Equal(t, "age1aaa", mappingValue(rule, "age").Value)
	})
}

// Compile-time sentinels: a rename of these schema/store identifiers should fail the build here.
var (
	_ = ageKeyStoreComponent
	_ = schema.AtmosConfiguration{}
	_ = store.StoreRegistry{}
)
