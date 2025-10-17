package exec

import (
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// BenchmarkFindAffectedParallel benchmarks the parallel implementation with indexing.
func BenchmarkFindAffectedParallel(b *testing.B) {
	atmosConfig, currentStacks, remoteStacks, changedFiles := setupBenchmarkData(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := findAffectedParallel(
			&currentStacks,
			&remoteStacks,
			atmosConfig,
			changedFiles,
			false, // includeSpaceliftAdminStacks
			false, // includeSettings
			"",    // stackToFilter
			false, // excludeLocked
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkChangedFilesIndexCreation benchmarks the index creation (P9.4).
func BenchmarkChangedFilesIndexCreation(b *testing.B) {
	atmosConfig, _, _, changedFiles := setupBenchmarkData(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = newChangedFilesIndex(atmosConfig, changedFiles)
	}
}

// BenchmarkIsComponentFolderChanged benchmarks original version.
func BenchmarkIsComponentFolderChanged(b *testing.B) {
	atmosConfig, _, _, changedFiles := setupBenchmarkData(b)
	component := "vpc"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := isComponentFolderChanged(
			component,
			cfg.TerraformComponentType,
			atmosConfig,
			changedFiles,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIsComponentFolderChangedIndexed benchmarks indexed version with pattern caching.
func BenchmarkIsComponentFolderChangedIndexed(b *testing.B) {
	atmosConfig, _, _, changedFiles := setupBenchmarkData(b)
	component := "vpc"
	filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
	patternCache := newComponentPathPatternCache()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := isComponentFolderChangedIndexed(
			component,
			cfg.TerraformComponentType,
			atmosConfig,
			filesIndex,
			patternCache,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// setupBenchmarkData creates test data for benchmarking.
//
//nolint:nestif // Test data construction requires nested type assertions for map hierarchy
func setupBenchmarkData(b *testing.B) (*schema.AtmosConfiguration, map[string]any, map[string]any, []string) {
	b.Helper()

	// Create a simple test atmosConfig.
	cwd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: cwd,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
	}

	// Create sample current stacks data.
	currentStacks := make(map[string]any)
	for i := 0; i < 50; i++ {
		stackName := "stack-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26))
		currentStacks[stackName] = map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{
							"enabled": true,
						},
						"component": "vpc",
						"vars": map[string]any{
							"name": "test-vpc",
						},
					},
					"eks": map[string]any{
						"metadata": map[string]any{
							"enabled": true,
						},
						"component": "eks",
						"vars": map[string]any{
							"cluster_name": "test-eks",
						},
					},
				},
			},
		}
	}

	// Create sample remote stacks data (slightly different from current).
	// Use deep copy to ensure mutations to remoteStacks don't affect currentStacks.
	// This ensures the benchmark properly tests drift detection paths.
	remoteStacks := make(map[string]any)
	for k, v := range currentStacks {
		clone, err := deepCopyAny(v)
		if err != nil {
			b.Fatal(err)
		}
		remoteStacks[k] = clone
	}

	// Modify a few stacks to create some affected components.
	if stack, ok := remoteStacks["stack-a-0"].(map[string]any); ok {
		if components, ok := stack["components"].(map[string]any); ok {
			if terraform, ok := components[cfg.TerraformComponentType].(map[string]any); ok {
				if vpc, ok := terraform["vpc"].(map[string]any); ok {
					if vars, ok := vpc["vars"].(map[string]any); ok {
						vars["name"] = "old-vpc-name" // Changed value
					}
				}
			}
		}
	}

	// Create sample changed files list.
	changedFiles := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		file := filepath.Join(cwd, "components", "terraform", "vpc", "main.tf")
		changedFiles = append(changedFiles, file)
	}

	return atmosConfig, currentStacks, remoteStacks, changedFiles
}
