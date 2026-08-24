package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestEnsureForComponent_NoSpecs(t *testing.T) {
	dir, err := EnsureForComponent(context.Background(), "helm", nil)
	require.NoError(t, err)
	assert.Empty(t, dir, "no declared plugins must leave HELM_PLUGINS untouched")
}

func TestEnsureForComponent_InvalidSpec(t *testing.T) {
	// An empty spec string fails to parse before any installation is attempted.
	_, err := EnsureForComponent(context.Background(), "helm", []string{""})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidHelmPluginSpec)
}

// TestExtractSpecs_Coercion covers the []any branch where non-string entries
// are coerced (123 -> "123") and nil entries are skipped.
func TestExtractSpecs_Coercion(t *testing.T) {
	got := ExtractSpecs(map[string]any{cfg.PluginsSectionName: []any{"diff@v3.9.4", 123, nil, "secrets"}})
	assert.Equal(t, []string{"diff@v3.9.4", "123", "secrets"}, got)
}

func TestStringify(t *testing.T) {
	assert.Equal(t, "diff", stringify("diff"))
	assert.Equal(t, "", stringify(nil))
	assert.Equal(t, "42", stringify(42))
}
