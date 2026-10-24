package sops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

func TestNewSops_FromSection(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	section := map[string]any{
		"dev-sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{"file": "secrets/dev.enc.yaml"},
		},
	}
	p, err := New(cfg, "dev-sops", section)
	require.NoError(t, err)
	assert.Equal(t, "sops/age", p.Kind())
}

// TestNewSops_FromSectionNamespacedName is the pkg/secrets side of cloudposse/atmos#3185: SOPS
// provider names are opaque map keys, so a namespaced name containing ':' must resolve, matching the
// relaxed manifest schema for `secrets.providers`.
func TestNewSops_FromSectionNamespacedName(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	section := map[string]any{
		"team/prod:sops": map[string]any{
			"kind": "sops/age",
			"spec": map[string]any{"file": "secrets/prod.enc.yaml"},
		},
	}
	p, err := New(cfg, "team/prod:sops", section)
	require.NoError(t, err)
	assert.Equal(t, "sops/age", p.Kind())
}

func TestNewSops_NotFound(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	_, err := New(cfg, "missing", nil)
	require.ErrorIs(t, err, providers.ErrProviderNotFound)
}
