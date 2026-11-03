package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authpkg "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// TestAuthValidateScoped exercises the actual config/stack/merge/validator pipeline
// without credentials. Each invocation gets isolated config, caches and flag state.
func TestAuthValidateScoped(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "global remains strict", want: "principal is required"},
		{name: "imported principal", args: []string{"--stack", "dev"}},
		{name: "physical stack", args: []string{"-s", "deploy/dev", "-c", "demo"}},
		{name: "different stack principal", args: []string{"-s", "prod", "-c", "demo"}},
		{name: "component override", args: []string{"-s", "mixed", "-c", "good"}},
		{name: "invalid sibling", args: []string{"-s", "mixed"}, want: `component "bad" (helmfile)`},
		{name: "all matching types", args: []string{"-s", "shared", "-c", "demo"}, want: `(helmfile)`},
		{name: "abstract skipped", args: []string{"-s", "abstract"}},
		{name: "missing component", args: []string{"-s", "dev", "-c", "absent"}, want: "no concrete components"},
		{name: "missing stack", args: []string{"-s", "absent"}, want: "no concrete components"},
		{name: "empty stack", args: []string{"-s", "empty"}, want: "no concrete components"},
		{name: "component needs stack", args: []string{"-c", "demo"}, want: "--component requires --stack"},
		{name: "invalid chain", args: []string{"-s", "cycle"}, want: "cycle"},
		{name: "missing identity", args: []string{"-s", "missing-parent"}, want: "missing"},
		{name: "dynamic principal", args: []string{"-s", "dynamic"}, want: "cannot fully validate"},
		{name: "missing provider", args: []string{"-s", "missing-provider"}, want: "missing-provider"},
		{name: "native function", args: []string{"-s", "function"}, want: "cannot fully validate"},
		{name: "unrelated expressions are not evaluated", args: []string{"-s", "inert"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root := setupScopedAuthFixture(t)
			cmd := authValidateCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags(tt.args))
			err := executeAuthValidateCommand(cmd, nil)
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
			assert.NoFileExists(t, filepath.Join(root, "EXECUTED"))
			assert.NoFileExists(t, filepath.Join(root, "aws", "credentials"))
		})
	}
}

func setupScopedAuthFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "xdg-data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "xdg-cache"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(root, "aws", "credentials"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	viper.Reset()
	t.Cleanup(viper.Reset)
	files := map[string]string{
		"atmos.yaml": `base_path: ./
stacks:
  base_path: stacks
  included_paths: ['deploy/**/*']
  name_pattern: '{stage}'
auth:
  providers:
    example-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://example.awsapps.com/start/
  identities:
    base:
      kind: aws/permission-set
      default: true
      via: {provider: example-sso}
      principal:
        name: ExamplePermissionSet
        account: {id: '111111111111'}
    deploy:
      kind: aws/assume-role
      via: {identity: base}
`,
		"stacks/catalog/auth.yaml": `auth:
  identities:
    deploy:
      principal: {assume_role: 'arn:aws:iam::222222222222:role/dev'}
`,
		"stacks/deploy/dev.yaml": `import: [catalog/auth]
vars: {stage: dev}
components:
  terraform:
    demo: {vars: {}}
`,
		"stacks/deploy/prod.yaml": `import: [catalog/auth]
vars: {stage: prod}
auth:
  identities:
    deploy:
      principal: {assume_role: 'arn:aws:iam::333333333333:role/prod'}
components:
  terraform:
    demo: {vars: {}}
`,
		"stacks/deploy/mixed.yaml": `vars: {stage: mixed}
components:
  terraform:
    good:
      auth:
        identities:
          deploy:
            principal: {assume_role: 'arn:aws:iam::222222222222:role/component'}
  helmfile:
    bad: {vars: {}}
`,
		"stacks/deploy/shared.yaml": `vars: {stage: shared}
components:
  terraform:
    demo:
      auth:
        identities:
          deploy:
            principal: {assume_role: 'arn:aws:iam::222222222222:role/component'}
  helmfile:
    demo: {vars: {}}
`,
		"stacks/deploy/abstract.yaml": `vars: {stage: abstract}
components:
  terraform:
    base: {metadata: {type: abstract}}
    demo:
      auth:
        identities:
          deploy:
            principal: {assume_role: 'arn:aws:iam::222222222222:role/component'}
`,
		"stacks/deploy/empty.yaml": `vars: {stage: empty}
`,
		"stacks/deploy/cycle.yaml": `import: [catalog/auth]
vars: {stage: cycle}
auth:
  identities:
    deploy:
      via: {identity: deploy}
components:
  terraform:
    demo: {vars: {}}
`,
		"stacks/deploy/missing-parent.yaml": `import: [catalog/auth]
vars: {stage: missing-parent}
auth:
  identities:
    deploy:
      via: {identity: missing}
components:
  terraform:
    demo: {vars: {}}
`,
		"stacks/deploy/dynamic.yaml": `vars: {stage: dynamic, role_arn: "arn:aws:iam::222222222222:role/dynamic"}
auth:
  identities:
    deploy:
      principal: {assume_role: '{{ .vars.role_arn }}'}
components:
  terraform:
    demo: {vars: {}}
`,
		"stacks/deploy/missing-provider.yaml": `import: [catalog/auth]
vars: {stage: missing-provider}
auth:
  identities:
    base:
      via: {provider: missing-provider}
components:
  terraform:
    demo: {vars: {}}
`,
		"stacks/deploy/function.yaml": `vars: {stage: function}
auth:
  identities:
    deploy:
      principal:
        assume_role: !exec touch EXECUTED
components:
  terraform:
    demo: {vars: {}}
`,
		"stacks/deploy/inert.yaml": `import: [catalog/auth]
vars:
  stage: inert
  unrelated: !exec touch EXECUTED
  unrelated_template: '{{ env.Getenv "AUTH_VALIDATE_MUST_NOT_EVALUATE" }}'
components:
  terraform:
    demo: {vars: {}}
`,
	}
	for name, contents := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	}
	return root
}

// TestAuthValidateScopedGlobalFlags proves that scoped validation retains config
// overlays and profiles rather than initializing configuration a second time.
func TestAuthValidateScopedGlobalFlags(t *testing.T) {
	for _, selector := range []string{"config", "config-path", "profile", "base-path"} {
		t.Run(selector, func(t *testing.T) {
			root := setupScopedAuthFixture(t)
			base, readErr := os.ReadFile(filepath.Join(root, "atmos.yaml"))
			require.NoError(t, readErr)
			overlay := string(base) + "  logs:\n    level: InvalidSelectedConfig\n"
			var value string
			switch selector {
			case "config":
				value = filepath.Join(root, "overlay.yaml")
				require.NoError(t, os.WriteFile(value, []byte(overlay), 0o600))
			case "config-path":
				value = filepath.Join(root, "overlay")
				require.NoError(t, os.MkdirAll(value, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(value, "atmos.yaml"), []byte(overlay), 0o600))
			case "profile":
				value = "validation"
				profileDir := filepath.Join(root, "profiles", value)
				require.NoError(t, os.MkdirAll(profileDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(profileDir, "auth.yaml"), []byte(overlay), 0o600))
			case "base-path":
				value = filepath.Join(root, "alternate")
				require.NoError(t, os.MkdirAll(value, 0o755))
				require.NoError(t, os.Rename(filepath.Join(root, "stacks"), filepath.Join(value, "stacks")))
			}
			cmd, globalParser := newTestCommandWithGlobalParser("validate")
			validateParser.RegisterFlags(cmd)
			require.NoError(t, cmd.ParseFlags([]string{"--stack", "dev", "--" + selector + "=" + value}))
			require.NoError(t, globalParser.BindFlagsToViper(cmd, viper.GetViper()))
			err := executeAuthValidateCommand(cmd, nil)
			if selector == "base-path" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "InvalidSelectedConfig")
			}
		})
	}
}

func TestCollectAuthValidationScopesAllTypes(t *testing.T) {
	kinds := []string{"terraform", "helmfile", "packer", "ansible", "aws/cloudformation", "helm", "kubernetes", "container", "emulator", "custom"}
	components := map[string]any{}
	for _, kind := range kinds {
		components[kind] = map[string]any{"demo": map[string]any{}, "base": map[string]any{"metadata": map[string]any{"type": "abstract"}}}
	}
	scopes := collectAuthValidationScopes(map[string]any{"dev": map[string]any{"components": components}})
	require.Len(t, scopes, len(kinds))
	var actual []string
	for _, scope := range scopes {
		assert.Equal(t, "demo", scope.component)
		actual = append(actual, scope.kind)
	}
	assert.ElementsMatch(t, kinds, actual)
	assert.IsIncreasing(t, actual)
}

func TestRejectDynamicAuthValues(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		custom bool
		want   bool
	}{
		{name: "template", value: "{{ .vars.role }}", want: true},
		{name: "custom delimiters", value: "[[ .vars.role ]]", custom: true, want: true},
		{name: "different delimiters are data", value: "{{ .vars.role }}", custom: true},
		{name: "native Starlark", value: (starlarksource.Source{Code: "result = 42"}).Encode(), want: true},
		{name: "function", value: "!terraform.output vpc id", want: true},
		{name: "mapping expression", value: map[string]any{"principal": "!template '{{ .vars.principal }}'"}, want: true},
		{name: "list expression", value: []any{"static", "!env SECRET"}, want: true},
		{name: "unrelated bang", value: "!not-a-function"},
		{name: "ARN", value: "arn:aws:iam::222222222222:role/example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pair := delimiters.Default()
			if tt.custom {
				pair = delimiters.Delimiters{Left: "[[", Right: "]]"}
			}
			err := rejectDynamicAuthValues(tt.value, "auth.test", pair, true)
			if tt.want {
				require.ErrorContains(t, err, "cannot fully validate auth.test")
				assert.NotContains(t, err.Error(), "SECRET")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateComponentAuthDoesNotMutateGlobal(t *testing.T) {
	config := &schema.AtmosConfiguration{Auth: schema.AuthConfig{
		Providers: map[string]schema.Provider{"sso": {Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start/"}},
		Identities: map[string]schema.Identity{
			"base":   {Kind: "aws/permission-set", Default: true, Via: &schema.IdentityVia{Provider: "sso"}, Principal: map[string]any{"name": "Example", "account": map[string]any{"id": "111111111111"}}},
			"deploy": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: "base"}},
		},
	}}
	before, err := authpkg.AuthConfigToMap(&config.Auth)
	require.NoError(t, err)
	section := map[string]any{"auth": map[string]any{"identities": map[string]any{"deploy": map[string]any{"default": true, "principal": map[string]any{"assume_role": "arn:aws:iam::222222222222:role/demo"}}}}}
	require.NoError(t, validateComponentAuth(config, section))
	after, err := authpkg.AuthConfigToMap(&config.Auth)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestValidateComponentAuthMalformedAndDynamicMappings(t *testing.T) {
	for _, value := range []string{"not-a-mapping", "!template '{{ .vars.auth }}'"} {
		t.Run(value, func(t *testing.T) {
			err := validateComponentAuth(&schema.AtmosConfiguration{}, map[string]any{"auth": value})
			require.Error(t, err)
		})
	}
}

func TestAuthValidateScopedNameTemplate(t *testing.T) {
	root := setupScopedAuthFixture(t)
	path := filepath.Join(root, "atmos.yaml")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	contents = []byte(strings.ReplaceAll(string(contents), "name_pattern: '{stage}'", "name_template: '{{ .vars.stage }}'"))
	require.NoError(t, os.WriteFile(path, contents, 0o600))
	cmd := authValidateCmd
	resetAuthCmdFlags(t, cmd)
	require.NoError(t, cmd.ParseFlags([]string{"--stack", "dev"}))
	require.NoError(t, executeAuthValidateCommand(cmd, nil))
}
