package sops

import (
	"fmt"
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

func TestSopsKeygenScopeDerivedRecipients(t *testing.T) {
	for _, pathKind := range []string{"default", "relative", "absolute"} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/nested=%t", pathKind, nested), func(t *testing.T) {
				base := t.TempDir()
				t.Chdir(base)
				t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(base, "keys.txt"))
				t.Setenv("SOPS_AGE_KEY", "")
				p := &sopsProvider{name: "vault", kind: "sops/age"}
				dir := "secrets"
				if pathKind != "default" {
					dir = filepath.Join("vault", "encrypted")
					p.path = dir
				}
				if pathKind == "absolute" {
					p.path = filepath.Join(base, dir)
				}
				if nested {
					p.recipientsFile = filepath.Join(dir, ".sops.yaml")
				}
				_, err := p.GenerateKey(base)
				require.NoError(t, err)
				for _, scope := range []providers.Scope{providers.ScopeStack, providers.ScopeInstance} {
					coord := providers.Coordinate{Stack: "dev", Component: "vpc/primary", Scope: scope, Key: "TOKEN"}
					require.NoError(t, p.Set(coord, "scope-derived-value"))
					name := "dev.enc.yaml"
					if scope == providers.ScopeInstance {
						name = filepath.Join("dev.vpc", "primary.enc.yaml")
					}
					encrypted, err := os.ReadFile(filepath.Join(base, dir, name))
					require.NoError(t, err)
					assert.NotContains(t, string(encrypted), "scope-derived-value")
					value, err := p.Get(coord)
					require.NoError(t, err)
					assert.Equal(t, "scope-derived-value", value)
				}
			})
		}
	}
}
