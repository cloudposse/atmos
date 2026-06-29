package format

import (
	"strings"
	"testing"
)

func TestRenderDependenciesTree_EmptyInput(t *testing.T) {
	output := RenderDependenciesTree(nil)

	if output != "No dependencies found" {
		t.Fatalf("expected empty message, got %q", output)
	}
}

func TestRenderDependenciesTree_SingleDirection(t *testing.T) {
	output := RenderDependenciesTree([]*DepTreeEntry{
		{
			Component: "app",
			Stack:     "dev",
			DependsOn: []*DepTreeNode{
				{
					Component: "db",
					Stack:     "dev",
					Children: []*DepTreeNode{
						{Component: "network", Stack: "shared"},
					},
				},
			},
		},
	})
	plain := stripANSI(output)

	for _, want := range []string{"Dependencies", "app [dev]", "db [dev]", "network [shared]"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "depends on") || strings.Contains(plain, "required by") {
		t.Fatalf("single-direction output should not include direction labels:\n%s", plain)
	}
}

func TestRenderDependenciesTree_BothDirectionsAndEmptyBranches(t *testing.T) {
	output := RenderDependenciesTree([]*DepTreeEntry{
		{
			Component: "app",
			Stack:     "dev",
			ShowBoth:  true,
			DependsOn: []*DepTreeNode{
				{Component: "db", Stack: "dev"},
			},
			RequiredBy: nil,
		},
	})
	plain := stripANSI(output)

	for _, want := range []string{"app [dev]", "depends on", "required by", "db [dev]", "(none)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, plain)
		}
	}
}

func TestRenderDependenciesTree_RequiredByFallbackAndCircular(t *testing.T) {
	output := RenderDependenciesTree([]*DepTreeEntry{
		{
			Component: "db",
			Stack:     "dev",
			RequiredBy: []*DepTreeNode{
				{
					Component: "app",
					Stack:     "dev",
					Circular:  true,
					Children: []*DepTreeNode{
						{Component: "should-not-render", Stack: "dev"},
					},
				},
			},
		},
	})
	plain := stripANSI(output)

	for _, want := range []string{"db [dev]", "app [dev] (circular reference)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "should-not-render") {
		t.Fatalf("circular nodes must not render children:\n%s", plain)
	}
}
