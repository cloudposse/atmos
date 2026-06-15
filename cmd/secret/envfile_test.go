package secret

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestParseEnvSecrets(t *testing.T) {
	input := strings.NewReader("# comment\n\nA=1\nB=\"two\"\nC=three=with=eq\n")
	got, err := parseEnvSecrets(input)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"A": "1",
		"B": "two",
		"C": "three=with=eq",
	}, got)
}

func TestParseJSONSecrets(t *testing.T) {
	input := strings.NewReader(`{"A":"1","B":2}`)
	got, err := parseJSONSecrets(input)
	require.NoError(t, err)
	assert.Equal(t, "1", got["A"])
	assert.Equal(t, "2", got["B"])
}

func TestSortedKeys(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, sortedKeys(map[string]string{"c": "", "a": "", "b": ""}))
}

func TestStatusLabel(t *testing.T) {
	t.Run("initialized", func(t *testing.T) {
		st := &secrets.Status{Initialized: true}
		assert.Equal(t, "initialized", statusLabel(st))
	})
	t.Run("missing", func(t *testing.T) {
		st := &secrets.Status{Initialized: false}
		assert.Equal(t, "missing", statusLabel(st))
	})
	t.Run("error", func(t *testing.T) {
		st := &secrets.Status{Err: fmt.Errorf("access denied")}
		assert.Equal(t, "error", statusLabel(st))
	})
}

func TestBackendLabel(t *testing.T) {
	t.Run("no name", func(t *testing.T) {
		decl := secrets.Declaration{}
		assert.Equal(t, "(none)", backendLabel(&decl))
	})
	t.Run("sops backend", func(t *testing.T) {
		decl := secrets.Declaration{BackendType: secrets.BackendSops, BackendName: "dev-sops"}
		assert.Equal(t, "sops:dev-sops", backendLabel(&decl))
	})
}

func TestStatusesToData(t *testing.T) {
	statuses := []secrets.Status{
		{
			Declaration: secrets.Declaration{
				Name:        "DATADOG_API_KEY",
				Description: "Datadog API key",
				BackendType: secrets.BackendSops,
				BackendName: "dev-sops",
				Scope:       secrets.ScopeStack,
			},
			Initialized: true,
		},
	}
	rows := statusesToData("dev", "api", statuses)
	require.Len(t, rows, 1)
	assert.Equal(t, "dev", rows[0]["stack"])
	assert.Equal(t, "api", rows[0]["component"])
	assert.Equal(t, "DATADOG_API_KEY", rows[0]["secret"])
	assert.Equal(t, "stack", rows[0]["scope"])
	assert.Equal(t, "sops:dev-sops", rows[0]["provider"])
	assert.Equal(t, "initialized", rows[0]["status"])
	assert.Equal(t, "Datadog API key", rows[0]["description"])
}

func TestSecretListColumns(t *testing.T) {
	cols := secretListColumns(false)
	require.Len(t, cols, 6)
	assert.Equal(t, "Stack", cols[0].Name)
	assert.Equal(t, "Scope", cols[3].Name)
	assert.Equal(t, "Status", cols[5].Name)

	verboseCols := secretListColumns(true)
	require.Len(t, verboseCols, 7)
	assert.Equal(t, "Description", verboseCols[6].Name)
}
