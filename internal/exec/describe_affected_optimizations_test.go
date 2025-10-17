//nolint:gocritic // Test file uses filepath.Join with path separators for cross-platform compatibility
package exec

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ==============================================================================
// P9.2: Pattern Cache Tests
// ==============================================================================

func TestComponentPathPatternCache_GetComponentPathPattern(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
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
	}

	cache := newComponentPathPatternCache()

	t.Run("terraform component pattern", func(t *testing.T) {
		pattern, err := cache.getComponentPathPattern("vpc", cfg.TerraformComponentType, atmosConfig)
		require.NoError(t, err)
		assert.Contains(t, pattern, "components/terraform/vpc")
		assert.Contains(t, pattern, "/**")
	})

	t.Run("helmfile component pattern", func(t *testing.T) {
		pattern, err := cache.getComponentPathPattern("app", cfg.HelmfileComponentType, atmosConfig)
		require.NoError(t, err)
		assert.Contains(t, pattern, "components/helmfile/app")
		assert.Contains(t, pattern, "/**")
	})

	t.Run("packer component pattern", func(t *testing.T) {
		pattern, err := cache.getComponentPathPattern("image", cfg.PackerComponentType, atmosConfig)
		require.NoError(t, err)
		assert.Contains(t, pattern, "components/packer/image")
		assert.Contains(t, pattern, "/**")
	})

	t.Run("unsupported component type", func(t *testing.T) {
		_, err := cache.getComponentPathPattern("test", "unknown", atmosConfig)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedComponentType)
	})

	t.Run("cache stores patterns", func(t *testing.T) {
		// First call.
		pattern1, err := cache.getComponentPathPattern("vpc", cfg.TerraformComponentType, atmosConfig)
		require.NoError(t, err)

		// Second call should return cached value.
		pattern2, err := cache.getComponentPathPattern("vpc", cfg.TerraformComponentType, atmosConfig)
		require.NoError(t, err)

		assert.Equal(t, pattern1, pattern2)
	})
}

func TestComponentPathPatternCache_ThreadSafety(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	cache := newComponentPathPatternCache()

	// Run concurrent reads and writes.
	var wg sync.WaitGroup
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			component := "vpc"
			_, err := cache.getComponentPathPattern(component, cfg.TerraformComponentType, atmosConfig)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// Verify cache has the pattern.
	pattern, err := cache.getComponentPathPattern("vpc", cfg.TerraformComponentType, atmosConfig)
	require.NoError(t, err)
	assert.NotEmpty(t, pattern)
}

func TestComponentPathPatternCache_Clear(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	cache := newComponentPathPatternCache()

	// Add patterns.
	_, err := cache.getComponentPathPattern("vpc", cfg.TerraformComponentType, atmosConfig)
	require.NoError(t, err)

	// Clear cache.
	cache.clear()

	// Verify cache is empty.
	cache.mu.RLock()
	assert.Equal(t, 0, len(cache.patterns))
	assert.Equal(t, 0, len(cache.modulePatterns))
	cache.mu.RUnlock()
}

// ==============================================================================
// P9.5: Custom Deep Comparison Tests
// ==============================================================================

func TestDeepEqualMaps(t *testing.T) {
	t.Run("equal empty maps", func(t *testing.T) {
		a := map[string]any{}
		b := map[string]any{}
		assert.True(t, deepEqualMaps(a, b))
	})

	t.Run("equal simple maps", func(t *testing.T) {
		a := map[string]any{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		}
		b := map[string]any{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		}
		assert.True(t, deepEqualMaps(a, b))
	})

	t.Run("different lengths", func(t *testing.T) {
		a := map[string]any{"key1": "value1"}
		b := map[string]any{"key1": "value1", "key2": "value2"}
		assert.False(t, deepEqualMaps(a, b))
	})

	t.Run("different values", func(t *testing.T) {
		a := map[string]any{"key1": "value1"}
		b := map[string]any{"key1": "value2"}
		assert.False(t, deepEqualMaps(a, b))
	})

	t.Run("missing key", func(t *testing.T) {
		a := map[string]any{"key1": "value1"}
		b := map[string]any{"key2": "value1"}
		assert.False(t, deepEqualMaps(a, b))
	})

	t.Run("nested maps equal", func(t *testing.T) {
		a := map[string]any{
			"nested": map[string]any{
				"inner": "value",
			},
		}
		b := map[string]any{
			"nested": map[string]any{
				"inner": "value",
			},
		}
		assert.True(t, deepEqualMaps(a, b))
	})

	t.Run("nested maps different", func(t *testing.T) {
		a := map[string]any{
			"nested": map[string]any{
				"inner": "value1",
			},
		}
		b := map[string]any{
			"nested": map[string]any{
				"inner": "value2",
			},
		}
		assert.False(t, deepEqualMaps(a, b))
	})

	// Nil vs empty map tests - critical for correct affected detection.
	t.Run("both nil maps are equal", func(t *testing.T) {
		var a map[string]any
		var b map[string]any
		assert.True(t, deepEqualMaps(a, b))
	})

	t.Run("nil map vs empty map are different", func(t *testing.T) {
		var a map[string]any // nil
		b := map[string]any{}
		assert.False(t, deepEqualMaps(a, b))
	})

	t.Run("empty map vs nil map are different", func(t *testing.T) {
		a := map[string]any{}
		var b map[string]any // nil
		assert.False(t, deepEqualMaps(a, b))
	})

	t.Run("nil map vs non-empty map are different", func(t *testing.T) {
		var a map[string]any // nil
		b := map[string]any{"key": "value"}
		assert.False(t, deepEqualMaps(a, b))
	})

	t.Run("non-empty map vs nil map are different", func(t *testing.T) {
		a := map[string]any{"key": "value"}
		var b map[string]any // nil
		assert.False(t, deepEqualMaps(a, b))
	})
}

func TestDeepEqualValues(t *testing.T) {
	t.Run("nil values", func(t *testing.T) {
		assert.True(t, deepEqualValues(nil, nil))
		assert.False(t, deepEqualValues(nil, "value"))
		assert.False(t, deepEqualValues("value", nil))
	})

	t.Run("string values", func(t *testing.T) {
		assert.True(t, deepEqualValues("test", "test"))
		assert.False(t, deepEqualValues("test1", "test2"))
	})

	t.Run("int values", func(t *testing.T) {
		assert.True(t, deepEqualValues(42, 42))
		assert.False(t, deepEqualValues(42, 43))
	})

	t.Run("int64 values", func(t *testing.T) {
		assert.True(t, deepEqualValues(int64(42), int64(42)))
		assert.False(t, deepEqualValues(int64(42), int64(43)))
	})

	t.Run("float64 values", func(t *testing.T) {
		assert.True(t, deepEqualValues(3.14, 3.14))
		assert.False(t, deepEqualValues(3.14, 3.15))
	})

	t.Run("bool values", func(t *testing.T) {
		assert.True(t, deepEqualValues(true, true))
		assert.True(t, deepEqualValues(false, false))
		assert.False(t, deepEqualValues(true, false))
	})

	t.Run("type mismatch", func(t *testing.T) {
		assert.False(t, deepEqualValues("42", 42))
		assert.False(t, deepEqualValues(42, int64(42)))
	})

	t.Run("slices equal", func(t *testing.T) {
		a := []any{"a", "b", "c"}
		b := []any{"a", "b", "c"}
		assert.True(t, deepEqualValues(a, b))
	})

	t.Run("slices different", func(t *testing.T) {
		a := []any{"a", "b", "c"}
		b := []any{"a", "b", "d"}
		assert.False(t, deepEqualValues(a, b))
	})
}

func TestDeepEqualSlices(t *testing.T) {
	t.Run("equal empty slices", func(t *testing.T) {
		a := []any{}
		b := []any{}
		assert.True(t, deepEqualSlices(a, b))
	})

	t.Run("equal slices", func(t *testing.T) {
		a := []any{1, "two", 3.0, true}
		b := []any{1, "two", 3.0, true}
		assert.True(t, deepEqualSlices(a, b))
	})

	t.Run("different lengths", func(t *testing.T) {
		a := []any{1, 2}
		b := []any{1, 2, 3}
		assert.False(t, deepEqualSlices(a, b))
	})

	t.Run("different values", func(t *testing.T) {
		a := []any{1, 2, 3}
		b := []any{1, 2, 4}
		assert.False(t, deepEqualSlices(a, b))
	})

	t.Run("nested slices equal", func(t *testing.T) {
		a := []any{[]any{1, 2}, []any{3, 4}}
		b := []any{[]any{1, 2}, []any{3, 4}}
		assert.True(t, deepEqualSlices(a, b))
	})

	t.Run("nested slices different", func(t *testing.T) {
		a := []any{[]any{1, 2}, []any{3, 4}}
		b := []any{[]any{1, 2}, []any{3, 5}}
		assert.False(t, deepEqualSlices(a, b))
	})

	// Nil vs empty slice tests - critical for correct affected detection.
	t.Run("both nil slices are equal", func(t *testing.T) {
		var a []any
		var b []any
		assert.True(t, deepEqualSlices(a, b))
	})

	t.Run("nil slice vs empty slice are different", func(t *testing.T) {
		var a []any // nil
		b := []any{}
		assert.False(t, deepEqualSlices(a, b))
	})

	t.Run("empty slice vs nil slice are different", func(t *testing.T) {
		a := []any{}
		var b []any // nil
		assert.False(t, deepEqualSlices(a, b))
	})

	t.Run("nil slice vs non-empty slice are different", func(t *testing.T) {
		var a []any // nil
		b := []any{"value"}
		assert.False(t, deepEqualSlices(a, b))
	})

	t.Run("non-empty slice vs nil slice are different", func(t *testing.T) {
		a := []any{"value"}
		var b []any // nil
		assert.False(t, deepEqualSlices(a, b))
	})
}

func TestIsEqual_CustomComparison(t *testing.T) {
	t.Run("equal sections", func(t *testing.T) {
		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{
								"cidr": "10.0.0.0/16",
								"name": "dev-vpc",
							},
						},
					},
				},
			},
		}

		localSection := map[string]any{
			"cidr": "10.0.0.0/16",
			"name": "dev-vpc",
		}

		result := isEqual(remoteStacks, "dev-stack", "terraform", "vpc", localSection, "vars")
		assert.True(t, result)
	})

	t.Run("different sections", func(t *testing.T) {
		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{
								"cidr": "10.0.0.0/16",
							},
						},
					},
				},
			},
		}

		localSection := map[string]any{
			"cidr": "10.1.0.0/16", // Different value
		}

		result := isEqual(remoteStacks, "dev-stack", "terraform", "vpc", localSection, "vars")
		assert.False(t, result)
	})

	t.Run("section not found", func(t *testing.T) {
		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{},
				},
			},
		}

		localSection := map[string]any{
			"cidr": "10.0.0.0/16",
		}

		result := isEqual(remoteStacks, "dev-stack", "terraform", "vpc", localSection, "vars")
		assert.False(t, result)
	})

	t.Run("complex nested structures", func(t *testing.T) {
		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{
								"subnets": []any{
									map[string]any{"cidr": "10.0.1.0/24", "az": "us-east-1a"},
									map[string]any{"cidr": "10.0.2.0/24", "az": "us-east-1b"},
								},
								"tags": map[string]any{
									"Environment": "dev",
									"Managed":     true,
								},
							},
						},
					},
				},
			},
		}

		localSection := map[string]any{
			"subnets": []any{
				map[string]any{"cidr": "10.0.1.0/24", "az": "us-east-1a"},
				map[string]any{"cidr": "10.0.2.0/24", "az": "us-east-1b"},
			},
			"tags": map[string]any{
				"Environment": "dev",
				"Managed":     true,
			},
		}

		result := isEqual(remoteStacks, "dev-stack", "terraform", "vpc", localSection, "vars")
		assert.True(t, result)
	})
}

// ==============================================================================
// P9.7: Terraform Module Pattern Caching Tests
// ==============================================================================

func TestComponentPathPatternCache_GetTerraformModulePatterns(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test Terraform component with modules.
	componentPath := filepath.Join(tempDir, "components/terraform/vpc")
	err := os.MkdirAll(componentPath, 0o755)
	require.NoError(t, err)

	// Create main.tf with module references.
	mainTf := `
module "subnets" {
  source = "./modules/subnets"
}

module "remote_module" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "3.0.0"
}
`
	err = os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte(mainTf), 0o644)
	require.NoError(t, err)

	// Create local module directory.
	modulePath := filepath.Join(componentPath, "modules/subnets")
	err = os.MkdirAll(modulePath, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(modulePath, "main.tf"), []byte("# module"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	cache := newComponentPathPatternCache()

	t.Run("get module patterns", func(t *testing.T) {
		patterns, err := cache.getTerraformModulePatterns("vpc", atmosConfig)
		require.NoError(t, err)

		// Should have one pattern for local module (remote module excluded).
		assert.NotEmpty(t, patterns)
		assert.Contains(t, patterns[0], "modules/subnets")
		assert.Contains(t, patterns[0], "/**")
	})

	t.Run("cache stores patterns", func(t *testing.T) {
		// First call.
		patterns1, err := cache.getTerraformModulePatterns("vpc", atmosConfig)
		require.NoError(t, err)

		// Second call should return cached value.
		patterns2, err := cache.getTerraformModulePatterns("vpc", atmosConfig)
		require.NoError(t, err)

		assert.Equal(t, patterns1, patterns2)
	})

	t.Run("non-existent component returns empty", func(t *testing.T) {
		patterns, err := cache.getTerraformModulePatterns("nonexistent", atmosConfig)
		require.NoError(t, err)
		assert.Empty(t, patterns)
	})
}

func TestComponentPathPatternCache_ModulePatternsThreadSafety(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test component.
	componentPath := filepath.Join(tempDir, "components/terraform/vpc")
	err := os.MkdirAll(componentPath, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte("# test"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	cache := newComponentPathPatternCache()

	// Run concurrent reads and writes.
	var wg sync.WaitGroup
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.getTerraformModulePatterns("vpc", atmosConfig)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()
}

// ==============================================================================
// P9.4: Changed Files Index Tests
// ==============================================================================

// TestChangedFilesIndex_EmptyBasePaths is a regression test for the bug where empty component
// base paths would cause files to be incorrectly indexed under the root basePath.
// This test verifies that buildNormalizedBasePaths correctly filters out empty base paths,
// preventing file indexing collisions.
func TestChangedFilesIndex_EmptyBasePaths(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("empty packer and stacks base paths are filtered out", func(t *testing.T) {
		// This reproduces the original bug: Packer.BasePath and Stacks.BasePath are empty,
		// which would cause filepath.Join(basePath, "") to return just basePath,
		// creating duplicate entries in the normalized paths.
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
				Helmfile: schema.Helmfile{
					BasePath: "components/helmfile",
				},
				Packer: schema.Packer{
					BasePath: "", // Empty - should be filtered out
				},
			},
			Stacks: schema.Stacks{
				BasePath: "", // Empty - should be filtered out
			},
		}

		// Create test files in configured component paths.
		helmfilePath := filepath.Join(tempDir, "components/helmfile/app")
		err := os.MkdirAll(helmfilePath, 0o755)
		require.NoError(t, err)
		helmfileFile := filepath.Join(helmfilePath, "helmfile.yaml")
		err = os.WriteFile(helmfileFile, []byte("releases: []"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{helmfileFile}
		index := newChangedFilesIndex(atmosConfig, changedFiles)

		// Verify the file is correctly indexed under the helmfile base path,
		// NOT under the root basePath.
		helmFiles := index.getRelevantFiles(cfg.HelmfileComponentType, atmosConfig)
		assert.Len(t, helmFiles, 1, "helmfile file should be indexed")
		assert.Contains(t, helmFiles, helmfileFile)

		// Verify the index doesn't have the root basePath as a key.
		index.mu.RLock()
		rootBasePath, _ := filepath.Abs(tempDir)
		_, hasRootPath := index.filesByBasePath[rootBasePath]
		index.mu.RUnlock()
		assert.False(t, hasRootPath, "root basePath should NOT be in index (empty component paths should be filtered)")
	})

	t.Run("file detection works with empty base paths", func(t *testing.T) {
		// Verify that file-only changes are detected even when some component types
		// have empty base paths.
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "", // Empty
				},
				Helmfile: schema.Helmfile{
					BasePath: "components/helmfile",
				},
				Packer: schema.Packer{
					BasePath: "", // Empty
				},
			},
			Stacks: schema.Stacks{
				BasePath: "", // Empty
			},
		}

		// Create helmfile component.
		helmfilePath := filepath.Join(tempDir, "components/helmfile/app")
		err := os.MkdirAll(helmfilePath, 0o755)
		require.NoError(t, err)
		helmfileFile := filepath.Join(helmfilePath, "helmfile.yaml")
		err = os.WriteFile(helmfileFile, []byte("releases: []"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{helmfileFile}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		// Test that component folder change detection works.
		changed, err := isComponentFolderChangedIndexed("app", cfg.HelmfileComponentType, atmosConfig, filesIndex, patternCache)
		require.NoError(t, err)
		assert.True(t, changed, "file change should be detected for component with configured base path")
	})

	t.Run("all base paths empty returns empty index", func(t *testing.T) {
		// Edge case: all component types have empty base paths.
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "",
				},
				Helmfile: schema.Helmfile{
					BasePath: "",
				},
				Packer: schema.Packer{
					BasePath: "",
				},
			},
			Stacks: schema.Stacks{
				BasePath: "",
			},
		}

		changedFiles := []string{
			filepath.Join(tempDir, "some/file.txt"),
		}

		index := newChangedFilesIndex(atmosConfig, changedFiles)

		// Verify no base paths were indexed.
		index.mu.RLock()
		indexSize := len(index.filesByBasePath)
		index.mu.RUnlock()

		assert.Equal(t, 0, indexSize, "index should be empty when all base paths are empty")

		// getAllFiles should still work (fallback behavior).
		allFiles := index.getAllFiles()
		assert.Equal(t, changedFiles, allFiles)
	})
}

// TestBuildNormalizedBasePaths tests the buildNormalizedBasePaths function directly.
func TestBuildNormalizedBasePaths(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("filters out empty base paths", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
				Helmfile: schema.Helmfile{
					BasePath: "", // Empty - should be filtered
				},
				Packer: schema.Packer{
					BasePath: "components/packer",
				},
			},
			Stacks: schema.Stacks{
				BasePath: "", // Empty - should be filtered
			},
		}

		paths := buildNormalizedBasePaths(atmosConfig)

		// Should only have 2 paths (terraform and packer).
		assert.Len(t, paths, 2, "should only include non-empty base paths")

		// Convert to set for easier checking.
		pathSet := make(map[string]bool)
		for _, p := range paths {
			pathSet[p] = true
		}

		// Check that configured paths are present.
		tfPath, _ := filepath.Abs(filepath.Join(tempDir, "components/terraform"))
		packerPath, _ := filepath.Abs(filepath.Join(tempDir, "components/packer"))

		assert.True(t, pathSet[tfPath], "terraform path should be included")
		assert.True(t, pathSet[packerPath], "packer path should be included")

		// Check that root basePath is NOT included.
		rootPath, _ := filepath.Abs(tempDir)
		assert.False(t, pathSet[rootPath], "root basePath should NOT be included")
	})

	t.Run("all base paths configured", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
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

		paths := buildNormalizedBasePaths(atmosConfig)

		// Should have all 4 paths.
		assert.Len(t, paths, 4, "should include all configured base paths")
	})

	t.Run("no base paths configured", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "",
				},
				Helmfile: schema.Helmfile{
					BasePath: "",
				},
				Packer: schema.Packer{
					BasePath: "",
				},
			},
			Stacks: schema.Stacks{
				BasePath: "",
			},
		}

		paths := buildNormalizedBasePaths(atmosConfig)

		// Should be empty.
		assert.Len(t, paths, 0, "should be empty when all base paths are empty")
	})

	t.Run("paths are absolute", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		paths := buildNormalizedBasePaths(atmosConfig)

		require.Len(t, paths, 1)
		assert.True(t, filepath.IsAbs(paths[0]), "returned paths should be absolute")
	})

	t.Run("no duplicates", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
				Helmfile: schema.Helmfile{
					BasePath: "components/helmfile",
				},
			},
			Stacks: schema.Stacks{
				BasePath: "stacks",
			},
		}

		paths := buildNormalizedBasePaths(atmosConfig)

		// Check for duplicates.
		seen := make(map[string]bool)
		for _, path := range paths {
			assert.False(t, seen[path], "path %s should not be duplicated", path)
			seen[path] = true
		}
	})
}

func TestChangedFilesIndex_GetRelevantFiles(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
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

	changedFiles := []string{
		filepath.Join(tempDir, "components/terraform/vpc/main.tf"),
		filepath.Join(tempDir, "components/terraform/eks/main.tf"),
		filepath.Join(tempDir, "components/helmfile/app/helmfile.yaml"),
		filepath.Join(tempDir, "components/packer/image/packer.json"),
		filepath.Join(tempDir, "stacks/dev.yaml"),
		filepath.Join(tempDir, "other/file.txt"), // Outside all base paths, NOT indexed
	}

	index := newChangedFilesIndex(atmosConfig, changedFiles)

	t.Run("get terraform files", func(t *testing.T) {
		files := index.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)
		// Should include ONLY terraform files (files in terraform base path).
		assert.Len(t, files, 2)
		assert.Contains(t, files, filepath.Join(tempDir, "components/terraform/vpc/main.tf"))
		assert.Contains(t, files, filepath.Join(tempDir, "components/terraform/eks/main.tf"))
		// Files outside base paths are NOT included.
		assert.NotContains(t, files, filepath.Join(tempDir, "other/file.txt"))
	})

	t.Run("get helmfile files", func(t *testing.T) {
		files := index.getRelevantFiles(cfg.HelmfileComponentType, atmosConfig)
		// Should include ONLY helmfile files (files in helmfile base path).
		assert.Len(t, files, 1)
		assert.Contains(t, files, filepath.Join(tempDir, "components/helmfile/app/helmfile.yaml"))
		assert.NotContains(t, files, filepath.Join(tempDir, "other/file.txt"))
	})

	t.Run("get packer files", func(t *testing.T) {
		files := index.getRelevantFiles(cfg.PackerComponentType, atmosConfig)
		// Should include ONLY packer files (files in packer base path).
		assert.Len(t, files, 1)
		assert.Contains(t, files, filepath.Join(tempDir, "components/packer/image/packer.json"))
		assert.NotContains(t, files, filepath.Join(tempDir, "other/file.txt"))
	})

	t.Run("get all files", func(t *testing.T) {
		files := index.getAllFiles()
		assert.Len(t, files, 6)
	})

	t.Run("only matched files", func(t *testing.T) {
		// Test with only files that match known base paths.
		matchedFiles := []string{
			filepath.Join(tempDir, "components/terraform/vpc/main.tf"),
			filepath.Join(tempDir, "components/helmfile/app/helmfile.yaml"),
		}
		matchedIndex := newChangedFilesIndex(atmosConfig, matchedFiles)

		tfFiles := matchedIndex.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)
		assert.Len(t, tfFiles, 1)
		assert.Contains(t, tfFiles, filepath.Join(tempDir, "components/terraform/vpc/main.tf"))

		helmFiles := matchedIndex.getRelevantFiles(cfg.HelmfileComponentType, atmosConfig)
		assert.Len(t, helmFiles, 1)
		assert.Contains(t, helmFiles, filepath.Join(tempDir, "components/helmfile/app/helmfile.yaml"))
	})
}

// TestChangedFilesIndex_PathCollisionPrevention tests that sibling paths with shared prefixes
// don't collide when indexing files. This is a critical bug fix to prevent files from being
// assigned to the wrong base path (e.g., "components/terraform-modules" incorrectly matching
// "components/terraform" with HasPrefix).
func TestChangedFilesIndex_PathCollisionPrevention(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
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

	t.Run("files in terraform subdirectory are correctly indexed", func(t *testing.T) {
		// File deep within terraform path should be correctly assigned.
		changedFiles := []string{
			filepath.Join(tempDir, "components/terraform/vpc/main.tf"),
		}
		index := newChangedFilesIndex(atmosConfig, changedFiles)

		tfFiles := index.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)
		assert.Contains(t, tfFiles, filepath.Join(tempDir, "components/terraform/vpc/main.tf"))
		assert.Len(t, tfFiles, 1)
	})

	t.Run("sibling path with shared prefix does not incorrectly match", func(t *testing.T) {
		// This tests the bug fix: with HasPrefix, "components/terraform-backup" would incorrectly
		// match "components/terraform" and be assigned to the wrong base path.
		// With filepath.Rel path boundary checking, it correctly does NOT match any base path.
		//
		// Files outside configured base paths are NOT indexed for component folder checking.
		// They will still be checked via:
		// - Module pattern cache (if referenced as Terraform modules)
		// - Dependency checking (if specified in component dependencies)
		changedFiles := []string{
			filepath.Join(tempDir, "components/terraform/vpc/main.tf"),        // Matches terraform
			filepath.Join(tempDir, "components/terraform-backup/old/data.tf"), // Sibling, should NOT match
			filepath.Join(tempDir, "components/helmfile/app/helmfile.yaml"),   // Matches helmfile
		}
		index := newChangedFilesIndex(atmosConfig, changedFiles)

		tfFiles := index.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)
		helmFiles := index.getRelevantFiles(cfg.HelmfileComponentType, atmosConfig)

		// Terraform files should include ONLY the terraform file.
		assert.Contains(t, tfFiles, filepath.Join(tempDir, "components/terraform/vpc/main.tf"))
		assert.Len(t, tfFiles, 1, "terraform files should only contain files in terraform base path")

		// The terraform-backup file should NOT be in the terraform list (it's outside the base path).
		assert.NotContains(t, tfFiles, filepath.Join(tempDir, "components/terraform-backup/old/data.tf"))

		// Helmfile files should include ONLY the helmfile file.
		assert.Contains(t, helmFiles, filepath.Join(tempDir, "components/helmfile/app/helmfile.yaml"))
		assert.Len(t, helmFiles, 1, "helmfile files should only contain files in helmfile base path")

		// The terraform-backup file should NOT be in the helmfile list either.
		assert.NotContains(t, helmFiles, filepath.Join(tempDir, "components/terraform-backup/old/data.tf"))
	})

	t.Run("parent path does not match child path", func(t *testing.T) {
		// File in parent directory should not be considered part of subdirectory.
		// Files outside configured base paths are NOT indexed.
		changedFiles := []string{
			filepath.Join(tempDir, "components/file.tf"), // Parent of terraform/helmfile, doesn't match any base path
		}
		index := newChangedFilesIndex(atmosConfig, changedFiles)

		tfFiles := index.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)

		// File should NOT be in the list (it's outside all configured base paths).
		assert.NotContains(t, tfFiles, filepath.Join(tempDir, "components/file.tf"))
		assert.Len(t, tfFiles, 0, "no files should match when changed file is outside all base paths")
	})
}

func TestChangedFilesIndex_ThreadSafety(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	changedFiles := []string{
		filepath.Join(tempDir, "components/terraform/vpc/main.tf"),
	}

	index := newChangedFilesIndex(atmosConfig, changedFiles)

	// Run concurrent reads.
	var wg sync.WaitGroup
	concurrency := 10

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			files := index.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)
			assert.NotNil(t, files)
		}()
	}

	wg.Wait()
}

// ==============================================================================
// Integration Tests
// ==============================================================================

func TestIsComponentFolderChangedIndexed(t *testing.T) {
	tempDir := t.TempDir()

	// Create component paths.
	componentPath := filepath.Join(tempDir, "components/terraform/vpc")
	err := os.MkdirAll(componentPath, 0o755)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	changedFiles := []string{
		filepath.Join(tempDir, "components/terraform/vpc/main.tf"),
		filepath.Join(tempDir, "other/file.txt"),
	}

	filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
	patternCache := newComponentPathPatternCache()

	t.Run("changed file in component folder", func(t *testing.T) {
		changed, err := isComponentFolderChangedIndexed("vpc", cfg.TerraformComponentType, atmosConfig, filesIndex, patternCache)
		require.NoError(t, err)
		assert.True(t, changed)
	})

	t.Run("no changed files in component folder", func(t *testing.T) {
		changed, err := isComponentFolderChangedIndexed("eks", cfg.TerraformComponentType, atmosConfig, filesIndex, patternCache)
		require.NoError(t, err)
		assert.False(t, changed)
	})
}

func TestAreTerraformComponentModulesChangedIndexed(t *testing.T) {
	tempDir := t.TempDir()

	// Create component with module.
	componentPath := filepath.Join(tempDir, "components/terraform/vpc")
	modulePath := filepath.Join(componentPath, "modules/subnets")
	err := os.MkdirAll(modulePath, 0o755)
	require.NoError(t, err)

	mainTf := `
module "subnets" {
  source = "./modules/subnets"
}
`
	err = os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte(mainTf), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(modulePath, "main.tf"), []byte("# module"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	t.Run("module file changed", func(t *testing.T) {
		changedFiles := []string{
			filepath.Join(modulePath, "main.tf"),
		}

		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		changed, err := areTerraformComponentModulesChangedIndexed("vpc", atmosConfig, filesIndex, patternCache)
		require.NoError(t, err)
		assert.True(t, changed)
	})

	t.Run("module file not changed", func(t *testing.T) {
		changedFiles := []string{
			filepath.Join(tempDir, "other/file.txt"),
		}

		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		changed, err := areTerraformComponentModulesChangedIndexed("vpc", atmosConfig, filesIndex, patternCache)
		require.NoError(t, err)
		assert.False(t, changed)
	})
}

// ==============================================================================
// Additional Integration Tests for Coverage
// ==============================================================================

func TestIsComponentDependentFolderOrFileChangedIndexed(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	t.Run("file dependency changed", func(t *testing.T) {
		depFile := filepath.Join(tempDir, "config/settings.yaml")
		err := os.MkdirAll(filepath.Dir(depFile), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(depFile, []byte("key: value"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{depFile}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)

		deps := schema.DependsOn{
			"dep1": schema.Context{File: depFile},
		}

		changed, changedType, changedPath, err := isComponentDependentFolderOrFileChangedIndexed(filesIndex, deps)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, "file", changedType)
		assert.Equal(t, depFile, changedPath)
	})

	t.Run("folder dependency changed", func(t *testing.T) {
		depFolder := filepath.Join(tempDir, "modules/vpc")
		depFile := filepath.Join(depFolder, "main.tf")
		err := os.MkdirAll(filepath.Dir(depFile), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(depFile, []byte("# vpc module"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{depFile}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)

		deps := schema.DependsOn{
			"dep1": schema.Context{Folder: depFolder},
		}

		changed, changedType, changedPath, err := isComponentDependentFolderOrFileChangedIndexed(filesIndex, deps)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, "folder", changedType)
		assert.Equal(t, depFolder, changedPath)
	})

	t.Run("no dependencies changed", func(t *testing.T) {
		changedFiles := []string{
			filepath.Join(tempDir, "other/unrelated.txt"),
		}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)

		deps := schema.DependsOn{
			"dep1": schema.Context{File: filepath.Join(tempDir, "config/settings.yaml")},
			"dep2": schema.Context{Folder: filepath.Join(tempDir, "modules/vpc")},
		}

		changed, _, _, err := isComponentDependentFolderOrFileChangedIndexed(filesIndex, deps)
		require.NoError(t, err)
		assert.False(t, changed)
	})

	t.Run("multiple dependencies, first changed", func(t *testing.T) {
		depFile1 := filepath.Join(tempDir, "config/first.yaml")
		err := os.MkdirAll(filepath.Dir(depFile1), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(depFile1, []byte("key: value"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{depFile1}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)

		deps := schema.DependsOn{
			"dep1": schema.Context{File: depFile1},
			"dep2": schema.Context{File: filepath.Join(tempDir, "config/second.yaml")},
			"dep3": schema.Context{Folder: filepath.Join(tempDir, "modules/vpc")},
		}

		changed, changedType, changedPath, err := isComponentDependentFolderOrFileChangedIndexed(filesIndex, deps)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, "file", changedType)
		assert.Equal(t, depFile1, changedPath)
	})

	t.Run("empty dependencies", func(t *testing.T) {
		changedFiles := []string{
			filepath.Join(tempDir, "some/file.txt"),
		}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)

		deps := schema.DependsOn{}

		changed, _, _, err := isComponentDependentFolderOrFileChangedIndexed(filesIndex, deps)
		require.NoError(t, err)
		assert.False(t, changed)
	})

	t.Run("mixed valid and empty dependencies", func(t *testing.T) {
		// This test verifies the bug fix where hasDependencies flag was not reset per iteration.
		// Previously, if a dependency had File/Folder set, and a later dependency had neither,
		// the code would process the empty dependency with stale values from the previous one.
		depFile := filepath.Join(tempDir, "config/settings.yaml")
		err := os.MkdirAll(filepath.Dir(depFile), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(depFile, []byte("key: value"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{depFile}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)

		// Create dependencies with both valid and empty entries.
		deps := schema.DependsOn{
			"dep1": schema.Context{File: depFile},
			// Empty dependency - has neither File nor Folder.
			"dep2": schema.Context{},
			// Another valid one after the empty one.
			"dep3": schema.Context{Folder: filepath.Join(tempDir, "modules/vpc")},
		}

		changed, changedType, changedPath, err := isComponentDependentFolderOrFileChangedIndexed(filesIndex, deps)
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, "file", changedType)
		assert.Equal(t, depFile, changedPath)
	})
}

func TestProcessHelmfileComponentsIndexed(t *testing.T) {
	tempDir := t.TempDir()

	// Create helmfile component structure.
	helmfilePath := filepath.Join(tempDir, "components/helmfile/app")
	err := os.MkdirAll(helmfilePath, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(helmfilePath, "helmfile.yaml"), []byte("releases: []"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
		},
	}

	t.Run("component metadata changed", func(t *testing.T) {
		currentStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.HelmfileComponentType: map[string]any{
						"app": map[string]any{
							"metadata": map[string]any{
								"enabled": true,
								"version": "2.0",
							},
							"component": "app",
							"vars": map[string]any{
								"namespace": "default",
							},
						},
					},
				},
			},
		}

		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.HelmfileComponentType: map[string]any{
						"app": map[string]any{
							"metadata": map[string]any{
								"enabled": true,
								"version": "1.0", // Different version
							},
							"component": "app",
							"vars": map[string]any{
								"namespace": "default",
							},
						},
					},
				},
			},
		}

		changedFiles := []string{}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		helmfileSection := (*currentStacks)["dev-stack"].(map[string]any)["components"].(map[string]any)[cfg.HelmfileComponentType].(map[string]any)

		affected, err := processHelmfileComponentsIndexed(
			"dev-stack",
			helmfileSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false, // includeSpaceliftAdminStacks
			false, // includeSettings
			false, // excludeLocked
		)

		require.NoError(t, err)
		assert.Len(t, affected, 1)
		assert.Equal(t, "app", affected[0].Component)
		assert.Equal(t, "stack.metadata", affected[0].Affected)
	})

	t.Run("component file changed", func(t *testing.T) {
		currentStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.HelmfileComponentType: map[string]any{
						"app": map[string]any{
							"metadata": map[string]any{
								"enabled": true,
							},
							"component": "app",
							"vars": map[string]any{
								"namespace": "default",
							},
						},
					},
				},
			},
		}

		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.HelmfileComponentType: map[string]any{
						"app": map[string]any{
							"metadata": map[string]any{
								"enabled": true,
							},
							"component": "app",
							"vars": map[string]any{
								"namespace": "default",
							},
						},
					},
				},
			},
		}

		changedFiles := []string{
			filepath.Join(helmfilePath, "helmfile.yaml"),
		}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		helmfileSection := (*currentStacks)["dev-stack"].(map[string]any)["components"].(map[string]any)[cfg.HelmfileComponentType].(map[string]any)

		affected, err := processHelmfileComponentsIndexed(
			"dev-stack",
			helmfileSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		// File changes SHOULD be detected independently of stack config changes.
		assert.Len(t, affected, 1, "component file changed, should be detected")
		assert.Equal(t, "app", affected[0].Component)
		assert.Equal(t, "component", affected[0].Affected)
	})

	t.Run("vars changed", func(t *testing.T) {
		currentStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.HelmfileComponentType: map[string]any{
						"app": map[string]any{
							"component": "app",
							"vars": map[string]any{
								"namespace": "production",
							},
						},
					},
				},
			},
		}

		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.HelmfileComponentType: map[string]any{
						"app": map[string]any{
							"component": "app",
							"vars": map[string]any{
								"namespace": "default",
							},
						},
					},
				},
			},
		}

		changedFiles := []string{}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		helmfileSection := (*currentStacks)["dev-stack"].(map[string]any)["components"].(map[string]any)[cfg.HelmfileComponentType].(map[string]any)

		affected, err := processHelmfileComponentsIndexed(
			"dev-stack",
			helmfileSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Len(t, affected, 1)
		assert.Equal(t, "app", affected[0].Component)
		assert.Equal(t, "stack.vars", affected[0].Affected)
	})
}

func TestProcessPackerComponentsIndexed(t *testing.T) {
	tempDir := t.TempDir()

	// Create packer component structure.
	packerPath := filepath.Join(tempDir, "components/packer/image")
	err := os.MkdirAll(packerPath, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(packerPath, "template.pkr.hcl"), []byte("# packer template"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	t.Run("component env changed", func(t *testing.T) {
		currentStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.PackerComponentType: map[string]any{
						"image": map[string]any{
							"metadata": map[string]any{
								"enabled": true,
							},
							"component": "image",
							"env": map[string]any{
								"AWS_REGION": "us-east-1",
							},
						},
					},
				},
			},
		}

		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.PackerComponentType: map[string]any{
						"image": map[string]any{
							"metadata": map[string]any{
								"enabled": true,
							},
							"component": "image",
							"env": map[string]any{
								"AWS_REGION": "us-west-2", // Different region
							},
						},
					},
				},
			},
		}

		changedFiles := []string{}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		packerSection := (*currentStacks)["dev-stack"].(map[string]any)["components"].(map[string]any)[cfg.PackerComponentType].(map[string]any)

		affected, err := processPackerComponentsIndexed(
			"dev-stack",
			packerSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Len(t, affected, 1)
		assert.Equal(t, "image", affected[0].Component)
		assert.Equal(t, "stack.env", affected[0].Affected)
	})

	t.Run("component file changed", func(t *testing.T) {
		currentStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.PackerComponentType: map[string]any{
						"image": map[string]any{
							"component": "image",
							"vars": map[string]any{
								"ami_name": "test-image",
							},
						},
					},
				},
			},
		}

		remoteStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.PackerComponentType: map[string]any{
						"image": map[string]any{
							"component": "image",
							"vars": map[string]any{
								"ami_name": "test-image",
							},
						},
					},
				},
			},
		}

		changedFiles := []string{
			filepath.Join(packerPath, "template.pkr.hcl"),
		}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		packerSection := (*currentStacks)["dev-stack"].(map[string]any)["components"].(map[string]any)[cfg.PackerComponentType].(map[string]any)

		affected, err := processPackerComponentsIndexed(
			"dev-stack",
			packerSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		// File changes SHOULD be detected independently of stack config changes.
		assert.Len(t, affected, 1, "component file changed, should be detected")
		assert.Equal(t, "image", affected[0].Component)
		assert.Equal(t, "component", affected[0].Affected)
	})

	t.Run("skip abstract component", func(t *testing.T) {
		currentStacks := &map[string]any{
			"dev-stack": map[string]any{
				"components": map[string]any{
					cfg.PackerComponentType: map[string]any{
						"image": map[string]any{
							"metadata": map[string]any{
								"type": "abstract",
							},
							"component": "image",
						},
					},
				},
			},
		}

		remoteStacks := &map[string]any{}
		changedFiles := []string{}
		filesIndex := newChangedFilesIndex(atmosConfig, changedFiles)
		patternCache := newComponentPathPatternCache()

		packerSection := (*currentStacks)["dev-stack"].(map[string]any)["components"].(map[string]any)[cfg.PackerComponentType].(map[string]any)

		affected, err := processPackerComponentsIndexed(
			"dev-stack",
			packerSection,
			remoteStacks,
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Len(t, affected, 0) // Abstract component should be skipped
	})
}

func TestIsComponentFolderChangedCoverage(t *testing.T) {
	tempDir := t.TempDir()

	// Create component structure.
	vpcPath := filepath.Join(tempDir, "components/terraform/vpc")
	err := os.MkdirAll(vpcPath, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(vpcPath, "main.tf"), []byte("# vpc"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
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
	}

	t.Run("terraform component changed", func(t *testing.T) {
		changedFiles := []string{
			filepath.Join(vpcPath, "main.tf"),
		}

		changed, err := isComponentFolderChanged("vpc", cfg.TerraformComponentType, atmosConfig, changedFiles)
		require.NoError(t, err)
		assert.True(t, changed)
	})

	t.Run("terraform component not changed", func(t *testing.T) {
		changedFiles := []string{
			filepath.Join(tempDir, "other/file.txt"),
		}

		changed, err := isComponentFolderChanged("vpc", cfg.TerraformComponentType, atmosConfig, changedFiles)
		require.NoError(t, err)
		assert.False(t, changed)
	})

	t.Run("helmfile component changed", func(t *testing.T) {
		helmfilePath := filepath.Join(tempDir, "components/helmfile/app")
		err := os.MkdirAll(helmfilePath, 0o755)
		require.NoError(t, err)
		helmfileFile := filepath.Join(helmfilePath, "helmfile.yaml")
		err = os.WriteFile(helmfileFile, []byte("releases: []"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{helmfileFile}

		changed, err := isComponentFolderChanged("app", cfg.HelmfileComponentType, atmosConfig, changedFiles)
		require.NoError(t, err)
		assert.True(t, changed)
	})

	t.Run("packer component changed", func(t *testing.T) {
		packerPath := filepath.Join(tempDir, "components/packer/image")
		err := os.MkdirAll(packerPath, 0o755)
		require.NoError(t, err)
		packerFile := filepath.Join(packerPath, "template.pkr.hcl")
		err = os.WriteFile(packerFile, []byte("# packer"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{packerFile}

		changed, err := isComponentFolderChanged("image", cfg.PackerComponentType, atmosConfig, changedFiles)
		require.NoError(t, err)
		assert.True(t, changed)
	})

	t.Run("unsupported component type", func(t *testing.T) {
		changedFiles := []string{}

		_, err := isComponentFolderChanged("test", "unknown", atmosConfig, changedFiles)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedComponentType)
	})

	t.Run("subdirectory file changed", func(t *testing.T) {
		subDir := filepath.Join(vpcPath, "modules/subnets")
		err := os.MkdirAll(subDir, 0o755)
		require.NoError(t, err)
		subFile := filepath.Join(subDir, "main.tf")
		err = os.WriteFile(subFile, []byte("# subnets"), 0o644)
		require.NoError(t, err)

		changedFiles := []string{subFile}

		changed, err := isComponentFolderChanged("vpc", cfg.TerraformComponentType, atmosConfig, changedFiles)
		require.NoError(t, err)
		assert.True(t, changed)
	})
}

func TestChangedFilesIndex_GetRelevantFiles_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	changedFiles := []string{
		filepath.Join(tempDir, "components/terraform/vpc/main.tf"),
	}

	index := newChangedFilesIndex(atmosConfig, changedFiles)

	t.Run("unknown component type returns all files", func(t *testing.T) {
		files := index.getRelevantFiles("unknown-type", atmosConfig)
		// Should fallback to all files for unknown types.
		assert.Equal(t, index.allFiles, files)
	})

	t.Run("base path not in index returns all files", func(t *testing.T) {
		// Create config with a base path that doesn't match any changed files.
		emptyConfig := &schema.AtmosConfiguration{
			BasePath: "/nonexistent/path",
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		emptyIndex := newChangedFilesIndex(emptyConfig, []string{})
		files := emptyIndex.getRelevantFiles(cfg.TerraformComponentType, emptyConfig)
		// Should return all files (empty in this case) as fallback.
		assert.Equal(t, emptyIndex.allFiles, files)
	})
}

func TestComponentPathPatternCache_GetTerraformModulePatterns_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	// Create component with remote module (has version).
	componentPath := filepath.Join(tempDir, "components/terraform/app")
	err := os.MkdirAll(componentPath, 0o755)
	require.NoError(t, err)

	// Create main.tf with only remote modules (should be skipped).
	mainTf := `
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
}

module "s3" {
  source  = "terraform-aws-modules/s3-bucket/aws"
  version = "3.0.0"
}
`
	err = os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte(mainTf), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	cache := newComponentPathPatternCache()

	t.Run("remote modules with version are skipped", func(t *testing.T) {
		patterns, err := cache.getTerraformModulePatterns("app", atmosConfig)
		require.NoError(t, err)
		// Should be empty since all modules are remote (have version).
		assert.Empty(t, patterns)
	})

	t.Run("mixed local and remote modules", func(t *testing.T) {
		// Create component with both local and remote modules.
		mixedPath := filepath.Join(tempDir, "components/terraform/mixed")
		err := os.MkdirAll(mixedPath, 0o755)
		require.NoError(t, err)

		mixedTf := `
module "local" {
  source = "./modules/local"
}

module "remote" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
}
`
		err = os.WriteFile(filepath.Join(mixedPath, "main.tf"), []byte(mixedTf), 0o644)
		require.NoError(t, err)

		// Create local module directory.
		localModulePath := filepath.Join(mixedPath, "modules/local")
		err = os.MkdirAll(localModulePath, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(localModulePath, "main.tf"), []byte("# local"), 0o644)
		require.NoError(t, err)

		patterns, err := cache.getTerraformModulePatterns("mixed", atmosConfig)
		require.NoError(t, err)
		// Should have only the local module pattern.
		assert.Len(t, patterns, 1)
		assert.Contains(t, patterns[0], "modules/local")
	})

	t.Run("component with multiple local modules", func(t *testing.T) {
		// Create component with multiple local modules.
		multiPath := filepath.Join(tempDir, "components/terraform/multi")
		err := os.MkdirAll(multiPath, 0o755)
		require.NoError(t, err)

		multiTf := `
module "networking" {
  source = "./modules/networking"
}

module "security" {
  source = "./modules/security"
}

module "storage" {
  source = "../shared/storage"
}
`
		err = os.WriteFile(filepath.Join(multiPath, "main.tf"), []byte(multiTf), 0o644)
		require.NoError(t, err)

		// Create local module directories.
		for _, moduleName := range []string{"networking", "security"} {
			modulePath := filepath.Join(multiPath, "modules", moduleName)
			err = os.MkdirAll(modulePath, 0o755)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(modulePath, "main.tf"), []byte("# "+moduleName), 0o644)
			require.NoError(t, err)
		}

		// Create shared module.
		sharedPath := filepath.Join(tempDir, "components/terraform/shared/storage")
		err = os.MkdirAll(sharedPath, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(sharedPath, "main.tf"), []byte("# storage"), 0o644)
		require.NoError(t, err)

		patterns, err := cache.getTerraformModulePatterns("multi", atmosConfig)
		require.NoError(t, err)
		// Should have patterns for all three local modules.
		assert.Len(t, patterns, 3)
	})
}

func TestProcessStackAffected_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	filesIndex := newChangedFilesIndex(atmosConfig, []string{})
	patternCache := newComponentPathPatternCache()

	t.Run("invalid stack section returns empty", func(t *testing.T) {
		// Stack section is not a map.
		invalidStackSection := "not-a-map"

		affected, err := processStackAffected(
			"test-stack",
			invalidStackSection,
			&map[string]any{},
			&map[string]any{},
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Empty(t, affected)
	})

	t.Run("no components section returns empty", func(t *testing.T) {
		// Stack section without components.
		stackSection := map[string]any{
			"other": "data",
		}

		affected, err := processStackAffected(
			"test-stack",
			stackSection,
			&map[string]any{},
			&map[string]any{},
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Empty(t, affected)
	})

	t.Run("components section not a map returns empty", func(t *testing.T) {
		// Components section is not a map.
		stackSection := map[string]any{
			"components": "not-a-map",
		}

		affected, err := processStackAffected(
			"test-stack",
			stackSection,
			&map[string]any{},
			&map[string]any{},
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Empty(t, affected)
	})

	t.Run("empty components sections", func(t *testing.T) {
		// Valid structure but no components.
		stackSection := map[string]any{
			"components": map[string]any{
				cfg.TerraformComponentType: map[string]any{},
				cfg.HelmfileComponentType:  map[string]any{},
				cfg.PackerComponentType:    map[string]any{},
			},
		}

		currentStacks := &map[string]any{
			"test-stack": stackSection,
		}

		affected, err := processStackAffected(
			"test-stack",
			stackSection,
			&map[string]any{},
			currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		require.NoError(t, err)
		assert.Empty(t, affected)
	})
}
