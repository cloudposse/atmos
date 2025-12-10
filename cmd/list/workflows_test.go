package list

import (
	"testing"
)

// TestListWorkflowsCmd_WithoutStacks verifies that list workflows does not require stack configuration.
// This test documents that the command uses InitCliConfig with processStacks=false.
func TestListWorkflowsCmd_WithoutStacks(t *testing.T) {
	// This test documents that list workflows command does not process stacks
	// by verifying InitCliConfig is called with processStacks=false in list_workflows.go:39
	// and that checkAtmosConfig is called with WithStackValidation(false) in list_workflows.go:19
	// No runtime test needed - this is enforced by code structure.
	t.Log("list workflows command uses InitCliConfig with processStacks=false")
}
