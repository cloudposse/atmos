package azure

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// isolateHome points every home-directory resolver used by the Azure helpers at a temp dir.
func isolateHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	homedir.Reset()
	homedir.DisableCache = true
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})
	return tmpHome
}

// TestUserProviders_PathsAreRealmIsolated verifies that the credential file the
// device-code and interactive providers report is scoped by realm and provider, so
// two repositories (realms) or two providers never share one credentials.json.
func TestUserProviders_PathsAreRealmIsolated(t *testing.T) {
	tmpHome := isolateHome(t)

	newProviders := func(t *testing.T, name string) map[string]interface {
		SetRealm(string)
		Paths() ([]authTypes.Path, error)
	} {
		t.Helper()
		dc, err := NewDeviceCodeProvider(name, &schema.Provider{
			Kind: "azure/device-code",
			Spec: map[string]interface{}{"tenant_id": "tenant-123"},
		})
		require.NoError(t, err)
		ip, err := NewInteractiveProvider(name, &schema.Provider{
			Kind: authTypes.ProviderKindAzureInteractive,
			Spec: map[string]interface{}{"tenant_id": "tenant-123"},
		})
		require.NoError(t, err)
		return map[string]interface {
			SetRealm(string)
			Paths() ([]authTypes.Path, error)
		}{"device-code": dc, "interactive": ip}
	}

	tests := []struct {
		name     string
		realm    string
		provider string
		want     string
	}{
		{name: "realm A", realm: "realm-a", provider: "azure-user", want: filepath.Join(tmpHome, ".azure", "atmos", "realm-a", "azure-user", "credentials.json")},
		{name: "realm B", realm: "realm-b", provider: "azure-user", want: filepath.Join(tmpHome, ".azure", "atmos", "realm-b", "azure-user", "credentials.json")},
		{name: "other provider", realm: "realm-a", provider: "azure-other", want: filepath.Join(tmpHome, ".azure", "atmos", "realm-a", "azure-other", "credentials.json")},
		{name: "no realm", realm: "", provider: "azure-user", want: filepath.Join(tmpHome, ".azure", "atmos", "azure-user", "credentials.json")},
	}

	for _, tt := range tests {
		for kind, p := range newProviders(t, tt.provider) {
			t.Run(tt.name+"/"+kind, func(t *testing.T) {
				p.SetRealm(tt.realm)
				paths, err := p.Paths()
				require.NoError(t, err)
				require.Len(t, paths, 1)
				assert.Equal(t, tt.want, paths[0].Location)
				assert.Equal(t, authTypes.PathTypeFile, paths[0].Type)
				assert.True(t, paths[0].Required)
			})
		}
	}
}
