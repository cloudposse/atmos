package exec

import (
	"path/filepath"
	"strings"
	"sync"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// changedFilesIndex provides efficient lookup of changed files by base path.
// This reduces PathMatch operations from O(stacks × components × files) to O(stacks × components × relevant_files).
// Expected impact: 60-80% reduction in PathMatch calls.
type changedFilesIndex struct {
	// filesByBasePath maps component base paths to changed files in that path.
	filesByBasePath map[string][]string

	// allFiles contains all changed files for fallback scenarios.
	allFiles []string

	mu sync.RWMutex
}

// newChangedFilesIndex creates an index of changed files organized by base path.
// This enables efficient filtering of relevant files for each component type.
func newChangedFilesIndex(atmosConfig *schema.AtmosConfiguration, changedFiles []string) *changedFilesIndex {
	defer perf.Track(atmosConfig, "exec.newChangedFilesIndex")()

	index := &changedFilesIndex{
		filesByBasePath: make(map[string][]string),
		allFiles:        changedFiles,
	}

	// Pre-compute absolute base paths for each component type.
	normalizedBasePaths := buildNormalizedBasePaths(atmosConfig)

	// Initialize empty slices for each base path.
	for _, absPath := range normalizedBasePaths {
		index.filesByBasePath[absPath] = make([]string, 0)
	}

	// Index each changed file by its base path.
	for _, changedFile := range changedFiles {
		indexChangedFile(index, changedFile, normalizedBasePaths)
	}

	return index
}

// buildNormalizedBasePaths constructs absolute base paths for all component types.
// Only includes non-empty component base paths to avoid indexing files under the root basePath.
func buildNormalizedBasePaths(atmosConfig *schema.AtmosConfiguration) []string {
	// Collect base paths, skipping empty ones to prevent root basePath collisions.
	basePaths := make([]string, 0, 4)

	// Add terraform base path if configured.
	if atmosConfig.Components.Terraform.BasePath != "" {
		basePaths = append(basePaths, filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath))
	}

	// Add helmfile base path if configured.
	if atmosConfig.Components.Helmfile.BasePath != "" {
		basePaths = append(basePaths, filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath))
	}

	// Add packer base path if configured.
	if atmosConfig.Components.Packer.BasePath != "" {
		basePaths = append(basePaths, filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath))
	}

	// Add stacks base path if configured.
	if atmosConfig.Stacks.BasePath != "" {
		basePaths = append(basePaths, filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath))
	}

	normalizedBasePaths := make([]string, 0, len(basePaths))
	for _, basePath := range basePaths {
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			// If conversion fails, use original path.
			absPath = basePath
		}
		normalizedBasePaths = append(normalizedBasePaths, absPath)
	}

	return normalizedBasePaths
}

// indexChangedFile indexes a single changed file by finding its matching base path.
// Files that don't match any base path are not indexed (they may still be checked via
// module patterns or dependency paths, which are independent mechanisms).
func indexChangedFile(index *changedFilesIndex, changedFile string, normalizedBasePaths []string) {
	absFile, err := filepath.Abs(changedFile)
	if err != nil {
		// If conversion fails, try using original path.
		absFile = changedFile
	}

	// Find which base path this file belongs to.
	// Use filepath.Rel to properly check path boundaries, not just string prefixes.
	// This prevents sibling paths like "components/terraform" and "components/terraform-modules"
	// from colliding due to shared prefixes.
	if matchedPath := findMatchingBasePath(absFile, normalizedBasePaths); matchedPath != "" {
		index.filesByBasePath[matchedPath] = append(index.filesByBasePath[matchedPath], absFile)
	}

	// Files that don't match any base path are NOT indexed for base path checking.
	// They will still be checked via:
	// - Module pattern cache (if referenced as Terraform modules)
	// - Dependency checking (if specified in component dependencies)
	// This maintains independence between component folder checks, module checks, and dependency checks.
}

// findMatchingBasePath returns the base path that contains the given file, or empty string if none match.
func findMatchingBasePath(absFile string, normalizedBasePaths []string) string {
	for _, basePath := range normalizedBasePaths {
		if isFileInBasePath(absFile, basePath) {
			return basePath
		}
	}
	return ""
}

// isFileInBasePath checks if a file is within a base path using proper path boundary checking.
func isFileInBasePath(absFile, basePath string) bool {
	rel, err := filepath.Rel(basePath, absFile)
	if err != nil {
		return false
	}

	// File is inside basePath if:
	// - rel is "." (file is directly at basePath), OR
	// - rel doesn't start with ".." (file is within basePath, not outside or in a sibling)
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// getRelevantFiles returns changed files relevant to a specific component.
// This significantly reduces the number of PathMatch operations needed.
func (idx *changedFilesIndex) getRelevantFiles(componentType string, atmosConfig *schema.AtmosConfiguration) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var basePath string

	switch componentType {
	case cfg.TerraformComponentType:
		basePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	case cfg.HelmfileComponentType:
		basePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	case cfg.PackerComponentType:
		basePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath)
	default:
		// Unknown component type - return all files as fallback.
		return idx.allFiles
	}

	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		// If conversion fails, return all files as fallback.
		return idx.allFiles
	}

	if files, ok := idx.filesByBasePath[absBasePath]; ok {
		return files
	}

	// If base path not found in index, return all files as fallback.
	return idx.allFiles
}

// getAllFiles returns all changed files (for operations that need to check everything).
func (idx *changedFilesIndex) getAllFiles() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.allFiles
}
