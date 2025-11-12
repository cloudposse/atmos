package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudposse/atmos/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
	"github.com/cloudposse/atmos/toolchain/registry/aqua"
	"github.com/cloudposse/atmos/toolchain/registry/cache"
)

// TestTypeAssertion verifies the type assertion works correctly.
func TestTypeAssertion(t *testing.T) {
	reg := toolchain.NewAquaRegistry()

	t.Logf("Registry type: %T", reg)

	// Try the type assertion that's in the code.
	if aquaReg, ok := reg.(*aqua.AquaRegistry); ok {
		t.Logf("SUCCESS: Type assertion to *aqua.AquaRegistry succeeded")
		t.Logf("AquaRegistry pointer: %p", aquaReg)
	} else {
		t.Errorf("FAILED: Type assertion to *aqua.AquaRegistry failed")
		t.Errorf("Actual type: %T", reg)
	}
}

// TestSearchShowsTotalCount verifies that search returns total count properly.
func TestSearchShowsTotalCount(t *testing.T) {
	// Create a temporary cache directory for the test.
	cacheDir := t.TempDir()

	// Set XDG_CACHE_HOME to use our temp directory to avoid polluting the real cache.
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	// Pre-populate the cache with test registry data to avoid live network calls.
	// The fetchRegistryIndex function checks the cache first with key "aqua-registry-index".
	// The cache path is {XDG_CACHE_HOME}/atmos/toolchain.
	atmosToolchainCacheDir := filepath.Join(cacheDir, "atmos", "toolchain")
	if err := os.MkdirAll(atmosToolchainCacheDir, 0o755); err != nil {
		t.Fatalf("Failed to create atmos toolchain cache dir: %v", err)
	}

	cacheStore := cache.NewFileStore(atmosToolchainCacheDir)
	ctx := context.Background()

	// Create mock registry YAML with multiple terraform packages to test pagination.
	// This registry has 10 packages total, with several matching "terraform".
	registryYAML := `packages:
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform-provider-aws
  - type: github_release
    repo_owner: hashicorp
    repo_name: terraform-provider-google
  - type: github_release
    repo_owner: terraform-linters
    repo_name: tflint
  - type: github_release
    repo_owner: gruntwork-io
    repo_name: terragrunt
  - type: github_release
    repo_owner: terraform-docs
    repo_name: terraform-docs
  - type: github_release
    repo_owner: cycloidio
    repo_name: terracognita
  - type: github_release
    repo_owner: aquasecurity
    repo_name: tfsec
  - type: github_release
    repo_owner: opentofu
    repo_name: opentofu
  - type: github_release
    repo_owner: infracost
    repo_name: infracost
`

	// Store the test data in the cache with a long TTL so it doesn't expire during the test.
	cacheKey := "aqua-registry-index"
	if err := cacheStore.Set(ctx, cacheKey, []byte(registryYAML), 24*time.Hour); err != nil {
		t.Fatalf("Failed to populate cache: %v", err)
	}

	// Create registry - it will use the pre-populated cache.
	reg := toolchain.NewAquaRegistry()

	// Perform a search with limit smaller than expected results to test pagination.
	limit := 3
	results, err := reg.Search(ctx, "terraform", toolchainregistry.WithLimit(limit))
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify we got results.
	if len(results) == 0 {
		t.Fatal("Expected search results but got none")
	}

	t.Logf("Requested limit: %d, got %d results from search", limit, len(results))

	// Get total count using the same type assertion as the command.
	totalResults := len(results)
	if aquaReg, ok := reg.(*aqua.AquaRegistry); ok {
		totalResults = aquaReg.GetLastSearchTotal()
		t.Logf("Total search results from GetLastSearchTotal: %d", totalResults)
	} else {
		t.Errorf("Failed to get total count - type assertion to *aqua.AquaRegistry failed")
		t.Errorf("Actual type: %T", reg)
	}

	// Verify total >= results length.
	if totalResults < len(results) {
		t.Errorf("Expected totalResults (%d) >= len(results) (%d)", totalResults, len(results))
	}

	// For a search like "terraform" with limit=3, we expect totalResults > 3 if there are more matches.
	if totalResults == len(results) {
		t.Logf("WARNING: totalResults (%d) == len(results) (%d) - pagination info won't be shown", totalResults, len(results))
	} else {
		t.Logf("SUCCESS: totalResults (%d) > len(results) (%d) - pagination info should be shown", totalResults, len(results))
	}
}
