package format

import (
	"strings"
	"testing"

	listtree "github.com/cloudposse/atmos/pkg/list/tree"
)

func TestRenderInstancesTree_SpacerBetweenComponents(t *testing.T) {
	// Create sample stacks with multiple components.
	stacksWithComponents := map[string]map[string][]*listtree.ImportNode{
		"plat-uw2-staging": {
			"vpc": {
				{Path: "orgs/acme/plat/staging/_defaults"},
				{Path: "orgs/acme/plat/_defaults"},
				{Path: "orgs/acme/_defaults"},
			},
			"vpc-flow-logs-bucket": {
				{Path: "orgs/acme/plat/staging/_defaults"},
				{Path: "orgs/acme/plat/_defaults"},
			},
			"eks": {
				{Path: "catalog/eks/defaults"},
			},
		},
	}

	output := RenderInstancesTree(stacksWithComponents, false)

	// Strip ANSI codes for testing.
	plainOutput := stripANSI(output)

	// Verify header is present.
	if !strings.Contains(plainOutput, "Component Instances") {
		t.Error("Expected 'Component Instances' header in output")
	}

	// Verify stack name is present.
	if !strings.Contains(plainOutput, "plat-uw2-staging") {
		t.Error("Expected 'plat-uw2-staging' stack in output")
	}

	// Verify all components are present.
	expectedComponents := []string{"vpc", "vpc-flow-logs-bucket", "eks"}
	for _, comp := range expectedComponents {
		if !strings.Contains(plainOutput, comp) {
			t.Errorf("Expected component '%s' in output", comp)
		}
	}

	// Verify spacer lines (│) exist between components.
	lines := strings.Split(plainOutput, "\n")
	spacerCount := 0
	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		// Look for lines that are just the vertical bar (spacer).
		if stripped == "│" {
			spacerCount++
		}
	}

	// We should have at least 3 spacers:
	// - 1 at top (after "Stacks" header)
	// - 2 between 3 components (vpc-vpc-flow-logs-bucket, vpc-flow-logs-bucket-eks)
	if spacerCount < 3 {
		t.Errorf("Expected at least 3 spacer lines, got %d", spacerCount)
		t.Logf("Output:\n%s", plainOutput)
	}
}

func TestRenderInstancesTree_EmptyInput(t *testing.T) {
	stacksWithComponents := map[string]map[string][]*listtree.ImportNode{}

	output := RenderInstancesTree(stacksWithComponents, false)

	if !strings.Contains(output, "No stacks found") {
		t.Errorf("Expected 'No stacks found' message, got: %s", output)
	}
}

func TestRenderInstancesTree_MultipleStacks(t *testing.T) {
	stacksWithComponents := map[string]map[string][]*listtree.ImportNode{
		"stack-a": {
			"component1": {{Path: "imports/a"}},
			"component2": {{Path: "imports/b"}},
		},
		"stack-b": {
			"component3": {{Path: "imports/c"}},
		},
	}

	output := RenderInstancesTree(stacksWithComponents, false)
	plainOutput := stripANSI(output)

	// Verify both stacks are present.
	if !strings.Contains(plainOutput, "stack-a") {
		t.Error("Expected 'stack-a' in output")
	}
	if !strings.Contains(plainOutput, "stack-b") {
		t.Error("Expected 'stack-b' in output")
	}

	// Verify all components are present.
	expectedComponents := []string{"component1", "component2", "component3"}
	for _, comp := range expectedComponents {
		if !strings.Contains(plainOutput, comp) {
			t.Errorf("Expected component '%s' in output", comp)
		}
	}

	// Count spacers.
	lines := strings.Split(plainOutput, "\n")
	spacerCount := 0
	for _, line := range lines {
		if strings.TrimSpace(stripANSI(line)) == "│" {
			spacerCount++
		}
	}

	// Should have:
	// - 1 at top (after "Stacks")
	// - 1 between component1 and component2 in stack-a
	// - 1 between stack-a and stack-b
	// Total: at least 3 spacers
	if spacerCount < 3 {
		t.Errorf("Expected at least 3 spacer lines, got %d", spacerCount)
		t.Logf("Output:\n%s", plainOutput)
	}
}
