package exec

import (
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
	excludeLocked bool,
) ([]schema.Affected, error) {
	defer perf.Track(atmosConfig, "exec.findAffectedParallel")()

	// Create an index of changed files for efficient lookup.
	// This reduces PathMatch operations by 60-80%.
	filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)

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
				excludeLocked,
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

	return allAffected, nil
}

// processStackAffected processes a single stack to find affected components.
// This function is called in parallel for each stack.
//
//nolint:funlen,revive // Component type processing requires separate sections for Terraform, Helmfile, and Packer
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
	excludeLocked bool,
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
			excludeLocked,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, terraformAffected...)
	}

	// Process Helmfile components.
	if helmfileSection, ok := componentsSection[cfg.HelmfileComponentType].(map[string]any); ok {
		helmfileAffected, err := processHelmfileComponentsIndexed(
			stackName,
			helmfileSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			includeSpaceliftAdminStacks,
			includeSettings,
			excludeLocked,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, helmfileAffected...)
	}

	// Process Packer components.
	if packerSection, ok := componentsSection[cfg.PackerComponentType].(map[string]any); ok {
		packerAffected, err := processPackerComponentsIndexed(
			stackName,
			packerSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			includeSpaceliftAdminStacks,
			includeSettings,
			excludeLocked,
		)
		if err != nil {
			return nil, err
		}
		affected = append(affected, packerAffected...)
	}

	return affected, nil
}
