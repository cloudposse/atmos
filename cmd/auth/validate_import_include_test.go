package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	// Match the CLI's registration of built-in configuration import adapters.
	_ "github.com/cloudposse/atmos/pkg/config/adapters"
)

func TestAuthValidateImportedConfigWithInclude(t *testing.T) {
	for _, test := range []struct{ name, imports, want string }{
		{"invalid imported auth", "[base.yaml]", "missing-control"},
		{"required missing import", "[{path: missing-required.yaml, skip_if_missing: false}]", "required config import failed to load"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := setupScopedAuthFixture(t)
			files := map[string]string{
				"atmos.yaml": "base_path: ./\nimport: " + test.imports + `
mcp:
  servers:
    local:
      command: local-control
      env: !include env.yaml
`,
				"env.yaml": "UPPER: value\n",
				"base.yaml": `auth:
  identities:
    security-control:
      kind: aws/permission-set
      via: {provider: missing-control}
      principal:
        name: Guardrail
        account: {id: '123456789012'}
`,
			}
			for name, content := range files {
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o600))
			}
			cmd := authValidateCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags(nil))
			require.ErrorContains(t, executeAuthValidateCommand(cmd, nil), test.want)
		})
	}
}
