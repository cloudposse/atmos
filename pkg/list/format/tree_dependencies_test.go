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
			Type:      "terraform",
			DependsOn: []*DepTreeNode{
				{
					Component: "db",
					Stack:     "dev",
					Type:      "terraform",
					Children: []*DepTreeNode{
						{Component: "network", Stack: "shared", Type: "helmfile"},
					},
				},
			},
		},
	})
	plain := stripANSI(output)

	for _, want := range []string{"Dependencies", "Stack", "Type", "Component", "dev", "terraform", "app", "db", "shared", "helmfile", "network"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, plain)
		}
	}
	for _, unwanted := range []string{"dev/app", "dev/db", "shared/network"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("expected output not to contain slash-qualified label %q:\n%s", unwanted, plain)
		}
	}
	for _, want := range []string{"app", "└──▶ db", "└──▶ network"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected component tree to contain %q:\n%s", want, plain)
		}
	}
	assertLineContains(t, plain, "dev", "app")
	assertLineContains(t, plain, "terraform", "app")
	assertTokenOrder(t, plain, "app", "app", "terraform")
	assertLineOmits(t, plain, "dev", "▶ db")
	assertLineOmits(t, plain, "terraform", "▶ db")
	assertLineContains(t, plain, "shared", "▶ network")
	assertLineContains(t, plain, "helmfile", "▶ network")
	assertTokenOrder(t, plain, "▶ network", "▶ network", "helmfile")
	assertNoRootTrunk(t, plain)
	if strings.Contains(plain, "depends on") || strings.Contains(plain, "required by") {
		t.Fatalf("single-direction output should not include direction labels:\n%s", plain)
	}
}

func TestRenderDependenciesTree_BothDirectionsAndEmptyBranches(t *testing.T) {
	output := RenderDependenciesTree([]*DepTreeEntry{
		{
			Component: "app",
			Stack:     "dev",
			Type:      "terraform",
			ShowBoth:  true,
			DependsOn: []*DepTreeNode{
				{Component: "db", Stack: "dev", Type: "terraform"},
			},
			RequiredBy: nil,
		},
	})
	plain := stripANSI(output)

	for _, want := range []string{"Stack", "Type", "Component", "dev", "terraform", "app", "depends on", "required by", "db", "(none)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, plain)
		}
	}
	for _, unwanted := range []string{"dev/app", "dev/db"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("expected output not to contain slash-qualified label %q:\n%s", unwanted, plain)
		}
	}
	for _, want := range []string{"app", "├──depends on", "└──▶ db", "└──required by"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected component tree to contain %q:\n%s", want, plain)
		}
	}
	assertLineContains(t, plain, "dev", "app")
	assertLineContains(t, plain, "terraform", "app")
	assertTokenOrder(t, plain, "app", "app", "terraform")
	assertLineOmits(t, plain, "dev", "▶ db")
	assertLineOmits(t, plain, "terraform", "▶ db")
	assertNoRootTrunk(t, plain)
}

func TestRenderDependenciesTree_TypeColumnAlignsAcrossComponentWidths(t *testing.T) {
	output := RenderDependenciesTree([]*DepTreeEntry{
		{Component: "app", Stack: "dev", Type: "terraform"},
		{Component: "dynamodb-table", Stack: "dev", Type: "terraform"},
	})
	plain := stripANSI(output)

	appTypeColumn := columnOfTokenOnLine(t, plain, "app", "terraform")
	tableTypeColumn := columnOfTokenOnLine(t, plain, "dynamodb-table", "terraform")
	if appTypeColumn != tableTypeColumn {
		t.Fatalf("expected Type column to align, got app=%d dynamodb-table=%d:\n%s", appTypeColumn, tableTypeColumn, plain)
	}
}

func TestRenderDependenciesTree_RequiredByFallbackAndCircular(t *testing.T) {
	output := RenderDependenciesTree([]*DepTreeEntry{
		{
			Component: "db",
			Stack:     "dev",
			Type:      "terraform",
			RequiredBy: []*DepTreeNode{
				{
					Component: "app",
					Stack:     "dev",
					Type:      "terraform",
					Circular:  true,
					Children: []*DepTreeNode{
						{Component: "should-not-render", Stack: "dev", Type: "terraform"},
					},
				},
			},
		},
	})
	plain := stripANSI(output)

	for _, want := range []string{"Stack", "Type", "Component", "dev", "terraform", "db", "app (circular reference)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, plain)
		}
	}
	for _, unwanted := range []string{"dev/db", "dev/app"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("expected output not to contain slash-qualified label %q:\n%s", unwanted, plain)
		}
	}
	for _, want := range []string{"db", "└──◀ app (circular reference)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected component tree to contain %q:\n%s", want, plain)
		}
	}
	assertLineContains(t, plain, "dev", "db")
	assertLineContains(t, plain, "terraform", "db")
	assertTokenOrder(t, plain, "db", "db", "terraform")
	assertLineOmits(t, plain, "dev", "◀ app (circular reference)")
	assertLineOmits(t, plain, "terraform", "◀ app (circular reference)")
	assertNoRootTrunk(t, plain)
	if strings.Contains(plain, "should-not-render") {
		t.Fatalf("circular nodes must not render children:\n%s", plain)
	}
}

func assertLineContains(t *testing.T, output, want, token string) {
	t.Helper()

	line := lineContaining(output, token)
	if line == "" {
		t.Fatalf("expected line containing %q:\n%s", token, output)
	}
	if !strings.Contains(line, want) {
		t.Fatalf("expected line containing %q to include %q, got %q:\n%s", token, want, line, output)
	}
}

func assertLineOmits(t *testing.T, output, unwanted, token string) {
	t.Helper()

	line := lineContaining(output, token)
	if line == "" {
		t.Fatalf("expected line containing %q:\n%s", token, output)
	}
	if strings.Contains(line, unwanted) {
		t.Fatalf("expected line containing %q to omit %q, got %q:\n%s", token, unwanted, line, output)
	}
}

func lineContaining(output, token string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, token) {
			return line
		}
	}
	return ""
}

func assertTokenOrder(t *testing.T, output, lineToken, first, second string) {
	t.Helper()

	line := lineContaining(output, lineToken)
	if line == "" {
		t.Fatalf("expected line containing %q:\n%s", lineToken, output)
	}
	firstIndex := strings.Index(line, first)
	secondIndex := strings.Index(line, second)
	if firstIndex < 0 || secondIndex < 0 {
		t.Fatalf("expected line containing %q to include %q and %q, got %q:\n%s", lineToken, first, second, line, output)
	}
	if firstIndex >= secondIndex {
		t.Fatalf("expected %q to appear before %q in line %q:\n%s", first, second, line, output)
	}
}

func columnOfTokenOnLine(t *testing.T, output, lineToken, token string) int {
	t.Helper()

	line := lineContaining(output, lineToken)
	if line == "" {
		t.Fatalf("expected line containing %q:\n%s", lineToken, output)
	}
	index := strings.Index(line, token)
	if index < 0 {
		t.Fatalf("expected line containing %q to include %q, got %q:\n%s", lineToken, token, line, output)
	}
	return len([]rune(line[:index]))
}

func assertNoRootTrunk(t *testing.T, output string) {
	t.Helper()

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "│" {
			t.Fatalf("expected output not to contain root trunk line:\n%s", output)
		}
	}
}
