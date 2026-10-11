package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfig_ImportedIdentities exercises the final config, including the
// identity reconstruction that used to discard the correctly merged imports.
func TestLoadConfig_ImportedIdentities(t *testing.T) {
	for _, source := range []string{"explicit", "atmos.d", ".atmos.d"} {
		t.Run(source, func(t *testing.T) {
			setupTestAdapters()
			dir := t.TempDir()
			t.Setenv("TEST_GIT_ROOT", dir)
			t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(dir, "cache"))
			t.Setenv("AUTH_IMPORT_ENDPOINT", "https://example.test")
			importPath := "shared.yaml"
			main := "base_path: ./\n"
			if source == "explicit" {
				main += "import: [./shared.yaml]\n"
			} else {
				importPath = filepath.Join(source, importPath)
			}
			writeIdentityImportFixture(t, dir, importPath, `env:
  Imported_ENV: imported
templates:
  settings:
    env:
      Template_ENV: template
auth:
  providers:
    shared/sso:
      kind: aws/iam-identity-center
  identities:
    Shared.Reader/Access:
      kind: aws/permission-set
      via:
        provider: shared/sso
      default: true
      principal:
        name: ReadOnlyAccess
        account:
          id: "222222222222"
          name: shared
      spec:
        endpoint_url: !env AUTH_IMPORT_ENDPOINT
    Imported.Only/Reader:
      kind: aws/permission-set
      via:
        provider: shared/sso
`)
			main += `auth:
  providers:
    main/sso:
      kind: aws/iam-identity-center
  identities:
    shared.reader/Access:
      default: false
      principal:
        account:
          name: overridden
    main/admin:
      kind: aws/permission-set
      via:
        provider: main/sso
`
			writeIdentityImportFixture(t, dir, "atmos.yaml", main)
			cfg, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
			require.NoError(t, err)
			require.Len(t, cfg.Auth.Identities, 3)
			assert.Contains(t, cfg.Auth.Identities, "main/admin")
			assert.Contains(t, cfg.Auth.Identities, "imported.only/reader")
			identity := cfg.Auth.Identities["shared.reader/access"]
			assert.Equal(t, "aws/permission-set", identity.Kind)
			require.NotNil(t, identity.Via)
			assert.Equal(t, "shared/sso", identity.Via.Provider)
			assert.False(t, identity.Default)
			assert.Equal(t, "ReadOnlyAccess", identity.Principal["name"])
			assert.Equal(t, map[string]any{"id": "222222222222", "name": "overridden"}, identity.Principal["account"])
			assert.Equal(t, "https://example.test", identity.Spec["endpoint_url"])
			assert.Equal(t, "shared.reader/Access", cfg.Auth.IdentityCaseMap["shared.reader/access"])
			assert.Equal(t, "Imported.Only/Reader", cfg.Auth.IdentityCaseMap["imported.only/reader"])
			assert.Equal(t, "imported", cfg.Env["Imported_ENV"])
			assert.Equal(t, "template", cfg.Templates.Settings.Env["Template_ENV"])
			assert.Contains(t, cfg.Auth.Providers, "main/sso")
			assert.Contains(t, cfg.Auth.Providers, "shared/sso")
		})
	}
}

// writeIdentityImportFixture writes an isolated configuration source and returns its path.
func writeIdentityImportFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestLoadConfig_ImportedIdentitiesHierarchy checks nested imports and profile overrides in the final identities.
func TestLoadConfig_ImportedIdentitiesHierarchy(t *testing.T) {
	setupTestAdapters()
	dir := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", dir)
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	writeIdentityImportFixture(t, dir, "nested.yaml", `auth:
  identities:
    Nested.Reader:
      kind: aws/permission-set
      principal:
        name: nested
`)
	writeIdentityImportFixture(t, dir, "shared.yaml", `import: [./nested.yaml]
auth:
  identities:
    Shared.Reader:
      kind: aws/permission-set
`)
	writeIdentityImportFixture(t, dir, "atmos.yaml", `base_path: ./
import: [./shared.yaml]
profiles:
  base_path: ./profiles
auth:
  identities:
    Main.Admin:
      kind: aws/permission-set
    Nested.Reader:
      principal:
        name: main
`)
	writeIdentityImportFixture(t, dir, "profiles/dev/auth.yaml", `auth:
  identities:
    Nested.Reader:
      default: true
    Profile.Reader:
      kind: aws/permission-set
`)
	cfg, err := LoadConfig(&schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{dir}, ProfilesFromArg: []string{"dev"},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"nested.reader", "shared.reader", "main.admin", "profile.reader"}, identityNames(&cfg))
	assert.Equal(t, "aws/permission-set", cfg.Auth.Identities["nested.reader"].Kind)
	assert.Equal(t, "main", cfg.Auth.Identities["nested.reader"].Principal["name"])
	assert.True(t, cfg.Auth.Identities["nested.reader"].Default)
}

// TestLoadConfig_ImportedIdentitiesRepeatedSources checks repeated merge precedence and deduplicated file reporting.
func TestLoadConfig_ImportedIdentitiesRepeatedSources(t *testing.T) {
	setupTestAdapters()
	dir := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", dir)
	shared := writeIdentityImportFixture(t, dir, "shared.yaml", `auth:
  identities:
    Shared.Reader:
      kind: aws/permission-set
      default: false
`)
	first := writeIdentityImportFixture(t, dir, "first.yaml", `base_path: ./
import: [./shared.yaml]
auth:
  identities:
    First.Admin:
      kind: aws/permission-set
`)
	second := writeIdentityImportFixture(t, dir, "second.yaml", `auth:
  identities:
    Shared.Reader:
      default: true
    Second.Admin:
      kind: aws/permission-set
`)
	// The explicit file-loading path processes its imports after each file.
	// Reapplying shared.yaml must win over second.yaml, just as it does in Viper.
	cfg, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{first, second}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"shared.reader", "first.admin", "second.admin"}, identityNames(&cfg))
	assert.False(t, cfg.Auth.Identities["shared.reader"].Default)
	assert.Equal(t, []string{first, shared, second}, LoadedConfigFiles())
}

// TestLoadConfig_ImportedIdentitiesProvisioned checks partial overrides of cached provisioned identities.
func TestLoadConfig_ImportedIdentitiesProvisioned(t *testing.T) {
	setupTestAdapters()
	dir := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", dir)
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	writeIdentityImportFixture(t, dir, "cache/atmos/auth/sso/provisioned-identities.yaml", `auth:
  identities:
    Account.Prod/Admin:
      kind: aws/permission-set
      via:
        provider: sso
      principal:
        name: AdministratorAccess
    Account.Prod/Reader:
      kind: aws/permission-set
      via:
        provider: sso
`)
	writeIdentityImportFixture(t, dir, "atmos.yaml", `base_path: ./
auth:
  providers:
    sso:
      kind: aws/iam-identity-center
      auto_provision_identities: true
  identities:
    Account.Prod/Admin:
      default: true
`)
	cfg, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"account.prod/admin", "account.prod/reader"}, identityNames(&cfg))
	admin := cfg.Auth.Identities["account.prod/admin"]
	assert.True(t, admin.Default)
	assert.Equal(t, "aws/permission-set", admin.Kind)
	require.NotNil(t, admin.Via)
	assert.Equal(t, "sso", admin.Via.Provider)
	assert.Equal(t, "AdministratorAccess", admin.Principal["name"])
	assert.Equal(t, "Account.Prod/Admin", cfg.Auth.IdentityCaseMap["account.prod/admin"])
}

// TestLoadConfig_ImportedIdentitiesTrackerCleanup checks tracker cleanup on successful and failed loads.
func TestLoadConfig_ImportedIdentitiesTrackerCleanup(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid=%t", invalid), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TEST_GIT_ROOT", dir)
			writeIdentityImportFixture(t, dir, "atmos.yaml", "base_path: ./\n")
			content := "auth:\n  identities:\n    Reader: {kind: mock}\n"
			if invalid {
				content = "invalid: ["
			}
			writeIdentityImportFixture(t, dir, ".atmos.d/auth.yaml", content)
			mergedFilesReg.mu.Lock()
			before := len(mergedFilesReg.trackers)
			mergedFilesReg.mu.Unlock()
			_, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
			if invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			mergedFilesReg.mu.Lock()
			after := len(mergedFilesReg.trackers)
			mergedFilesReg.mu.Unlock()
			assert.Equal(t, before, after, "the load and its temporary Viper must both be released")
		})
	}
}

// TestLoadConfig_ImportedIdentitiesConcurrent checks that concurrent loads retain only their own imported identities.
func TestLoadConfig_ImportedIdentitiesConcurrent(t *testing.T) {
	setupTestAdapters()
	root := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", root)
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(root, "cache"))
	var dirs []string
	for _, name := range []string{"Alpha", "Beta"} {
		dir := filepath.Join(root, name)
		writeIdentityImportFixture(t, dir, "atmos.yaml", "base_path: ./\nimport: [./shared.yaml]\nauth:\n  identities:\n    Main: {kind: mock}\n")
		writeIdentityImportFixture(t, dir, "shared.yaml", fmt.Sprintf("auth:\n  identities:\n    %s.Reader: {kind: mock}\nenv:\n  %s_ENV: value\n", name, name))
		dirs = append(dirs, dir)
	}
	var wg sync.WaitGroup
	for range 10 {
		for _, dir := range dirs {
			wg.Go(func() {
				cfg, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
				if !assert.NoError(t, err) {
					return
				}
				name := filepath.Base(dir)
				assert.ElementsMatch(t, []string{"main", strings.ToLower(name) + ".reader"}, identityNames(&cfg))
				assert.Equal(t, name+".Reader", cfg.Auth.IdentityCaseMap[strings.ToLower(name)+".reader"])
				assert.Equal(t, map[string]string{name + "_ENV": "value"}, cfg.Env)
			})
		}
	}
	wg.Wait()
}

// identityNames collects identity lookup keys for order-independent assertions.
func identityNames(cfg *schema.AtmosConfiguration) []string {
	names := make([]string, 0, len(cfg.Auth.Identities))
	for name := range cfg.Auth.Identities {
		names = append(names, name)
	}
	return names
}

// Imported identities must also work when the main file defines no identities.
func TestLoadConfig_ImportedIdentitiesWithoutMainIdentities(t *testing.T) {
	setupTestAdapters()
	dir := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", dir)
	writeIdentityImportFixture(t, dir, "atmos.yaml", "base_path: ./\nimport: [./shared.yaml]\n")
	writeIdentityImportFixture(t, dir, "shared.yaml", "auth:\n  identities:\n    Imported.Reader: {kind: mock}\n")
	cfg, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{dir}})
	require.NoError(t, err)
	assert.Equal(t, []string{"imported.reader"}, identityNames(&cfg))
	assert.Equal(t, "mock", cfg.Auth.Identities["imported.reader"].Kind)
	assert.Equal(t, "Imported.Reader", cfg.Auth.IdentityCaseMap["imported.reader"])
}
