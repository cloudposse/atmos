package exec

import (
	"cmp"
	"slices"
	"sync"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stackAffectedResult holds the result of processing a single stack for affected detection.
type stackAffectedResult struct {
	affected []schema.Affected
	err      error
}

// findAffectedParallel processes stacks in parallel to detect affected components.
// This is a parallel implementation of findAffected that provides 40-60% performance improvement (P9.1).
// Combined with changed files indexing (P9.4), this provides 70-85% total improvement.
// The gitRepoRoot parameter is the absolute path to the git repository root, used to resolve
// relative file paths from git diff.
//
//nolint:funlen,revive // Parallel processing logic requires comprehensive setup and cleanup
func findAffectedParallel(
	currentStacks *map[string]any,
	remoteStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	stackToFilter string,
	filter AffectedFilter,
	gitRepoRoot string,
) ([]schema.Affected, error) {
	defer perf.Track(atmosConfig, "exec.findAffectedParallel")()

	// Create an index of changed files for efficient lookup.
	// This reduces PathMatch operations by 60-80%.
	// Pass gitRepoRoot to resolve relative paths from git diff correctly.
	filesIndex := newChangedFilesIndex(atmosConfig, changedFiles, gitRepoRoot)

	// Create pattern cache for component paths.
	// This eliminates repeated pattern construction.
	patternCache := newComponentPathPatternCache()

	// Create buffered channel for results.
	stacks := *currentStacks
	results := make(chan stackAffectedResult, len(stacks))
	var wg sync.WaitGroup

	// Process each stack in parallel.
	for stackName, stackSection := range stacks {
		// If `--stack` is provided, skip other stacks.
		if stackToFilter != "" && stackToFilter != stackName {
			continue
		}

		wg.Add(1)
		go func(name string, section any) {
			defer wg.Done()

			affected, err := processStackAffected(
				name,
				section,
				remoteStacks,
				currentStacks,
				atmosConfig,
				filesIndex,
				patternCache,
				includeSpaceliftAdminStacks,
				includeSettings,
				filter,
			)

			results <- stackAffectedResult{
				affected: affected,
				err:      err,
			}
		}(stackName, stackSection)
	}

	// Close results channel when all goroutines complete.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results.
	allAffected := []schema.Affected{}
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		allAffected = append(allAffected, result.affected...)
	}

	// Detect deleted components (exist in BASE/remote but not in HEAD/current).
	deletedAffected, err := detectDeletedComponents(
		remoteStacks,
		currentStacks,
		atmosConfig,
		stackToFilter,
		filter,
	)
	if err != nil {
		return nil, err
	}
	allAffected = append(allAffected, deletedAffected...)

	// Stacks finish in any order and component maps iterate in any order, so the same
	// comparison would otherwise print its results in a different order on every run.
	sortAffected(allAffected)

	return allAffected, nil
}

// sortAffected makes the order of results independent of goroutine scheduling and map
// iteration: results are grouped by stack, component types within a stack follow the
// processing order (deleted components included), and components of one type are ordered
// by name.
func sortAffected(affected []schema.Affected) {
	slices.SortStableFunc(affected, func(a, b schema.Affected) int {
		return cmp.Or(
			cmp.Compare(a.Stack, b.Stack),
			cmp.Compare(affectedTypeRank(a.ComponentType), affectedTypeRank(b.ComponentType)),
			cmp.Compare(a.ComponentType, b.ComponentType),
			cmp.Compare(a.Component, b.Component),
			cmp.Compare(a.Affected, b.Affected),
		)
	})
}

// affectedTypeOrder lists component types in the order processStackAffected processes them.
var affectedTypeOrder = slices.Concat([]string{cfg.TerraformComponentType}, simpleAffectedComponentTypes,
	[]string{cfg.KubernetesComponentType, cfg.HelmComponentType, cfg.CloudFormationComponentType})

// affectedTypeRank orders component types by affectedTypeOrder. An unknown type sorts after the
// known ones.
func affectedTypeRank(componentType string) int {
	if index := slices.Index(affectedTypeOrder, componentType); index >= 0 {
		return index
	}
	return len(affectedTypeOrder)
}

// processStackAffected processes a single stack to find affected components.
// This function is called in parallel for each stack.
//
//nolint:funlen,revive // Component type processing requires separate sections for supported component types
func processStackAffected(
	stackName string,
	stackSection any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	filter AffectedFilter,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	stackSectionMap, ok := stackSection.(map[string]any)
	if !ok {
		return affected, nil
	}

	componentsSection, ok := stackSectionMap["components"].(map[string]any)
	if !ok {
		return affected, nil
	}

	// Process Terraform components.
	if terraformSection, ok := componentsSection[cfg.TerraformComponentType].(map[string]any); ok {
		terraformAffected, err := processTerraformComponentsIndexed(
			stackName,
			terraformSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			includeSpaceliftAdminStacks,
			includeSettings,
			filter,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, terraformAffected...)
	}

	// Process the simple (non-Terraform) component types that share processSimpleComponentsIndexed:
	// helmfile, packer, ansible, container, and emulator. Terraform, Kubernetes, and Helm have
	// dedicated processors with type-specific logic and are handled separately. See #3203.
	for _, componentType := range simpleAffectedComponentTypes {
		section, ok := componentsSection[componentType].(map[string]any)
		if !ok {
			continue
		}
		simpleAffected, err := processSimpleComponentsIndexed(
			componentType,
			stackName,
			section,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			includeSpaceliftAdminStacks,
			includeSettings,
			filter,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, simpleAffected...)
	}

	// Process Kubernetes components.
	if kubernetesSection, ok := componentsSection[cfg.KubernetesComponentType].(map[string]any); ok {
		kubernetesAffected, err := processKubernetesComponentsIndexed(
			stackName,
			kubernetesSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			includeSpaceliftAdminStacks,
			includeSettings,
			filter,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, kubernetesAffected...)
	}

	// Process Helm components.
	if helmSection, ok := componentsSection[cfg.HelmComponentType].(map[string]any); ok {
		helmAffected, err := processHelmComponentsIndexed(
			stackName,
			helmSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			includeSpaceliftAdminStacks,
			includeSettings,
			filter,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, helmAffected...)
	}

	// Process aws/cloudformation components.
	if cloudFormationSection, ok := componentsSection[cfg.CloudFormationComponentType].(map[string]any); ok {
		cloudFormationAffected, err := processCloudFormationComponentsIndexed(
			stackName,
			cloudFormationSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			includeSpaceliftAdminStacks,
			includeSettings,
			filter,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, cloudFormationAffected...)
	}

	return affected, nil
}
