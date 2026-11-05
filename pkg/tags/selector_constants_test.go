package tags_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/tags"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestForbiddenSelectorFunctionsMatchConstants cross-checks the literal
// deny-list in this package against the canonical AtmosYamlFunc* constants in
// pkg/utils, so a renamed function constant fails this test instead of
// silently drifting.
func TestForbiddenSelectorFunctionsMatchConstants(t *testing.T) {
	t.Parallel()

	expected := []string{
		u.AtmosYamlFuncExec,
		u.AtmosYamlFuncSecret,
		u.AtmosYamlFuncStore,
		u.AtmosYamlFuncStoreGet,
		u.AtmosYamlFuncTerraformOutput,
		u.AtmosYamlFuncTerraformState,
		u.AtmosYamlFuncAwsAccountID,
		u.AtmosYamlFuncAwsCallerIdentityArn,
		u.AtmosYamlFuncAwsCallerIdentityUserID,
		u.AtmosYamlFuncAwsRegion,
		u.AtmosYamlFuncAwsOrganizationID,
		u.AtmosYamlFuncAwsCloudFormationOutput,
		u.AtmosYamlFuncEmulator,
		u.AtmosYamlFuncRandom,
		u.AtmosYamlFuncStarlark,
	}
	if len(expected) != len(tags.ForbiddenSelectorFunctionsForTest) {
		t.Fatalf("deny-list has %d entries, constants list has %d", len(tags.ForbiddenSelectorFunctionsForTest), len(expected))
	}
	for _, name := range expected {
		if _, ok := tags.ForbiddenSelectorFunctionsForTest[name]; !ok {
			t.Fatalf("deny-list is missing %q", name)
		}
	}
}

// TestForbiddenSelectorFunctionsCompleteness guards the selector purity
// contract against drift: every YAML function registered in utils.AtmosYamlTags
// must be explicitly classified as either forbidden in selectors (requires
// authentication, process execution, or nondeterminism) or selector-safe.
// Adding a new YAML function without classifying it here fails this test,
// forcing a deliberate purity decision instead of a silent default-allow.
func TestForbiddenSelectorFunctionsCompleteness(t *testing.T) {
	t.Parallel()

	// Functions allowed to appear in metadata.tags/metadata.labels values:
	// resolvable without authentication, subprocesses, or nondeterminism.
	// They still resolve only during full evaluation (SelectorUnresolved
	// reports them as undecidable, deferring the scope decision), but their
	// presence is not a purity violation.
	selectorSafe := map[string]struct{}{
		u.AtmosYamlFuncCILabels:      {},
		u.AtmosYamlFuncTemplate:      {},
		u.AtmosYamlFuncEnv:           {},
		u.AtmosYamlFuncCEL:           {},
		u.AtmosYamlFuncGitRoot:       {},
		u.AtmosYamlFuncGitRootAlias:  {},
		u.AtmosYamlFuncGitSha:        {},
		u.AtmosYamlFuncGitBranch:     {},
		u.AtmosYamlFuncGitRef:        {},
		u.AtmosYamlFuncGitRepository: {},
		u.AtmosYamlFuncGitOwner:      {},
		u.AtmosYamlFuncGitName:       {},
		u.AtmosYamlFuncGitHost:       {},
		u.AtmosYamlFuncGitUrl:        {},
		u.AtmosYamlFuncAppend:        {},
		u.AtmosYamlFuncCwd:           {},
		u.AtmosYamlFuncUnset:         {},
		u.AtmosYamlFuncLiteral:       {},
		u.AtmosYamlFuncVersion:       {},
		u.AtmosYamlFuncTags:          {},
		u.AtmosYamlFuncLabels:        {},
		u.AtmosYamlFuncLabelsKeys:    {},
		u.AtmosYamlFuncLabelsValues:  {},
	}

	require.NotEmpty(t, u.AtmosYamlTags, "utils.AtmosYamlTags must list the registered YAML functions")
	for _, name := range u.AtmosYamlTags {
		_, forbidden := tags.ForbiddenSelectorFunctionsForTest[name]
		_, safe := selectorSafe[name]
		switch {
		case forbidden && safe:
			t.Errorf("%q is classified as both forbidden and selector-safe", name)
		case !forbidden && !safe:
			t.Errorf("%q is unclassified: add it to tags.ForbiddenSelectorFunctionsForTest if it requires auth/exec/nondeterminism, or to this test's selector-safe list otherwise", name)
		}
	}

	// Every deny-list and selector-safe entry must correspond to a registered
	// YAML function; a stale entry means the function was renamed or removed
	// upstream.
	registered := make(map[string]struct{}, len(u.AtmosYamlTags))
	for _, name := range u.AtmosYamlTags {
		registered[name] = struct{}{}
	}
	for name := range tags.ForbiddenSelectorFunctionsForTest {
		if _, ok := registered[name]; !ok {
			t.Errorf("deny-list entry %q is not registered in utils.AtmosYamlTags", name)
		}
	}
	for name := range selectorSafe {
		if _, ok := registered[name]; !ok {
			t.Errorf("selector-safe entry %q is not registered in utils.AtmosYamlTags", name)
		}
	}
}
