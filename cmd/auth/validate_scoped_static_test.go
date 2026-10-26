package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthValidateScopedStaticLoading(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{
			name: "top level and imported locals remain inert",
			manifest: `import: [catalog/static]
vars: {stage: static}
locals:
  forbidden: !exec atmos-auth-static-must-not-execute
components:
  terraform:
    demo:
      metadata:
        terraform_workspace_template: '{{ file.Read "must-not-read" }}'
      vars:
        include: !include.raw must-not-read
`,
		},
		{
			name: "unresolved include cannot satisfy required auth value",
			manifest: `import: [catalog/static]
vars: {stage: static}
components:
  terraform:
    demo:
      auth:
        identities:
          deploy:
            principal:
              assume_role: !include.raw must-not-read
`,
			want: "cannot fully validate",
		},
		{
			name: "remote import is not fetched",
			manifest: `import: [https://must-not-fetch.invalid/stack.yaml]
vars: {stage: static}
components:
  terraform:
    demo: {}
`,
			want: "without fetching it",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root := setupScopedAuthFixture(t)
			imported := `import: [catalog/auth]
locals:
  forbidden: !exec atmos-auth-import-must-not-execute
`
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "catalog", "static.yaml"), []byte(imported), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "deploy", "static.yaml"), []byte(tt.manifest), 0o600))
			cmd := authValidateCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags([]string{"--stack", "static"}))
			err := executeAuthValidateCommand(cmd, nil)
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}

func TestAuthValidateScopedStaticImports(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		custom      bool
		skipMissing bool
		want        string
	}{
		{name: "required template", path: "catalog/{{ .vars.catalog }}", want: "cannot inspect templated import"},
		{name: "required custom template", path: "catalog/[[ .vars.catalog ]]", custom: true, want: "cannot inspect templated import"},
		{name: "optional template", path: "catalog/{{ .vars.catalog }}", skipMissing: true, want: "cannot inspect templated import"},
		{name: "optional custom template", path: "catalog/[[ .vars.catalog ]]", custom: true, skipMissing: true, want: "cannot inspect templated import"},
		{name: "literal invalid identity", path: "catalog/invalid", want: `referenced identity "nonexistent" does not exist`},
		{name: "literal valid import", path: "catalog/auth"},
		{name: "literal optional missing", path: "catalog/absent", skipMissing: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root := setupScopedAuthFixture(t)
			invalid := `auth:
  identities:
    omitted:
      kind: aws/assume-role
      via: {identity: nonexistent}
      principal: {assume_role: 'arn:aws:iam::111111111111:role/test'}
`
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "catalog", "invalid.yaml"), []byte(invalid), 0o600))
			manifest := fmt.Sprintf(`import:
  - catalog/auth
  - path: %q
    skip_if_missing: %t
vars: {stage: static, catalog: invalid}
components:
  terraform:
    demo: {}
`, tt.path, tt.skipMissing)
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "deploy", "static.yaml"), []byte(manifest), 0o600))
			if tt.custom {
				configPath := filepath.Join(root, "atmos.yaml")
				config, err := os.ReadFile(configPath)
				require.NoError(t, err)
				config = append(config, []byte("\ntemplates:\n  settings:\n    delimiters: ['[[', ']]']\n")...)
				require.NoError(t, os.WriteFile(configPath, config, 0o600))
			}
			cmd := authValidateCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags([]string{"--stack", "static", "--component", "demo"}))
			err := executeAuthValidateCommand(cmd, nil)
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
			require.NoFileExists(t, filepath.Join(root, "EXECUTED"))
			require.NoFileExists(t, filepath.Join(root, "aws", "credentials"))
		})
	}
}
