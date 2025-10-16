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
		filepath.Join(tempDir, "other/file.txt"), // Unmatched files added to all paths as fallback
	}

	index := newChangedFilesIndex(atmosConfig, changedFiles)

	t.Run("get terraform files", func(t *testing.T) {
		files := index.getRelevantFiles(cfg.TerraformComponentType, atmosConfig)
		// Should include terraform files + unmatched files (fallback safety).
		assert.GreaterOrEqual(t, len(files), 2)
		assert.Contains(t, files, filepath.Join(tempDir, "components/terraform/vpc/main.tf"))
		assert.Contains(t, files, filepath.Join(tempDir, "components/terraform/eks/main.tf"))
	})

	t.Run("get helmfile files", func(t *testing.T) {
		files := index.getRelevantFiles(cfg.HelmfileComponentType, atmosConfig)
		// Should include helmfile files + unmatched files (fallback safety).
		assert.GreaterOrEqual(t, len(files), 1)
		assert.Contains(t, files, filepath.Join(tempDir, "components/helmfile/app/helmfile.yaml"))
	})

	t.Run("get packer files", func(t *testing.T) {
		files := index.getRelevantFiles(cfg.PackerComponentType, atmosConfig)
		// Should include packer files + unmatched files (fallback safety).
		assert.GreaterOrEqual(t, len(files), 1)
		assert.Contains(t, files, filepath.Join(tempDir, "components/packer/image/packer.json"))
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
