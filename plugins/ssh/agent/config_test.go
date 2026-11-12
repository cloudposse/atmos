package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ident(name string, allow ...Allow) Identity {
	return Identity{Name: name, Store: "file", Key: name, Allow: allow}
}

func TestIdentityValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      Identity
		wantErr bool
	}{
		{"signing", ident("ops", AllowSigning), false},
		{"both", ident("ops", AllowSigning, AllowAuthentication), false},
		{"empty name", ident("", AllowSigning), true},
		{"unsafe name", ident("../x", AllowSigning), true},
		{"no store", Identity{Name: "ops", Key: "ops", Allow: []Allow{AllowSigning}}, true},
		{"no key", Identity{Name: "ops", Store: "file", Allow: []Allow{AllowSigning}}, true},
		{"no allow", ident("ops"), true},
		{"unknown allow", ident("ops", Allow("root")), true},
		{"duplicate allow", ident("ops", AllowSigning, AllowSigning), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidConfig)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConfigValidate(t *testing.T) {
	require.ErrorIs(t, (&Config{Version: 2}).Validate(), ErrInvalidConfig)
	dup := &Config{Version: 1, Identities: []Identity{ident("a", AllowSigning), ident("a", AllowSigning)}}
	require.ErrorIs(t, dup.Validate(), ErrInvalidConfig)
	require.NoError(t, (&Config{Version: 1}).Validate())
}

func TestLoadConfigMissing(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "agent.json"))
	require.ErrorIs(t, err, ErrConfigNotFound)
}

func TestLoadConfigRejectsBadFiles(t *testing.T) {
	tests := map[string]string{
		"not json":      `{`,
		"unknown field": `{"version":1,"identities":[],"extra":true}`,
		"wrong version": `{"version":9,"identities":[]}`,
		"bad identity":  `{"version":1,"identities":[{"name":"a","store":"file","key":"a","allow":["root"]}]}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.json")
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
			_, err := LoadConfig(path)
			require.ErrorIs(t, err, ErrInvalidConfig)
		})
	}
}

func TestUpsertAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "agent.json")

	require.NoError(t, UpsertIdentity(path, ident("zed", AllowSigning)))
	require.NoError(t, UpsertIdentity(path, ident("alpha", AllowAuthentication)))
	require.NoError(t, UpsertIdentity(path, ident("zed", AllowSigning, AllowAuthentication)))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Identities, 2)
	assert.Equal(t, "alpha", cfg.Identities[0].Name, "sorted by name")
	assert.Equal(t, []Allow{AllowAuthentication}, cfg.Identities[0].Allow)
	assert.Equal(t, "zed", cfg.Identities[1].Name)
	assert.Equal(t, []Allow{AllowSigning, AllowAuthentication}, cfg.Identities[1].Allow, "upsert replaced the entry")

	removed, err := RemoveIdentity(path, "alpha")
	require.NoError(t, err)
	assert.True(t, removed)
	removed, err = RemoveIdentity(path, "alpha")
	require.NoError(t, err)
	assert.False(t, removed)

	cfg, err = LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Identities, 1)
	assert.Equal(t, "zed", cfg.Identities[0].Name)
}

func TestRemoveFromMissingFile(t *testing.T) {
	removed, err := RemoveIdentity(filepath.Join(t.TempDir(), "agent.json"), "a")
	require.NoError(t, err)
	assert.False(t, removed)
}

func TestUpsertRejectsInvalidAndKeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	require.NoError(t, UpsertIdentity(path, ident("a", AllowSigning)))
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	require.ErrorIs(t, UpsertIdentity(path, ident("b")), ErrInvalidConfig)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestSaveConfigMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	require.NoError(t, UpsertIdentity(path, ident("a", AllowSigning)))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary files left behind")
}

func TestSaveConfigEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	require.NoError(t, SaveConfig(path, &Config{Version: ConfigVersion}))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"identities": []`)
}
