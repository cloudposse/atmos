package list

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	e "github.com/cloudposse/atmos/internal/exec"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config"
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

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o755))
}

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

	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
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
		false, // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip — nothing skipped, so !terraform.state is evaluated
		newAuthenticatedManager(t),
		false, // authDisabled
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

// TestSkipCredentialBackedYAMLFunctionsForInventory pins the skip set itself: every
// credential-backed function must be present, the caller's own `--skip` entries must be
// preserved, and the tokens must be bare (no leading `!`) to match `skipFunc`, which
// trims the tag prefix before comparing.
func TestSkipCredentialBackedYAMLFunctionsForInventory(t *testing.T) {
	t.Run("adds every credential-backed function", func(t *testing.T) {
		skip := skipCredentialBackedYAMLFunctionsForInventory(nil)

		require.NotEmpty(t, skip)
		for _, name := range credentialBackedYAMLFunctions() {
			assert.Contains(t, skip, name)
			assert.NotContains(t, name, "!", "skip tokens must be bare so skipFunc can match them")
		}
	})

	t.Run("preserves caller-supplied skip entries without duplicating", func(t *testing.T) {
		caller := []string{"exec", strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!")}

		skip := skipCredentialBackedYAMLFunctionsForInventory(caller)

		assert.Contains(t, skip, "exec")
		assert.Equal(t, 1, countOccurrences(skip, strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!")),
			"an entry already supplied by the caller must not be added twice")
		assert.Equal(t, []string{"exec", strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!")}, caller,
			"the caller's slice must not be mutated")
	})
}

func countOccurrences(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
