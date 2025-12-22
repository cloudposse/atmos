package format

import (
	"strings"
	"testing"

	listtree "github.com/cloudposse/atmos/pkg/list/tree"
)

func TestRenderStacksTree_SpacerBetweenStacks(t *testing.T) {
	// Create sample stacks with imports.
	stacksWithImports := map[string][]*listtree.ImportNode{
		"plat-uw2-dev": {
			{Path: "orgs/acme/plat/dev/_defaults"},
			{Path: "orgs/acme/plat/_defaults"},
			{Path: "orgs/acme/_defaults"},
		},
		"plat-uw2-prod": {
			{Path: "orgs/acme/plat/prod/_defaults"},
			{Path: "orgs/acme/plat/_defaults"},
			{Path: "orgs/acme/_defaults"},
		},
		"plat-uw2-staging": {
			{Path: "orgs/acme/plat/staging/_defaults"},
			{Path: "orgs/acme/plat/_defaults"},
			{Path: "orgs/acme/_defaults"},
		},
	}

	output := RenderStacksTree(stacksWithImports, false)

	// Strip ANSI codes for testing.
	plainOutput := stripANSI(output)

	// Verify header is present.
	if !strings.Contains(plainOutput, "Stacks") {
		t.Error("Expected 'Stacks' header in output")
	}

	// Verify spacer lines (│) exist between stacks and at the top.
	lines := strings.Split(plainOutput, "\n")
	spacerCount := 0
	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		// Look for lines that are just the vertical bar (spacer).
		if stripped == "│" {
			spacerCount++
		}
	}

	// We should have 3 spacers (1 at top + 2 between 3 stacks: dev-prod and prod-staging).
	if spacerCount < 3 {
		t.Errorf("Expected at least 3 spacer lines (1 at top + 2 between stacks), got %d", spacerCount)
		t.Logf("Output:\n%s", plainOutput)
	}

	// Verify all stack names are present.
	expectedStacks := []string{"plat-uw2-dev", "plat-uw2-prod", "plat-uw2-staging"}
	for _, stack := range expectedStacks {
		if !strings.Contains(plainOutput, stack) {
			t.Errorf("Expected stack '%s' in output", stack)
		}
	}

	// When showImports=false, import paths should NOT be present.
	unexpectedImports := []string{
		"orgs/acme/plat/dev/_defaults",
		"orgs/acme/plat/_defaults",
		"orgs/acme/_defaults",
	}
	for _, imp := range unexpectedImports {
		if strings.Contains(plainOutput, imp) {
			t.Errorf("Did not expect import path '%s' in output when showImports=false", imp)
		}
	}
}

func TestRenderStacksTree_NoSpacerAfterLastStack(t *testing.T) {
	stacksWithImports := map[string][]*listtree.ImportNode{
		"stack-a": {
			{Path: "imports/a"},
		},
		"stack-b": {
			{Path: "imports/b"},
		},
	}

	output := RenderStacksTree(stacksWithImports, false)
	plainOutput := stripANSI(output)

	lines := strings.Split(plainOutput, "\n")

	// Find the last non-empty line.
	lastNonEmptyIndex := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastNonEmptyIndex = i
			break
		}
	}

	// Verify the last line is not a spacer.
	if lastNonEmptyIndex >= 0 {
		lastLine := strings.TrimSpace(lines[lastNonEmptyIndex])
		if lastLine == "│" {
			t.Error("Expected no spacer after the last stack")
		}
	}
}

func TestRenderStacksTree_EmptyInput(t *testing.T) {
	stacksWithImports := map[string][]*listtree.ImportNode{}

	output := RenderStacksTree(stacksWithImports, false)

	if !strings.Contains(output, "No stacks found") {
		t.Errorf("Expected 'No stacks found' message, got: %s", output)
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "text with color codes",
			input:    "\x1b[32mgreen text\x1b[0m",
			expected: "green text",
		},
		{
			name:     "text with multiple colors",
			input:    "\x1b[31mred\x1b[0m \x1b[34mblue\x1b[0m",
			expected: "red blue",
		},
		{
			name:     "tree characters with styling",
			input:    "\x1b[90m├──\x1b[0m",
			expected: "├──",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
