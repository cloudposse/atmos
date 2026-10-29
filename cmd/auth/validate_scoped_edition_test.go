package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateComponentAuthYAMLExpressionEditions(t *testing.T) {
	for _, edition := range []string{"", "2026-10-19", "2026-10-20"} {
		for _, native := range []bool{false, true} {
			for _, global := range []bool{false, true} {
				t.Run(edition+"/"+map[bool]string{false: "quoted", true: "native"}[native]+"/"+map[bool]string{false: "component", true: "global"}[global], func(t *testing.T) {
					value := "!env SECRET_SHOULD_NOT_APPEAR"
					if native {
						value = (yamlsource.Source{Tag: "!env", Argument: "SECRET_SHOULD_NOT_APPEAR"}).Encode()
					}
					config := &schema.AtmosConfiguration{Edition: edition, Auth: schema.AuthConfig{
						Providers:  map[string]schema.Provider{"sso": {Kind: "aws/iam-identity-center", Region: "us-east-1", StartURL: "https://example.awsapps.com/start/"}},
						Identities: map[string]schema.Identity{"base": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "sso"}, Principal: map[string]any{"name": "Example", "account": map[string]any{"id": "111111111111"}}}},
					}}
					section := map[string]any{}
					if global {
						config.Auth.Identities["base"].Principal["name"] = value
					} else {
						section["auth"] = map[string]any{"identities": map[string]any{"base": map[string]any{"principal": map[string]any{"name": value}}}}
					}
					err := validateComponentAuth(config, section)
					if native || edition == "2026-10-19" {
						require.ErrorContains(t, err, "cannot fully validate auth.identities.base.principal.name")
						assert.NotContains(t, err.Error(), "SECRET_SHOULD_NOT_APPEAR")
						assert.NotContains(t, err.Error(), "atmos-yaml-source")
					} else {
						require.NoError(t, err)
					}
				})
			}
		}
	}
}

func TestAuthValidateScopedNativeAndQuotedFunctions(t *testing.T) {
	for _, edition := range []string{"2026-10-19", "2026-10-20"} {
		for _, native := range []bool{false, true} {
			t.Run(edition+"/"+map[bool]string{false: "quoted", true: "native"}[native], func(t *testing.T) {
				root := setupScopedAuthFixture(t)
				t.Setenv("ATMOS_EDITION", edition)
				expression := "!exec touch EXECUTED"
				if !native {
					expression = "'" + expression + "'"
				}
				stack := "import: [catalog/auth]\nvars: {stage: expression}\nauth:\n  identities:\n    base:\n      principal:\n        name: " + expression + "\ncomponents:\n  terraform:\n    demo: {}\n"
				require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "deploy", "expression.yaml"), []byte(stack), 0o600))
				cmd := authValidateCmd
				resetAuthCmdFlags(t, cmd)
				cmd.SetContext(context.Background())
				require.NoError(t, cmd.ParseFlags([]string{"--stack", "expression"}))
				err := executeAuthValidateCommand(cmd, nil)
				if native || edition == "2026-10-19" {
					require.ErrorContains(t, err, "cannot fully validate")
				} else {
					require.NoError(t, err)
				}
				assert.NoFileExists(t, filepath.Join(root, "EXECUTED"))
			})
		}
	}
}

func TestValidateComponentAuthEncodedMapping(t *testing.T) {
	value := (yamlsource.Source{Tag: "!template", Argument: "SECRET_MAPPING"}).Encode()
	err := validateComponentAuth(&schema.AtmosConfiguration{}, map[string]any{"auth": value})
	require.ErrorContains(t, err, "cannot fully validate auth")
	assert.NotContains(t, err.Error(), "SECRET_MAPPING")
}

func TestValidateComponentAuthEncodedProviderFields(t *testing.T) {
	for _, field := range []string{"region", "scopes"} {
		t.Run(field, func(t *testing.T) {
			value := (yamlsource.Source{Tag: "!env", Argument: "SECRET_PROVIDER"}).Encode()
			provider := schema.Provider{Kind: "gcp/adc", Spec: map[string]any{}}
			if field == "region" {
				provider.Region = value
			} else {
				provider.Spec["scopes"] = []string{value}
			}
			config := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Providers: map[string]schema.Provider{"example": provider}}}
			err := validateComponentAuth(config, nil)
			require.ErrorContains(t, err, "cannot fully validate auth.providers.example")
			assert.NotContains(t, err.Error(), "SECRET_PROVIDER")
		})
	}
}
