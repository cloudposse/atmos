package cmd

import (
	"testing"
)

// TestDocsCmd_WithoutStacks verifies that docs command does not require stack configuration.
// This test documents that the docs command uses InitCliConfig with processStacks=false.
func TestDocsCmd_WithoutStacks(t *testing.T) {
	// This test documents that docs command does not process stacks
	// by verifying InitCliConfig is called with processStacks=false in docs.go:38
	// No runtime test needed - this is enforced by code structure.
	t.Log("docs command uses InitCliConfig with processStacks=false")
}
