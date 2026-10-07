package tests

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Compile-time sentinels for the schema and filter fields these tests rely on.
var (
	_ = schema.Affected{Affected: "", AffectedAll: nil, StackSlug: "", Dependents: nil, Deleted: false}
	_ = schema.Dependent{StackSlug: "", Dependents: nil}
	_ = e.AffectedFilter{ExcludeLocked: false, Tags: nil, Labels: nil, DeferSelectors: false}
	_ = e.AffectedDependentsOptions{Filter: e.AffectedFilter{}, Flatten: false}
)

const (
	selectorsFixtureName = "atmos-describe-affected-with-selectors"

	// Stack slugs (`<stack>-<component with / replaced by ->`) of the fixture components.
	slugHubUE1          = "ue1-network-tgw-hub"
	slugHubUW2          = "uw2-network-tgw-hub"
	slugAttachmentUE1   = "ue1-network-tgw-attachment"
	slugAttachmentUW2   = "uw2-network-tgw-attachment"
	slugAttachmentProd1 = "ue1-prod-tgw-attachment"
	slugAttachmentProd2 = "uw2-prod-tgw-attachment"
	slugIAM             = "ue1-network-iam"
	slugApp             = "ue1-network-app"
	slugChainBase       = "ue1-network-chain-base"
	slugChainMiddle     = "ue1-network-chain-middle"
	slugChainLeaf       = "ue1-network-chain-leaf"
	slugTemplated       = "ue1-network-templated"
	slugFlipToAuto      = "ue1-network-flip-to-auto"
	slugFlipToManual    = "ue1-network-flip-to-manual"
	slugBareMetadata    = "ue1-network-shape-bare-metadata"
	slugAWSTagsOnly     = "ue1-network-shape-aws-tags-only"
	slugMock            = "ue1-network-mock"
	slugAbstractAuto    = "ue1-network-abstract-auto"
	slugDisabledAuto    = "ue1-network-disabled-auto"
	slugAudit           = "ue1-network-audit"
	slugLegacyLocked    = "ue1-network-legacy-locked"
	slugOldManual       = "ue1-network-old-manual"
	slugDeletedNoLabel  = "ue1-network-deleted-no-metadata-label"
	slugOldAuto         = "ue1-legacy-old-auto"
	slugLegacyOldManual = "ue1-legacy-old-manual"
)

// selectorsRun describes one `describe affected` invocation against the selectors fixture.
type selectorsRun struct {
	filter            e.AffectedFilter
	includeDependents bool
	flatten           bool
}

// TestDescribeAffectedSelectors runs `describe affected` with `--tags` / `--labels` /
// `--exclude-locked` / `--include-dependents` / `--flatten` against a real stack fixture
// (HEAD = `stacks/`, BASE = `stacks-affected/`) and checks which components, dependents and
// reasons are reported.
func TestDescribeAffectedSelectors(t *testing.T) {
	RequireGitRemoteWithValidURL(t)

	basePath := filepath.Join("tests", "fixtures", "scenarios", selectorsFixtureName)
	pathPrefix := ".."

	stacksPath := filepath.Join(pathPrefix, basePath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// A second, untouched configuration for describing the HEAD stacks directly (the terraform parity check).
	headConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	tempDir := t.TempDir()

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		OnSymlink:     func(string) cp.SymlinkAction { return cp.Skip },
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if strings.Contains(src, "node_modules") ||
				strings.Contains(src, ".claude") ||
				strings.Contains(src, ".terraform") {
				return true, nil
			}
			isSocket, err := u.IsSocket(src)
			if err != nil {
				return true, err
			}
			return isSocket, nil
		},
	}

	prepareDescribeAffectedWithSelectorsBaseRepo(t, pathPrefix, tempDir, basePath, stacksPath, &copyOptions)

	// Set BasePath for the fixture.
	atmosConfig.BasePath = basePath

	run := func(t *testing.T, r selectorsRun) []schema.Affected {
		t.Helper()
		return describeAffectedWithSelectors(t, &atmosConfig, tempDir, r)
	}

	autoLabel := map[string]string{"ci": "auto"}
	manualLabel := map[string]string{"ci": "manual"}

	// Components that must never be reported for `ci=auto`: no label, no metadata, abstract, disabled,
	// flipped to manual in HEAD, and deleted ones whose BASE label is manual or missing.
	notAuto := []string{
		slugBareMetadata, slugAWSTagsOnly, slugMock, slugAbstractAuto, slugDisabledAuto,
		slugFlipToManual, slugOldManual, slugDeletedNoLabel, slugLegacyOldManual, slugIAM, slugChainMiddle,
	}

	autoTopLevel := []string{
		slugHubUE1, slugHubUW2, slugChainBase, slugTemplated, slugFlipToAuto,
		slugAudit, slugLegacyLocked, slugOldAuto, slugApp,
	}

	t.Run("labels ci=auto with dependents", func(t *testing.T) {
		affected := run(t, selectorsRun{
			filter:            e.AffectedFilter{Labels: autoLabel},
			includeDependents: true,
		})

		assert.ElementsMatch(t, autoTopLevel, topLevelSlugs(affected))
		assertNoneOf(t, topLevelSlugs(affected), notAuto)

		bySlug := indexAffected(t, affected)

		// The privileged parent `iam` is dropped, and its automated dependent is promoted.
		assert.Equal(t, "dependent", bySlug[slugApp].Affected)
		assert.Equal(t, []string{"dependent"}, bySlug[slugApp].AffectedAll)
		assert.False(t, bySlug[slugApp].Deleted)

		// Non-matching dependents are pruned and their matching dependents move up.
		assert.ElementsMatch(t, []string{slugChainLeaf}, directDependentSlugs(bySlug[slugChainBase].Dependents))

		// Every matching `tgw/attachment` stays nested under the hub it depends on.
		assert.ElementsMatch(t, []string{slugAttachmentUE1, slugAttachmentProd1}, directDependentSlugs(bySlug[slugHubUE1].Dependents))
		assert.ElementsMatch(t, []string{slugAttachmentUW2, slugAttachmentProd2}, directDependentSlugs(bySlug[slugHubUW2].Dependents))
		assert.Empty(t, directDependentSlugs(bySlug[slugApp].Dependents))

		// Dependents that were never top-level stay nested.
		topLevel := topLevelSlugs(affected)
		assertNoneOf(t, topLevel, []string{slugChainLeaf, slugAttachmentUE1, slugAttachmentUW2, slugAttachmentProd1, slugAttachmentProd2})

		// Deleted components are matched on their BASE metadata.
		for _, slug := range []string{slugAudit, slugLegacyLocked, slugOldAuto} {
			assert.True(t, bySlug[slug].Deleted, "%s should be reported as deleted", slug)
		}

		// A component that is modified and matches keeps its own reason.
		assert.NotEqual(t, "dependent", bySlug[slugHubUE1].Affected)
		assert.NotEmpty(t, bySlug[slugHubUE1].AffectedAll)
	})

	t.Run("labels ci=auto with dependents and exclude-locked", func(t *testing.T) {
		affected := run(t, selectorsRun{
			filter:            e.AffectedFilter{Labels: autoLabel, ExcludeLocked: true},
			includeDependents: true,
		})

		expected := without(autoTopLevel, slugLegacyLocked)
		assert.ElementsMatch(t, expected, topLevelSlugs(affected))
		assertNoneOf(t, topLevelSlugs(affected), append([]string{slugLegacyLocked}, notAuto...))

		bySlug := indexAffected(t, affected)
		assert.Equal(t, "dependent", bySlug[slugApp].Affected)
		assert.ElementsMatch(t, []string{slugChainLeaf}, directDependentSlugs(bySlug[slugChainBase].Dependents))
		assert.ElementsMatch(t, []string{slugAttachmentUE1, slugAttachmentProd1}, directDependentSlugs(bySlug[slugHubUE1].Dependents))
		assert.ElementsMatch(t, []string{slugAttachmentUW2, slugAttachmentProd2}, directDependentSlugs(bySlug[slugHubUW2].Dependents))
		assert.True(t, bySlug[slugAudit].Deleted)
	})

	t.Run("labels ci=auto with dependents and flatten", func(t *testing.T) {
		affected := run(t, selectorsRun{
			filter:            e.AffectedFilter{Labels: autoLabel},
			includeDependents: true,
			flatten:           true,
		})

		expected := append(append([]string{}, autoTopLevel...),
			slugChainLeaf, slugAttachmentUE1, slugAttachmentUW2, slugAttachmentProd1, slugAttachmentProd2)
		assert.ElementsMatch(t, expected, topLevelSlugs(affected))
		assertNoneOf(t, topLevelSlugs(affected), notAuto)

		// indexAffected fails the test on a duplicate slug.
		bySlug := indexAffected(t, affected)

		for _, slug := range []string{slugChainLeaf, slugApp, slugAttachmentUE1, slugAttachmentUW2, slugAttachmentProd1, slugAttachmentProd2} {
			require.Contains(t, bySlug, slug)
			assert.Equal(t, "dependent", bySlug[slug].Affected, "%s should be reported as a dependent", slug)
			assert.Equal(t, []string{"dependent"}, bySlug[slug].AffectedAll, "%s should carry the dependent reason", slug)
		}
		for _, slug := range []string{slugHubUE1, slugHubUW2, slugChainBase, slugTemplated, slugFlipToAuto} {
			assert.NotEqual(t, "dependent", bySlug[slug].Affected, "%s changed itself and should keep its own reason", slug)
		}

		for i := range affected {
			assert.Empty(t, affected[i].Dependents, "%s should have no nested dependents once flattened", affected[i].StackSlug)
		}
	})

	t.Run("labels ci=manual with dependents", func(t *testing.T) {
		affected := run(t, selectorsRun{
			filter:            e.AffectedFilter{Labels: manualLabel},
			includeDependents: true,
		})

		// `chain/base` is automated and so dropped, which promotes its manual dependent `chain/middle`.
		assert.ElementsMatch(t,
			[]string{slugIAM, slugFlipToManual, slugOldManual, slugLegacyOldManual, slugChainMiddle},
			topLevelSlugs(affected))
		assertNoneOf(t, topLevelSlugs(affected), []string{slugApp, slugChainBase, slugChainLeaf})

		// `app` is automated, so it is pruned from the manual parent instead of being kept as a dependent.
		bySlug := indexAffected(t, affected)
		assert.Empty(t, directDependentSlugs(bySlug[slugIAM].Dependents))
		assert.NotEqual(t, "dependent", bySlug[slugIAM].Affected)

		// The promoted `chain/middle` is reported as a dependent, and its automated `chain/leaf` is pruned.
		assert.Equal(t, "dependent", bySlug[slugChainMiddle].Affected)
		assert.Equal(t, []string{"dependent"}, bySlug[slugChainMiddle].AffectedAll)
		assert.Empty(t, directDependentSlugs(bySlug[slugChainMiddle].Dependents))
	})

	t.Run("tags security", func(t *testing.T) {
		affected := run(t, selectorsRun{filter: e.AffectedFilter{Tags: []string{"security"}}})

		assert.ElementsMatch(t, []string{slugIAM, slugAudit}, topLevelSlugs(affected))

		bySlug := indexAffected(t, affected)
		assert.True(t, bySlug[slugAudit].Deleted)
		assert.False(t, bySlug[slugIAM].Deleted)
	})

	t.Run("tags security with dependents", func(t *testing.T) {
		affected := run(t, selectorsRun{
			filter:            e.AffectedFilter{Tags: []string{"security"}},
			includeDependents: true,
		})

		// `app` is tagged `workload`, so it is neither nested under `iam` nor promoted.
		assert.ElementsMatch(t, []string{slugIAM, slugAudit}, topLevelSlugs(affected))
		assert.Empty(t, directDependentSlugs(indexAffected(t, affected)[slugIAM].Dependents))
	})

	t.Run("no selectors with dependents", func(t *testing.T) {
		affected := run(t, selectorsRun{includeDependents: true})

		unfiltered := []string{
			slugHubUE1, slugHubUW2, slugIAM, slugChainBase, slugBareMetadata, slugAWSTagsOnly, slugMock,
			slugTemplated, slugFlipToAuto, slugFlipToManual,
			slugAudit, slugLegacyLocked, slugOldManual, slugDeletedNoLabel, slugOldAuto, slugLegacyOldManual,
		}
		require.Len(t, unfiltered, 16)
		assert.ElementsMatch(t, unfiltered, topLevelSlugs(affected))
		assertNoneOf(t, topLevelSlugs(affected), []string{slugAbstractAuto, slugDisabledAuto, slugApp, slugChainMiddle, slugChainLeaf})

		bySlug := indexAffected(t, affected)

		// Without selectors nothing is pruned or promoted: `app` stays nested under `iam`.
		assert.ElementsMatch(t, []string{slugApp}, directDependentSlugs(bySlug[slugIAM].Dependents))
		assert.ElementsMatch(t, []string{slugChainMiddle}, directDependentSlugs(bySlug[slugChainBase].Dependents))
		assert.ElementsMatch(t,
			[]string{slugChainLeaf},
			directDependentSlugs(bySlug[slugChainBase].Dependents[0].Dependents))
		assert.ElementsMatch(t, []string{slugAttachmentUE1, slugAttachmentProd1}, directDependentSlugs(bySlug[slugHubUE1].Dependents))
		assert.ElementsMatch(t, []string{slugAttachmentUW2, slugAttachmentProd2}, directDependentSlugs(bySlug[slugHubUW2].Dependents))
	})

	// `atmos terraform plan --affected --include-dependents --labels=ci=auto` must run the same live
	// components as `atmos describe affected --include-dependents --flatten --labels=ci=auto`.
	t.Run("terraform parity with flattened describe affected", func(t *testing.T) {
		flattened := run(t, selectorsRun{
			filter:            e.AffectedFilter{Labels: autoLabel},
			includeDependents: true,
			flatten:           true,
		})

		// The unfiltered affected set is what `terraform --affected` seeds the graph with.
		unfiltered := run(t, selectorsRun{})

		stacks, err := e.ExecuteDescribeStacks(&headConfig, "", nil, nil, nil, false, true, true, false, nil, nil)
		require.NoError(t, err)

		graph, err := adapters.BuildTerraformGraph(stacks)
		require.NoError(t, err)

		var seedIDs []string
		for i := range unfiltered {
			if unfiltered[i].Deleted {
				continue
			}
			for id, node := range graph.Nodes {
				if node.Stack == unfiltered[i].Stack && node.Component == unfiltered[i].Component {
					seedIDs = append(seedIDs, id)
				}
			}
		}
		require.NotEmpty(t, seedIDs)

		info := &schema.ConfigAndStacksInfo{
			SubCommand:        "plan",
			Labels:            autoLabel,
			IncludeDependents: -1,
		}
		filtered, err := adapters.FilterTerraformGraph(&headConfig, graph, info, &adapters.TerraformSelection{NodeIDs: seedIDs})
		require.NoError(t, err)

		var terraformRuns []string
		for _, node := range filtered.Nodes {
			terraformRuns = append(terraformRuns, node.Stack+"/"+node.Component)
		}
		sort.Strings(terraformRuns)

		var describeRuns []string
		for i := range flattened {
			if flattened[i].Deleted {
				continue
			}
			describeRuns = append(describeRuns, flattened[i].Stack+"/"+flattened[i].Component)
		}
		sort.Strings(describeRuns)

		require.NotEmpty(t, describeRuns)
		assert.Equal(t, describeRuns, terraformRuns)
	})
}

// describeAffectedWithSelectors mirrors what `atmos describe affected` does for a repo path: it computes
// the affected set (deferring the selectors when dependents are requested) and then resolves the
// dependents, applies the selectors and optionally flattens.
func describeAffectedWithSelectors(t *testing.T, atmosConfig *schema.AtmosConfiguration, repoPath string, r selectorsRun) []schema.Affected {
	t.Helper()

	hasSelectors := len(r.filter.Tags) > 0 || len(r.filter.Labels) > 0
	findFilter := r.filter
	findFilter.DeferSelectors = r.includeDependents && hasSelectors

	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPathWithOptions(
		atmosConfig,
		repoPath,
		false, // includeSpaceliftAdminStacks
		false, // includeSettings
		"",    // stack
		true,  // processTemplates
		true,  // processYamlFunctions
		nil,   // skip
		findFilter,
		nil, // authManager
		false,
		e.DescribeStacksErrorOptions{},
	)
	require.NoError(t, err)

	if r.includeDependents && len(affected) > 0 {
		err = e.FinalizeAffectedDependents(atmosConfig, &affected, &e.AffectedDependentsOptions{
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Filter:               r.filter,
			Flatten:              r.flatten,
		})
		require.NoError(t, err)
	}

	return affected
}

// topLevelSlugs returns the stack slugs of the top-level affected entries, sorted for stable failure output.
func topLevelSlugs(affected []schema.Affected) []string {
	slugs := make([]string, 0, len(affected))
	for i := range affected {
		slugs = append(slugs, affected[i].StackSlug)
	}
	sort.Strings(slugs)
	return slugs
}

// directDependentSlugs returns the stack slugs of the given dependents (one level, not recursive).
func directDependentSlugs(dependents []schema.Dependent) []string {
	slugs := make([]string, 0, len(dependents))
	for i := range dependents {
		slugs = append(slugs, dependents[i].StackSlug)
	}
	sort.Strings(slugs)
	return slugs
}

// indexAffected indexes the top-level entries by stack slug and fails the test on a duplicate.
func indexAffected(t *testing.T, affected []schema.Affected) map[string]schema.Affected {
	t.Helper()

	bySlug := make(map[string]schema.Affected, len(affected))
	for i := range affected {
		slug := affected[i].StackSlug
		_, duplicate := bySlug[slug]
		require.False(t, duplicate, "%s is reported more than once at the top level", slug)
		bySlug[slug] = affected[i]
	}
	return bySlug
}

// assertNoneOf fails for every unwanted slug that is present in slugs.
func assertNoneOf(t *testing.T, slugs, unwanted []string) {
	t.Helper()

	present := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		present[slug] = true
	}
	for _, slug := range unwanted {
		assert.False(t, present[slug], "%s must not be reported at the top level", slug)
	}
}

// without returns a copy of slugs with the given slug removed.
func without(slugs []string, remove string) []string {
	out := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		if slug != remove {
			out = append(out, slug)
		}
	}
	return out
}

// prepareDescribeAffectedWithSelectorsBaseRepo builds the BASE repo for the selectors fixture: the mock
// component, the fixture atmos.yaml, and `stacks-affected` copied over `stacks`, committed with a remote.
// The fixture has no `config` directory, so none is copied.
func prepareDescribeAffectedWithSelectorsBaseRepo(
	t *testing.T,
	sourceRoot string,
	tempDir string,
	basePath string,
	stacksPath string,
	copyOptions *cp.Options,
) {
	t.Helper()

	copyFixturePath(
		t,
		filepath.Join(sourceRoot, "tests", "fixtures", "components"),
		filepath.Join(tempDir, "tests", "fixtures", "components"),
		copyOptions,
	)
	copyFixturePath(
		t,
		filepath.Join(stacksPath, "atmos.yaml"),
		filepath.Join(tempDir, basePath, "atmos.yaml"),
		copyOptions,
	)

	// Copy the BASE stacks over the normal stacks path in the temp repo. This simulates a base ref
	// with different manifests without copying the full checkout.
	copyFixturePath(
		t,
		filepath.Join(stacksPath, "stacks-affected"),
		filepath.Join(tempDir, basePath, "stacks"),
		copyOptions,
	)

	repo, err := git.PlainInit(tempDir, false)
	require.NoError(t, err)

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/cloudposse/atmos.git"},
	})
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	_, err = worktree.Add(".")
	require.NoError(t, err)

	_, err = worktree.Commit("base fixture", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Atmos Tests",
			Email: "test@example.com",
			When:  time.Unix(0, 0),
		},
	})
	require.NoError(t, err)
}
