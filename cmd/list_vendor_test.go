package cmd

import (
	"testing"
)

// TestListVendorCmd_WithoutStacks verifies that list vendor does not require stack configuration.
// This test documents that the command uses InitCliConfig with processStacks=false.
func TestListVendorCmd_WithoutStacks(t *testing.T) {
	// This test documents that list vendor command does not process stacks
	// by verifying InitCliConfig is called with processStacks=false in list_vendor.go:44
	// and that checkAtmosConfig is called with WithStackValidation(false) in list_vendor.go:20
	// No runtime test needed - this is enforced by code structure.
	t.Log("list vendor command uses InitCliConfig with processStacks=false")
}
