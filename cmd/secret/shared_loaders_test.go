package secret

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	storepkg "github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/providers"
)

// writeMinimalAtmosProject writes a self-contained Atmos project (config + one stack + one
// terraform component) into a temp dir and returns its path. The component declares no auth and no
// secrets, so loadService resolves it fully in-process — InitCliConfig, a no-op auth manager, and
// component description — without any cloud credentials.
func writeMinimalAtmosProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	write("atmos.yaml", `base_path: "."
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.stage}}"
`)
	write("stacks/deploy/dev.yaml", `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        name: myvpc
`)
	write("components/terraform/vpc/main.tf", "# vpc component.\n")
	return dir
}

// writeAtmosProjectWithUnresolvedState is writeMinimalAtmosProject plus a second component,
// app-config, whose vars reference the vpc component's terraform state (never provisioned in this
// test) and which declares one secret. It reproduces a component shaped like
// examples/quick-start-advanced's app-config: resolving where to write/read its secrets should not
// require sibling components' terraform state to already exist.
func writeAtmosProjectWithUnresolvedState(t *testing.T) string {
	t.Helper()

	dir := writeMinimalAtmosProject(t)
	full := filepath.Join(dir, "stacks", "deploy", "dev.yaml")
	require.NoError(t, os.WriteFile(full, []byte(`vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        name: myvpc
    app-config:
      vars:
        bucket_id: !terraform.state vpc bucket_id
      secrets:
        vars:
          API_KEY:
            store: secrets/ssm
            required: true
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "components", "terraform", "app-config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "components", "terraform", "app-config", "main.tf"), []byte("# app-config component.\n"), 0o644))
	return dir
}

// TestLoadServiceAndConfig_ToleratesUnresolvedTerraformState guards against a regression where
// loadServiceAndConfig (backing every `secret set/get/delete/push/pull/import/init/validate`
// subcommand) eagerly evaluated every YAML function in the component's full merged config —
// including !terraform.state references to sibling components that haven't been deployed yet —
// even though resolving where to write/read a secret only needs secrets.vars/secrets.providers.
func TestLoadServiceAndConfig_ToleratesUnresolvedTerraformState(t *testing.T) {
	t.Chdir(writeAtmosProjectWithUnresolvedState(t))

	svc, atmosConfig, err := loadServiceAndConfig(secretScope{Stack: "dev", Component: "app-config"})
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, atmosConfig)
	assert.True(t, svc.IsDeclared("API_KEY"), "the secret declared on app-config must still be visible")
}

func TestLoadService_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	svc, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)

	// The component declares no secrets, so the service has no declarations and reports unknown
	// names as not declared — confirming a real, queryable service was built.
	assert.Empty(t, svc.Declarations())
	assert.False(t, svc.IsDeclared("ANY_SECRET"))
}

func TestLoadServiceAndConfig_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	svc, atmosConfig, err := loadServiceAndConfig(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, atmosConfig)
	assert.NotEmpty(t, atmosConfig.BasePath, "the resolved config must carry the project base path")
}

func TestLoadService_InitConfigError(t *testing.T) {
	// An empty dir has no Atmos config, so InitCliConfig fails before any component work.
	t.Chdir(t.TempDir())

	_, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestInjectSecretStoreAuthResolver_ResolverOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	authManager := authtypes.NewMockAuthManager(ctrl)
	mockStore := storepkg.NewMockIdentityAwareStore(ctrl)

	authManager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{})
	mockStore.EXPECT().
		SetAuthContext(gomock.Not(nil), "").
		Do(func(resolver storepkg.AuthContextResolver, identityName string) {
			assert.NotNil(t, resolver)
			assert.Empty(t, identityName)
		})

	atmosConfig := &schema.AtmosConfiguration{
		Stores: storepkg.StoreRegistry{
			"explicit-identity-store": mockStore,
		},
	}

	injectSecretStoreAuthResolver(atmosConfig, authManager, secretScope{Identity: "explicit-identity"})
}

func TestInjectSecretStoreAuthResolver_AppliesDefaultIdentity(t *testing.T) {
	// With no explicit --identity, the effective identity is the tail of the auth chain.
	// injectSecretStoreAuthResolver computes it and records it on SecretsAuth (consumed by the
	// cloud-KMS SOPS path). This test asserts that CLI-side computation and wiring, and exercises
	// the SetAuthContextResolverWithDefaultIdentity call against a real registry; it does not itself
	// assert the per-store result. The store-level application of the default (identity-less stores
	// adopt it, explicit-identity stores keep theirs) is asserted in
	// pkg/store.TestSetAuthContextResolverWithDefaultIdentity_DefaultsOnlyEmptyStores.
	ctrl := gomock.NewController(t)
	authManager := authtypes.NewMockAuthManager(ctrl)
	authManager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{}).AnyTimes()
	authManager.EXPECT().GetChain().Return([]string{"sso", "role-b"}).AnyTimes()

	// A real, identity-less SSM store (client init is deferred, so no credentials are needed).
	ssmStore, err := providers.NewSSMStore(providers.SSMStoreOptions{Region: "us-east-1"}, "")
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		Stores: storepkg.StoreRegistry{"secrets/ssm": ssmStore},
	}

	injectSecretStoreAuthResolver(atmosConfig, authManager, secretScope{})

	require.NotNil(t, atmosConfig.SecretsAuth)
	assert.Equal(t, "role-b", atmosConfig.SecretsAuth.DefaultIdentity)
	assert.NotNil(t, atmosConfig.SecretsAuth.Resolver)
}

func TestLoadServiceAndConfig_ComponentNotFound(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// The stack exists but the component does not, so buildAuthManager's component description
	// fails — exercising the auth-load error path.
	_, _, err := loadServiceAndConfig(secretScope{Stack: "dev", Component: "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load component config for auth")
}

func TestLoadServiceSeams_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// The deps.go seam wrappers delegate to the real loaders; drive them directly so their wrapping
	// logic is covered, not just the underlying functions.
	svc, err := loadServiceFn(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)

	svc2, atmosConfig, err := loadServiceAndConfigFn(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc2)
	require.NotNil(t, atmosConfig)
}

func TestLoadServiceSeams_Error(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := loadServiceFn(secretScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)

	_, _, err = loadServiceAndConfigFn(secretScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
}

func TestLoadServiceForList_VerifyFalse_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// verify=false is the credential-free path: it resolves the component with auth disabled and
	// builds the service from declarations alone — no identity authentication, no store resolver.
	svc, err := loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, false)
	require.NoError(t, err)
	require.NotNil(t, svc)

	// The component declares no secrets, so the service is real but has no declarations.
	assert.Empty(t, svc.Declarations())
	assert.False(t, svc.IsDeclared("ANY_SECRET"))
}

func TestLoadServiceForList_VerifyTrue_Delegates(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// verify=true delegates to loadService, which authenticates; the no-auth/no-secret component
	// still resolves fully in-process, so a real service is returned.
	svc, err := loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, true)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Empty(t, svc.Declarations())
}

func TestLoadServiceForList_InitConfigError(t *testing.T) {
	// An empty dir has no Atmos config, so InitCliConfig fails before any component work, on both
	// the verify=false branch and (via loadService) the verify=true branch.
	t.Chdir(t.TempDir())

	_, err := loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)

	_, err = loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestLoadServiceForListSeam_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// The deps.go seam wrapper delegates to the real loader; drive it directly (both verify values)
	// so its wrapping logic is covered, not just the underlying function.
	svc, err := loadServiceForListFn(secretScope{Stack: "dev", Component: "vpc"}, false)
	require.NoError(t, err)
	require.NotNil(t, svc)

	svc2, err := loadServiceForListFn(secretScope{Stack: "dev", Component: "vpc"}, true)
	require.NoError(t, err)
	require.NotNil(t, svc2)
}

func TestLoadServiceForListSeam_Error(t *testing.T) {
	t.Chdir(t.TempDir())

	// Both seam branches must propagate the underlying loader error: verify=false (credential-free)
	// and verify=true (delegates to loadService) both fail when there is no Atmos config.
	_, err := loadServiceForListFn(secretScope{Stack: "dev", Component: "vpc"}, false)
	require.Error(t, err)

	_, err = loadServiceForListFn(secretScope{Stack: "dev", Component: "vpc"}, true)
	require.Error(t, err)
}

func TestBuildAuthManager_ComponentNotFound(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// Resolve a real config, then drive buildAuthManager directly with a missing component to cover
	// its error branch independently of the loaders.
	_, atmosConfig, err := loadServiceAndConfig(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)

	_, err = buildAuthManager(atmosConfig, secretScope{Stack: "dev", Component: "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load component config for auth")
}

// Both collision validation and ordinary loading must ignore unrelated live
// outputs when discovering secret declarations, even if the producer is absent.
func TestSecretDiscoverySkipsCloudFormationOutputs(t *testing.T) {
	dir := writeMinimalAtmosProject(t)
	configPath := filepath.Join(dir, "atmos.yaml")
	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, append(configData, []byte("templates:\n  settings:\n    enabled: true\n")...), 0o644))
	manifest := `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        live: !aws.cloudformation.output missing-producer dev Value
        template_live: '{{ (atmos.Component "missing-producer" "dev").outputs.Value }}'
        vault_name: vault
      secrets:
        vars:
          TOKEN:
            store: '{{ .vars.vault_name }}'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
	t.Chdir(dir)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	require.NoError(t, checkStackSopsCollisions(secretScope{Stack: "dev"}))
	svc, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.True(t, svc.IsDeclared("TOKEN"))
	require.Equal(t, "vault", svc.Declarations()[0].BackendName)
}

// TestSecretBackendSelectorsResolveCloudFormationOutputs verifies the authenticated service resolves
// both backend selectors lazily for the declarations used, while listing remains credential-free and
// unrelated component outputs are never fetched.
func TestSecretBackendSelectorsResolveCloudFormationOutputs(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if !assert.NoError(t, r.ParseForm()) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, "DescribeStacks", r.Form.Get("Action"))
		assert.Equal(t, "dev-secret-backends", r.Form.Get("StackName"))
		assert.Contains(t, r.Header.Get("Authorization"), "Credential=selector-test/")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, `<DescribeStacksResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/"><DescribeStacksResult><Stacks><member><StackName>dev-secret-backends</StackName><StackStatus>CREATE_COMPLETE</StackStatus><Outputs><member><OutputKey>StoreName</OutputKey><OutputValue>resolved-store</OutputValue></member><member><OutputKey>SopsName</OutputKey><OutputValue>resolved-sops</OutputValue></member></Outputs></member></Stacks></DescribeStacksResult></DescribeStacksResponse>`)
	}))
	defer server.Close()
	dir := writeMinimalAtmosProject(t)
	manifest := `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        unrelated: !aws.cloudformation.output missing-producer dev Value
      secrets:
        vars:
          API_KEY:
            store: !aws.cloudformation.output secret-backends dev StoreName
            value: !aws.cloudformation.output missing-producer dev Ignored
          SOPS_KEY:
            sops: !aws.cloudformation.output secret-backends dev SopsName
  aws/cloudformation:
    secret-backends:
      stack_name: dev-secret-backends
      settings:
        aws_cloudformation:
          region: us-east-2
      template:
        Resources: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
	t.Chdir(dir)
	t.Setenv("AWS_ENDPOINT_URL_CLOUDFORMATION", server.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "selector-test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "synthetic-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	scope := secretScope{Stack: "dev", Component: "vpc"}
	listed, err := loadServiceForList(scope, false)
	require.NoError(t, err)
	require.Len(t, listed.Declarations(), 2)
	for _, declaration := range listed.Declarations() {
		assert.Contains(t, declaration.BackendName, "!aws.cloudformation.output")
	}
	assert.Zero(t, requests.Load(), "credential-free listing must not read output selectors")
	loaded, err := loadService(scope)
	require.NoError(t, err)
	require.Len(t, loaded.Declarations(), 2)
	assert.Zero(t, requests.Load(), "loading must not evaluate selectors for declarations no command uses")

	// Selectors resolve lazily, per declaration actually used. The resolved names are not configured
	// in this project, so each lookup fails on the resolved name (not on the raw selector text), which
	// proves the CloudFormation output was read and substituted.
	_, err = loaded.Get("API_KEY", secrets.ResolveOptions{})
	require.ErrorIs(t, err, secrets.ErrStoreNotFound)
	assert.Contains(t, err.Error(), `"resolved-store"`)
	assert.Equal(t, int32(1), requests.Load(), "only the used declaration is resolved")

	_, err = loaded.Get("SOPS_KEY", secrets.ResolveOptions{})
	require.ErrorIs(t, err, secrets.ErrProviderNotFound)
	assert.Contains(t, err.Error(), `"resolved-sops"`)
	assert.Equal(t, int32(2), requests.Load())
}

// TestSecretDeclarationFieldsAndProvidersPreserved covers supported fields and
// provider-specific nested options through includes and local template dependencies.
func TestSecretDeclarationFieldsAndProvidersPreserved(t *testing.T) {
	dir := writeMinimalAtmosProject(t)
	configPath := filepath.Join(dir, "atmos.yaml")
	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, append(configData, []byte("templates:\n  settings:\n    enabled: true\n")...), 0o644))
	declaration := `sops: '{{ .vars.vault }}'
description: '{{ .vars.description }}'
reference: '{{ .vars.reference }}'
required: true
value: !aws.cloudformation.output missing-producer dev Ignored
ignored_template: '{{ (atmos.Component "missing-producer" "dev").outputs.Value }}'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "declaration.yaml"), []byte(declaration), 0o644))
	manifest := `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        vault: local
        description: token description
        reference: token-reference
        provider_kind: sops/age
        provider_file: fixture.enc.yaml
        nested_option: nested-config
        unrelated: !aws.cloudformation.output missing-producer dev Ignored
      secrets:
        vars:
          TOKEN: !include declaration.yaml
          GLOBAL:
            store: vault
            scope: global
            value: !aws.cloudformation.output missing-producer dev Ignored
        providers:
          local:
            kind: '{{ .vars.provider_kind }}'
            spec:
              file: '{{ .vars.provider_file }}'
              extension:
                nested: '{{ .vars.nested_option }}'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
	t.Chdir(dir)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	scope := secretScope{Stack: "dev", Component: "vpc"}
	loaded, err := loadService(scope)
	require.NoError(t, err)
	require.Equal(t, []secrets.Declaration{
		{Name: "GLOBAL", BackendType: secrets.BackendStore, BackendName: "vault", Scope: secrets.ScopeGlobal},
		{Name: "TOKEN", BackendType: secrets.BackendSops, BackendName: "local", Description: "token description", Reference: "token-reference", Required: true, Scope: secrets.ScopeInstance},
	}, loaded.Declarations())
	assert.Equal(t, []string{"fixture.enc.yaml"}, loaded.FileDependencies(), "the resolved provider definition must reach the real SOPS provider")
	entries, _, err := enumerateSecretScopes(scope)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	expected := map[string]any{"local": map[string]any{"kind": "sops/age", "spec": map[string]any{"file": "fixture.enc.yaml", "extension": map[string]any{"nested": "nested-config"}}}}
	assert.Equal(t, expected, secrets.ExtractProviders(entries[0].Section))
}

// TestSecretGeneratedDeclarationIgnoresUnsupportedFields retains declaration maps
// produced by !template while excluding unsupported output reads inside them.
func TestSecretGeneratedDeclarationIgnoresUnsupportedFields(t *testing.T) {
	dir := writeMinimalAtmosProject(t)
	manifest := `vars:
  stage: dev
components:
  terraform:
    vpc:
      secrets:
        vars: !template '{"TOKEN":{"store":"vault","description":"generated","value":"!aws.cloudformation.output missing-producer dev Ignored"}}'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
	t.Chdir(dir)
	loaded, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.Equal(t, []secrets.Declaration{{Name: "TOKEN", BackendType: secrets.BackendStore, BackendName: "vault", Description: "generated", Scope: secrets.ScopeInstance}}, loaded.Declarations())
}
