package registry

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/toolchain"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
	"github.com/cloudposse/atmos/toolchain/registry/aqua"
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
	// Create registry.
	reg := toolchain.NewAquaRegistry()

	// Perform a search with limit.
	ctx := context.Background()
	limit := 5
	results, err := reg.Search(ctx, "terraform", toolchainregistry.WithLimit(limit))
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Skip if no results (registry might be empty in CI).
	if len(results) == 0 {
		t.Skip("No search results - skipping test")
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

	// For a search like "terraform" with limit=5, we expect totalResults > 5.
	if totalResults == len(results) {
		t.Logf("WARNING: totalResults (%d) == len(results) (%d) - pagination info won't be shown", totalResults, len(results))
	} else {
		t.Logf("SUCCESS: totalResults (%d) > len(results) (%d) - pagination info should be shown", totalResults, len(results))
	}
}
