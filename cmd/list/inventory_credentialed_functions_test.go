package list

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Compile-time sentinels: the fixture below and the assertions in this file depend on
// these YAML function names. A rename must break the build, not silently weaken the test.
var (
	_ = u.AtmosYamlFuncTerraformState
	_ = u.AtmosYamlFuncTerraformOutput
	_ = u.AtmosYamlFuncStore
)

// crossAccountAtmosYAML mirrors a multi-account repository: one stack per stage, each
// stage in its own account with its own Terraform state backend.
const crossAccountAtmosYAML = `base_path: "./"

components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
    init_run_reconfigure: true
    auto_generate_backend_file: false

stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  excluded_paths:
    - "**/_defaults.yaml"
  name_pattern: "{stage}"

templates:
  settings:
    enabled: true
    sprig:
      enabled: true
    gomplate:
      enabled: true
`

// crossAccountStackTemplate reproduces the shape reported in
// https://github.com/cloudposse/atmos/issues/2566: a `customer` component whose var is
// resolved from the `global` component's Terraform state in *another* stage's account.
// The referenced stack does not exist in this fixture, so evaluating `!terraform.state`
// fails immediately and deterministically — no network, no credentials, no timeouts —
// standing in for the AccessDenied a real cross-account state read produces.
const crossAccountStackTemplate = `vars:
  stage: %STAGE%

components:
  terraform:
    global:
      metadata:
        component: mock
      vars: {}

    customer:
      metadata:
        component: mock
      vars:
        data_bucket_name: !terraform.state global %UNREACHABLE_STACK% data_bucket_name
`

// writeCrossAccountFixture builds a throwaway multi-stage Atmos repository in a temp
// directory and returns its path. Building it at runtime (instead of committing a
// scenario under tests/fixtures) keeps the test hermetic and CWD-independent.
func writeCrossAccountFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	mustMkdirAll(t, filepath.Join(root, "components", "terraform", "mock"))
	mustMkdirAll(t, filepath.Join(root, "stacks", "deploy"))

	mustWriteFile(t, filepath.Join(root, "atmos.yaml"), crossAccountAtmosYAML)
	mustWriteFile(t, filepath.Join(root, "components", "terraform", "mock", "main.tf"),
		"output \"data_bucket_name\" {\n  value = \"mock\"\n}\n")

	// Each stage points at the *other* stage's stack, so no single set of credentials
	// could ever resolve both — exactly the reported multi-account topology.
	for stage, unreachable := range map[string]string{"dev": "prd", "prd": "dev"} {
		content := strings.NewReplacer(
			"%STAGE%", stage,
			// Reference a stack name that is not part of this fixture.
			"%UNREACHABLE_STACK%", unreachable+"-unreachable",
		).Replace(crossAccountStackTemplate)
		mustWriteFile(t, filepath.Join(root, "stacks", "deploy", stage+".yaml"), content)
	}

	return root
}

// mustMkdirAll creates path and all missing parents, failing the test on error.
func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o755))
}

// mustWriteFile writes content to path, failing the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// loadCrossAccountConfig chdir's into a freshly built cross-account fixture and returns
// its initialized Atmos configuration.
func loadCrossAccountConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()

	initExecutorTestIO(t)
	t.Chdir(writeCrossAccountFixture(t))

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, []string{}, nil)
	require.NoError(t, err)

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	return &atmosConfig
}

// newAuthenticatedManager returns a stand-in for the AuthManager that
// `createAuthManagerForList` builds when the user passes `--identity`.
func newAuthenticatedManager(t *testing.T) *authTypes.MockAuthManager {
	t.Helper()

	manager := authTypes.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().GetStackInfo().Return(nil).AnyTimes()
	return manager
}

// TestCrossAccountFixture_FailsWhenCredentialBackedFunctionsAreEvaluated is the
// prerequisite check for the regression tests below: it proves the fixture really does
// blow up when `!terraform.state` is evaluated. Without this, a fixture that quietly
// stopped exercising the YAML function would turn every assertion below into a
// vacuous pass.
func TestCrossAccountFixture_FailsWhenCredentialBackedFunctionsAreEvaluated(t *testing.T) {
	atmosConfig := loadCrossAccountConfig(t)

	_, err := e.ExecuteDescribeStacksWithAuthDisabled(
		atmosConfig, "", nil, nil, nil,
		false, // ignoreMissingFiles.
		true,  // processTemplates.
		true,  // processYamlFunctions.
		false, // includeEmptyStacks.
		nil,   // skip — nothing skipped, so !terraform.state is evaluated.
		newAuthenticatedManager(t),
		false, // authDisabled.
	)

	require.Error(t, err, "the fixture must fail when !terraform.state is evaluated, otherwise the regression tests prove nothing")
}

// TestExecuteAndExtractStacks_SkipsCredentialBackedFunctionsWithIdentity is the
// regression guard for https://github.com/cloudposse/atmos/issues/2566.
//
// `atmos list stacks --identity <one-account>` used to keep credential-backed YAML
// functions enabled whenever an identity was supplied, so the full-repository scan
// tried to read every *other* account's Terraform state and aborted with AccessDenied.
// Listing stacks never surfaces those values, so they must be skipped even when an
// identity is present.
func TestExecuteAndExtractStacks_SkipsCredentialBackedFunctionsWithIdentity(t *testing.T) {
	atmosConfig := loadCrossAccountConfig(t)

	opts := &StacksOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	// Strict error options: a credential/backend failure must stay fatal, so the test
	// cannot pass merely because upstream's --error-mode=warn degraded it to a warning.
	stacks, stacksMap, err := executeAndExtractStacks(
		atmosConfig, opts, newAuthenticatedManager(t), e.DescribeStacksErrorOptions{},
	)

	require.NoError(t, err, "listing stacks must not read Terraform state, even with an explicit --identity")
	require.Len(t, stacks, 2)
	assert.Contains(t, stacksMap, "dev")
	assert.Contains(t, stacksMap, "prd")

	names := []string{stacks[0]["stack"].(string), stacks[1]["stack"].(string)}
	assert.ElementsMatch(t, []string{"dev", "prd"}, names)
}

// TestInitAndExtractComponents_SkipsCredentialBackedFunctionsWithIdentity covers the
// same regression for `atmos list components`, the second command named in #2566.
func TestInitAndExtractComponents_SkipsCredentialBackedFunctionsWithIdentity(t *testing.T) {
	atmosConfig := loadCrossAccountConfig(t)

	opts := &ComponentsOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	// opts.ErrorMode is left empty so describeStacksErrorOptions yields strict mode: a
	// credential/backend failure must stay fatal rather than degrade to a warning.
	result, err := executeAndExtractComponents(atmosConfig, opts, newAuthenticatedManager(t))

	require.NoError(t, err, "listing components must not read Terraform state, even with an explicit --identity")
	require.NotEmpty(t, result.components)
	require.Nil(t, result.collector, "strict mode must not create a degradation collector")

	names := make([]string, 0, len(result.components))
	for _, component := range result.components {
		name, _ := component["component"].(string)
		names = append(names, name)
	}
	assert.Contains(t, names, "global")
	assert.Contains(t, names, "customer")
}

// TestExecuteAndExtractStacks_ResolvesWhenColumnsAreCustomized is the negative path for the
// demand-driven skip, and the guard for the review constraint that list output is
// customizable and must not have resolution disabled outright: once the user asks for a
// custom column, the credential-backed functions are evaluated again and the fixture's
// unreachable `!terraform.state` fails the command rather than being silently skipped.
//
// Without this, the skip could regress into an unconditional disable and every assertion
// above would still pass.
func TestExecuteAndExtractStacks_ResolvesWhenColumnsAreCustomized(t *testing.T) {
	atmosConfig := loadCrossAccountConfig(t)

	opts := &StacksOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: true,
		// A custom column can reference `.vars`, so values must still be resolved.
		Columns: []string{"Bucket={{ .vars.data_bucket_name }}"},
	}

	_, _, err := executeAndExtractStacks(
		atmosConfig, opts, newAuthenticatedManager(t), e.DescribeStacksErrorOptions{},
	)

	require.Error(t, err,
		"custom columns must keep credential-backed functions resolvable, not silently skipped")
	require.ErrorIs(t, err, errUtils.ErrDescribeComponent,
		"the failure must come from resolving !terraform.state against the unreachable stack")
}

// TestStacksOutputCanSurfaceValues pins how `list stacks` classifies its own output.
func TestStacksOutputCanSurfaceValues(t *testing.T) {
	defaultConfig := &schema.AtmosConfiguration{}

	assert.False(t, stacksOutputCanSurfaceValues(defaultConfig, &StacksOptions{}),
		"default columns render `.stack` only")
	assert.True(t, stacksOutputCanSurfaceValues(defaultConfig, &StacksOptions{Columns: []string{"B={{ .vars.b }}"}}),
		"--columns can reference .vars")

	configured := &schema.AtmosConfiguration{}
	configured.Stacks.List.Columns = []schema.ListColumnConfig{{Name: "B", Value: "{{ .vars.b }}"}}
	assert.True(t, stacksOutputCanSurfaceValues(configured, &StacksOptions{}),
		"stacks.list.columns in atmos.yaml can reference .vars")
}

// TestComponentsOutputCanSurfaceValues pins how `list components` classifies its own output.
func TestComponentsOutputCanSurfaceValues(t *testing.T) {
	defaultConfig := &schema.AtmosConfiguration{}

	assert.False(t, componentsOutputCanSurfaceValues(defaultConfig, &ComponentsOptions{}),
		"default columns render component/type/stack_count only")
	assert.True(t, componentsOutputCanSurfaceValues(defaultConfig, &ComponentsOptions{Columns: []string{"B={{ .vars.b }}"}}),
		"--columns can reference .vars")

	configured := &schema.AtmosConfiguration{}
	configured.List.Components.Columns = []schema.ListColumnConfig{{Name: "B", Value: "{{ .vars.b }}"}}
	assert.True(t, componentsOutputCanSurfaceValues(configured, &ComponentsOptions{}),
		"list.components.columns in atmos.yaml can reference .vars")
}

// TestInstancesOutputCanSurfaceValues pins how `list instances` classifies its own output:
// beyond columns it has --query, --filter and the --upload payload.
func TestInstancesOutputCanSurfaceValues(t *testing.T) {
	assert.False(t, instancesOutputCanSurfaceValues(&InstancesOptions{}))
	assert.True(t, instancesOutputCanSurfaceValues(&InstancesOptions{Columns: []string{"B={{ .vars.b }}"}}))
	assert.True(t, instancesOutputCanSurfaceValues(&InstancesOptions{Query: ".vars.b"}))
	assert.True(t, instancesOutputCanSurfaceValues(&InstancesOptions{Filter: ".vars.b == \"x\""}))
	assert.True(t, instancesOutputCanSurfaceValues(&InstancesOptions{Upload: true}),
		"--upload ships the full instance payload to Atmos Pro")
}

// TestSkipCredentialBackedYAMLFunctionsForInventory pins the skip set itself.
//
// The expected tokens are written out literally rather than derived from
// credentialBackedYAMLFunctions(): deriving them would only prove the merge preserves
// whatever the helper happens to return today, so dropping `secret`, `store.get` or an
// `aws.*` function from the helper would still pass. The tokens are bare (no leading `!`)
// because skipFunc trims the tag prefix before comparing.
func TestSkipCredentialBackedYAMLFunctionsForInventory(t *testing.T) {
	expected := []string{
		"terraform.state",
		"terraform.output",
		"store",
		"store.get",
		"secret",
		"aws.account_id",
		"aws.caller_identity_arn",
		"aws.caller_identity_user_id",
		"aws.region",
		"aws.organization_id",
	}

	t.Run("helper returns exactly the credential-backed set", func(t *testing.T) {
		assert.ElementsMatch(t, expected, credentialBackedYAMLFunctions(),
			"a function added to or removed from the helper must be reflected here deliberately")
	})

	t.Run("adds every credential-backed function when output cannot surface values", func(t *testing.T) {
		skip := skipCredentialBackedYAMLFunctionsForInventory(nil, false)

		assert.ElementsMatch(t, expected, skip)
		for _, name := range skip {
			assert.NotContains(t, name, "!", "skip tokens must be bare so skipFunc can match them")
		}
	})

	t.Run("returns the caller's skip untouched when output can surface values", func(t *testing.T) {
		caller := []string{"exec"}

		skip := skipCredentialBackedYAMLFunctionsForInventory(caller, true)

		assert.Equal(t, caller, skip,
			"custom columns/query must keep credential-backed functions resolvable")
		for _, name := range expected {
			if name == "exec" {
				continue
			}
			assert.NotContains(t, skip, name)
		}
	})

	t.Run("preserves caller-supplied skip entries without duplicating", func(t *testing.T) {
		caller := []string{"exec", "terraform.state"}

		skip := skipCredentialBackedYAMLFunctionsForInventory(caller, false)

		assert.Contains(t, skip, "exec")
		assert.Equal(t, 1, countOccurrences(skip, "terraform.state"),
			"an entry already supplied by the caller must not be added twice")
		assert.Equal(t, []string{"exec", "terraform.state"}, caller,
			"the caller's slice must not be mutated")
		assert.Subset(t, skip, expected)
	})
}

// TestListOutputCanSurfaceValues covers the predicate that decides whether resolution is
// skipped: default output is provably value-free, any customization is not.
func TestListOutputCanSurfaceValues(t *testing.T) {
	assert.False(t, listOutputCanSurfaceValues(false),
		"default columns with no extra consumers render identity fields only")
	assert.True(t, listOutputCanSurfaceValues(true),
		"custom columns can reference .vars and must resolve")
	assert.False(t, listOutputCanSurfaceValues(false, false, false),
		"explicitly false extra consumers must not enable resolution")
	assert.True(t, listOutputCanSurfaceValues(false, false, true),
		"any true extra consumer (query/filter/upload) must enable resolution")
}

// countOccurrences returns how many times target appears in values, used to prove the skip
// merge does not duplicate an entry the caller already supplied.
func countOccurrences(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

// stubListAuthManagerFactory replaces the package-level AuthManagerFactory for the test's
// duration so createAuthManagerForList produces an authenticated-looking manager without
// performing real authentication. This is the seam #2801 added for exactly this purpose —
// it lets the instances tests below drive the real executeListInstancesCmd (including its
// instancesOutputCanSurfaceValues wiring) instead of re-deriving the skip in the test,
// which would only restate the production code rather than exercise it.
func stubListAuthManagerFactory(t *testing.T) {
	t.Helper()

	ctrl := gomock.NewController(t)
	manager := newAuthenticatedManager(t)

	factory := NewMockAuthManagerFactory(ctrl)
	factory.EXPECT().
		CreateWithStackScan(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(auth.AuthManager(manager), nil).
		AnyTimes()

	original := listAuthManagerFactory
	t.Cleanup(func() { listAuthManagerFactory = original })
	listAuthManagerFactory = factory
}

// newInstancesCmd builds the instances command against the cross-account fixture with an
// explicit identity. The identity is set explicitly (rather than left to default) to
// isolate the test from Viper state established by unrelated command tests in the same
// package — the same precaution TestCreateAuthManagerForList_EvaluationPolicy takes.
func newInstancesCmd(t *testing.T, identity string) *cobra.Command {
	t.Helper()

	cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
	// The global builder may register --identity on either flag set depending on how the
	// command was assembled; add it only if absent so this never double-registers.
	if cmd.Flags().Lookup(cfg.IdentityFlagName) == nil &&
		cmd.PersistentFlags().Lookup(cfg.IdentityFlagName) == nil {
		cmd.PersistentFlags().String(cfg.IdentityFlagName, "", "identity")
	}
	require.NoError(t, cmd.Flags().Set(cfg.IdentityFlagName, identity),
		"setting --identity explicitly isolates this test from Viper state left by other tests")
	return cmd
}

// TestExecuteListInstancesCmd_SkipsCredentialBackedFunctionsByDefault extends the #2566
// regression coverage to `list instances`, driving the real command entry point against the
// cross-account fixture. With the default columns and no --query/--filter/--upload, the
// unreachable `!terraform.state` must never be evaluated even though an identity resolved.
func TestExecuteListInstancesCmd_SkipsCredentialBackedFunctionsByDefault(t *testing.T) {
	initExecutorTestIO(t)
	t.Chdir(writeCrossAccountFixture(t))
	stubListAuthManagerFactory(t)

	opts := &InstancesOptions{
		Format:           "json",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	require.NoError(t, executeListInstancesCmd(newInstancesCmd(t, "prd-access"), []string{}, opts),
		"default instances output must not read Terraform state, even with an identity")
}

// TestExecuteListInstancesCmd_ResolvesWhenValuesRequested is the negative path. `list
// instances` has the largest value-surfacing surface of the inventory commands, so each of
// --query, --filter and --columns must switch resolution back on: the fixture's unreachable
// `!terraform.state` then fails the command rather than being silently skipped.
//
// Without these, a regression to an unconditional skip would leave the test above passing.
func TestExecuteListInstancesCmd_ResolvesWhenValuesRequested(t *testing.T) {
	tests := []struct {
		name string
		opts InstancesOptions
	}{
		{name: "query", opts: InstancesOptions{Query: ".vars.data_bucket_name"}},
		{name: "filter", opts: InstancesOptions{Filter: `.vars.data_bucket_name == "x"`}},
		{name: "columns", opts: InstancesOptions{Columns: []string{"Bucket={{ .vars.data_bucket_name }}"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initExecutorTestIO(t)
			t.Chdir(writeCrossAccountFixture(t))
			stubListAuthManagerFactory(t)

			opts := tt.opts
			opts.Format = "json"
			opts.ProcessTemplates = true
			opts.ProcessFunctions = true

			err := executeListInstancesCmd(newInstancesCmd(t, "prd-access"), []string{}, &opts)

			require.Error(t, err,
				"requesting a value must keep credential-backed functions resolvable, not silently skipped")
			// Pin the cause: without this the test would also pass if the query/filter
			// itself were malformed, which would prove nothing about resolution.
			require.ErrorIs(t, err, errUtils.ErrDescribeComponent,
				"the failure must come from resolving !terraform.state against the unreachable stack")
		})
	}
}
