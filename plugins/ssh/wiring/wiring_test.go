package wiring

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/sdk/api"
)

func TestRegisterAuthoritiesIsIdempotentAndRegistersGitHubAndSelf(t *testing.T) {
	RegisterAuthorities()
	RegisterAuthorities()

	assert.Contains(t, authority.Names(), "github")
	assert.Contains(t, authority.Names(), "self")
	_, err := authority.New("github", authority.Options{})
	assert.NoError(t, err)
}

func TestServicesBuildsAllThreeExports(t *testing.T) {
	services := Services(&api.Client{}, "0.1.0")

	assert.NotNil(t, services.Provider)
	assert.NotNil(t, services.Identity)
	assert.NotNil(t, services.Commands)
}

func TestFindSSHKeygenChecksTheKnownLocationsInOrder(t *testing.T) {
	tests := []struct {
		goos   string
		exists map[string]bool
		want   string
		err    error
	}{
		{"darwin", map[string]bool{"/usr/bin/ssh-keygen": true, "/opt/homebrew/bin/ssh-keygen": true}, "/usr/bin/ssh-keygen", nil},
		{"linux", map[string]bool{"/usr/local/bin/ssh-keygen": true}, "/usr/local/bin/ssh-keygen", nil},
		{"darwin", map[string]bool{"/opt/homebrew/bin/ssh-keygen": true}, "/opt/homebrew/bin/ssh-keygen", nil},
		{"windows", map[string]bool{`C:\Windows\System32\OpenSSH\ssh-keygen.exe`: true}, `C:\Windows\System32\OpenSSH\ssh-keygen.exe`, nil},
		{"windows", map[string]bool{"/usr/bin/ssh-keygen": true}, "", identity.ErrNoSSHKeygen},
		{"linux", nil, "", identity.ErrNoSSHKeygen},
	}
	for _, tt := range tests {
		got, err := findSSHKeygen(tt.goos, func(p string) bool { return tt.exists[p] })

		assert.Equal(t, tt.want, got, tt.goos)
		if tt.err != nil {
			assert.ErrorIs(t, err, tt.err)
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestSleepWaitsOrStopsWithTheContext(t *testing.T) {
	require.NoError(t, Sleep(context.Background(), time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Sleep(ctx, time.Hour)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestResolvePathsUsesTheRootWhenGiven(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	p, err := resolvePaths(root)
	require.NoError(t, err)
	assert.Equal(t, root, p.Root())

	d, err := resolvePaths("")
	require.NoError(t, err)
	assert.NotEmpty(t, d.Root())
}

func TestFileExistsMeansARegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	assert.True(t, fileExists(file))
	assert.False(t, fileExists(dir))
	assert.False(t, fileExists(filepath.Join(dir, "missing")))
}
