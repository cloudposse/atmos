package sops

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

func TestPossibleLocations(t *testing.T) {
	const selector = "!aws.cloudformation.output producer dev Path"

	tests := []struct {
		name string
		spec map[string]any
		want providers.FileLocations
	}{
		{"nil spec uses the default path", nil, providers.FileLocations{Folders: []string{"secrets"}}},
		{"path is cleaned", map[string]any{"path": "vault/prod/"}, providers.FileLocations{Folders: []string{filepath.Join("vault", "prod")}}},
		{"literal file is exact", map[string]any{"file": "vault/shared.enc.yaml"}, providers.FileLocations{Files: []string{"vault/shared.enc.yaml"}}},
		{"templated file covers the prefix directory", map[string]any{"file": "vault/{{ .atmos_stack }}.enc.yaml"}, providers.FileLocations{Folders: []string{"vault"}}},
		{"prefix ending mid-name covers its directory", map[string]any{"file": "vault/prod-{{ .atmos_stack }}.enc.yaml"}, providers.FileLocations{Folders: []string{"vault"}}},
		{"template at the start covers the working directory", map[string]any{"file": "{{ .atmos_stack }}.enc.yaml"}, providers.FileLocations{Folders: []string{"."}}},
		{"file wins over path", map[string]any{"file": "a/x.enc.yaml", "path": "b"}, providers.FileLocations{Files: []string{"a/x.enc.yaml"}}},
		{"selector path covers the working directory", map[string]any{"path": selector}, providers.FileLocations{Folders: []string{"."}}},
		{"selector file covers the working directory", map[string]any{"file": selector}, providers.FileLocations{Folders: []string{"."}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, possibleLocations(tt.spec))
		})
	}
}

// TestPossibleLocations_Registered proves the SOPS track is registered for conservative lookups and
// an unregistered (store-backed) track is not file-backed.
func TestPossibleLocations_Registered(t *testing.T) {
	loc, ok := providers.PossibleLocations(providers.TrackSops, map[string]any{"path": "vault"})
	require.True(t, ok)
	assert.Equal(t, []string{"vault"}, loc.Folders)

	_, ok = providers.PossibleLocations(providers.TrackStore, nil)
	assert.False(t, ok)
}
