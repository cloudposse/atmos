package sops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

func TestSopsKeygenNestedRecipients(t *testing.T) {
	for _, absolute := range []bool{false, true} {
		name := "relative file"
		if absolute {
			name = "absolute file"
		}
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			t.Chdir(base)
			keys := filepath.Join(base, "keys.txt")
			t.Setenv("SOPS_AGE_KEY_FILE", keys)
			t.Setenv("SOPS_AGE_KEY", "")
			file := filepath.Join("secrets", "{{ .atmos_stack }}.enc.yaml")
			if absolute {
				file = filepath.Join(base, file)
			}
			p := &sopsProvider{name: "vault", kind: "sops/age", file: file, recipientsFile: filepath.Join("secrets", ".sops.yaml")}
			_, err := p.GenerateKey(base)
			require.NoError(t, err)
			coord := providers.Coordinate{Stack: "dev", Key: "TOKEN"}
			require.NoError(t, p.Set(coord, "nested-recipient-value"))
			encrypted, err := os.ReadFile(filepath.Join(base, "secrets", "dev.enc.yaml"))
			require.NoError(t, err)
			assert.NotContains(t, string(encrypted), "nested-recipient-value")
			value, err := p.Get(coord)
			require.NoError(t, err)
			assert.Equal(t, "nested-recipient-value", value)
		})
	}
}
