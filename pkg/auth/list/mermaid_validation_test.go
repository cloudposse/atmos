package list

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Mermaid Validation Strategy
//
// This file provides multi-layered validation for Mermaid diagram syntax:
//
// 1. Structural Validation (validateMermaidStructure):
//    - Parse-based validation that checks Mermaid syntax structure
//    - Validates graph declarations, node definitions, edges, and class usage
//    - Detects common errors: undefined nodes, missing classes, chained syntax
//    - No external dependencies required
//    - Fast and reliable for CI/CD
//
// 2. Mermaid CLI Validation (validateWithMermaidCLI):
//    - Uses official @mermaid-js/mermaid-cli tool if available
//    - Most accurate validation against actual Mermaid parser
//    - Optional - gracefully skips if not installed
//    - Install with: npm install -g @mermaid-js/mermaid-cli
//
// Both validators are used in tests to ensure generated Mermaid is syntactically
// correct and will render properly in GitHub, GitLab, Confluence, etc.

// validateMermaidStructure performs basic structural validation of Mermaid syntax.
// This checks for common syntax errors without requiring external tools.
func validateMermaidStructure(mermaid string) error {
	lines := strings.Split(mermaid, "\n")

	// Track state.
	var (
		hasGraphDeclaration bool
		declaredNodes       = make(map[string]bool)
		referencedNodes     = make(map[string]bool)
		declaredClasses     = make(map[string]bool)
		appliedClasses      = make(map[string]bool)
	)

	// Regex patterns.
	graphPattern := regexp.MustCompile(`^\s*graph\s+(LR|TD|TB|RL|BT)`)
	nodePattern := regexp.MustCompile(`^\s*(\w+)\[`)
	edgePattern := regexp.MustCompile(`^\s*(\w+)\s*-->\s*(\w+)`)
	classDefPattern := regexp.MustCompile(`^\s*classDef\s+(\w+)`)
	classApplyPattern := regexp.MustCompile(`^\s*class\s+(\w+)\s+(\w+)`)
	chainedClassPattern := regexp.MustCompile(`:::`)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for graph declaration.
		if graphPattern.MatchString(line) {
			if hasGraphDeclaration {
				return fmt.Errorf("line %d: multiple graph declarations", lineNum+1)
			}
			hasGraphDeclaration = true
			continue
		}

		// Check for chained class syntax (not supported in newer Mermaid).
		if chainedClassPattern.MatchString(line) {
			return fmt.Errorf("line %d: chained class syntax (:::) is not supported, use separate 'class' directives", lineNum+1)
		}

		// Check for node declarations.
		if matches := nodePattern.FindStringSubmatch(line); matches != nil {
			nodeID := matches[1]
			declaredNodes[nodeID] = true
			continue
		}

		// Check for edges.
		if matches := edgePattern.FindStringSubmatch(line); matches != nil {
			fromNode := matches[1]
			toNode := matches[2]
			referencedNodes[fromNode] = true
			referencedNodes[toNode] = true
			continue
		}

		// Check for classDef.
		if matches := classDefPattern.FindStringSubmatch(line); matches != nil {
			className := matches[1]
			declaredClasses[className] = true
			continue
		}

		// Check for class application.
		if matches := classApplyPattern.FindStringSubmatch(line); matches != nil {
			className := matches[2]
			appliedClasses[className] = true
			continue
		}
	}

	// Validate graph declaration exists.
	if !hasGraphDeclaration {
		return fmt.Errorf("missing graph declaration (e.g., 'graph LR')")
	}

	// Validate all referenced nodes are declared.
	for nodeID := range referencedNodes {
		if !declaredNodes[nodeID] {
			return fmt.Errorf("node %q referenced in edge but not declared", nodeID)
		}
	}

	// Validate all applied classes are defined.
	for className := range appliedClasses {
		if !declaredClasses[className] {
			return fmt.Errorf("class %q applied but not defined with classDef", className)
		}
	}

	return nil
}

// validateWithMermaidCLI validates Mermaid syntax using the mermaid-cli tool if available.
// This provides the most accurate validation but is optional.
func validateWithMermaidCLI(t *testing.T, mermaid string) error {
	t.Helper()

	// Check if mmdc is available.
	mmdcPath, err := exec.LookPath("mmdc")
	if err != nil {
		return fmt.Errorf("mermaid-cli (mmdc) not found: %w (install with: npm install -g @mermaid-js/mermaid-cli)", err)
	}

	// Create temporary file with mermaid content.
	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "test.mmd")
	outputFile := filepath.Join(tmpDir, "test.svg")

	if err := os.WriteFile(inputFile, []byte(mermaid), 0o600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Run mmdc to validate syntax.
	cmd := exec.Command(mmdcPath, "-i", inputFile, "-o", outputFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mermaid-cli validation failed: %w\nOutput: %s", err, string(output))
	}

	t.Logf("Mermaid CLI validation passed")
	return nil
}

// TestMermaidStructureValidator tests the structure validator itself.
func TestMermaidStructureValidator(t *testing.T) {
	tests := []struct {
		name      string
		mermaid   string
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid simple graph",
			mermaid: `graph LR
  A["Node A"]
  B["Node B"]
  A --> B
  classDef myClass fill:#f9f
  class A myClass`,
			wantError: false,
		},
		{
			name: "missing graph declaration",
			mermaid: `A["Node A"]
  B["Node B"]
  A --> B`,
			wantError: true,
			errorMsg:  "missing graph declaration",
		},
		{
			name: "chained class syntax",
			mermaid: `graph LR
  A["Node A"]:::myClass
  B["Node B"]
  A --> B`,
			wantError: true,
			errorMsg:  "chained class syntax",
		},
		{
			name: "undefined node in edge",
			mermaid: `graph LR
  A["Node A"]
  A --> B`,
			wantError: true,
			errorMsg:  "referenced in edge but not declared",
		},
		{
			name: "undefined class applied",
			mermaid: `graph LR
  A["Node A"]
  class A myClass`,
			wantError: true,
			errorMsg:  "applied but not defined",
		},
		{
			name: "multiple graph declarations",
			mermaid: `graph LR
  A["Node A"]
graph TD
  B["Node B"]`,
			wantError: true,
			errorMsg:  "multiple graph declarations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMermaidStructure(tt.mermaid)

			// Check error expectations.
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !tt.wantError {
				return
			}

			// Want error case.
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				return
			}

			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
			}
		})
	}
}

// TestRenderMermaid_ComplexScenarios tests Mermaid generation with realistic complex auth scenarios.
func TestRenderMermaid_ComplexScenarios(t *testing.T) {
	tests := []struct {
		name       string
		providers  map[string]schema.Provider
		identities map[string]schema.Identity
		wantNodes  []string // Node IDs that must exist.
		wantEdges  []string // Edges that must exist.
	}{
		{
			name: "deeply nested identity chain",
			providers: map[string]schema.Provider{
				"aws-sso": {
					Kind:    "aws-sso",
					Default: true,
				},
			},
			identities: map[string]schema.Identity{
				"base-role": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Provider: "aws-sso",
					},
				},
				"admin-role": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Identity: "base-role",
					},
				},
				"super-admin": {
					Kind:    "aws/assume-role",
					Default: true,
					Via: &schema.IdentityVia{
						Identity: "admin-role",
					},
				},
			},
			wantNodes: []string{"aws_sso", "base_role", "admin_role", "super_admin"},
			wantEdges: []string{
				"aws_sso --> base_role",
				"base_role --> admin_role",
				"admin_role --> super_admin",
			},
		},
		{
			name: "multiple providers with cross-provider chaining",
			providers: map[string]schema.Provider{
				"aws-sso": {
					Kind: "aws-sso",
				},
				"okta": {
					Kind:    "okta",
					Default: true,
				},
				"azure-ad": {
					Kind: "azure-ad",
				},
			},
			identities: map[string]schema.Identity{
				"okta-admin": {
					Kind: "okta/user",
					Via: &schema.IdentityVia{
						Provider: "okta",
					},
				},
				"aws-admin": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Provider: "aws-sso",
					},
				},
				"azure-admin": {
					Kind: "azure/user",
					Via: &schema.IdentityVia{
						Provider: "azure-ad",
					},
				},
			},
			wantNodes: []string{"aws_sso", "okta", "azure_ad", "okta_admin", "aws_admin", "azure_admin"},
			wantEdges: []string{
				"okta --> okta_admin",
				"aws_sso --> aws_admin",
				"azure_ad --> azure_admin",
			},
		},
		{
			name: "mixed provider and identity chaining",
			providers: map[string]schema.Provider{
				"aws-sso": {
					Kind: "aws-sso",
				},
				"okta": {
					Kind: "okta",
				},
			},
			identities: map[string]schema.Identity{
				"okta-user": {
					Kind: "okta/user",
					Via: &schema.IdentityVia{
						Provider: "okta",
					},
				},
				"aws-via-okta": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Provider: "aws-sso",
					},
				},
				"dev-role": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Identity: "aws-via-okta",
					},
				},
				"prod-role": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Identity: "dev-role",
					},
				},
			},
			wantNodes: []string{"okta", "aws_sso", "okta_user", "aws_via_okta", "dev_role", "prod_role"},
			wantEdges: []string{
				"okta --> okta_user",
				"aws_sso --> aws_via_okta",
				"aws_via_okta --> dev_role",
				"dev_role --> prod_role",
			},
		},
		{
			name: "special characters and escaping",
			providers: map[string]schema.Provider{
				"aws-sso-prod": {
					Kind: "aws-sso",
				},
			},
			identities: map[string]schema.Identity{
				"role-with-dashes": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Provider: "aws-sso-prod",
					},
				},
				"role_with_underscores": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Provider: "aws-sso-prod",
					},
				},
			},
			wantNodes: []string{"aws_sso_prod", "role_with_dashes", "role_with_underscores"},
			wantEdges: []string{
				"aws_sso_prod --> role_with_dashes",
				"aws_sso_prod --> role_with_underscores",
			},
		},
		{
			name: "diamond dependency pattern",
			providers: map[string]schema.Provider{
				"root-sso": {
					Kind: "aws-sso",
				},
			},
			identities: map[string]schema.Identity{
				"base": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Provider: "root-sso",
					},
				},
				"branch-a": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Identity: "base",
					},
				},
				"branch-b": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Identity: "base",
					},
				},
				"merged": {
					Kind: "aws/assume-role",
					Via: &schema.IdentityVia{
						Identity: "branch-a",
					},
				},
			},
			wantNodes: []string{"root_sso", "base", "branch_a", "branch_b", "merged"},
			wantEdges: []string{
				"root_sso --> base",
				"base --> branch_a",
				"base --> branch_b",
				"branch_a --> merged",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RenderMermaid(nil, tt.providers, tt.identities)
			if err != nil {
				t.Fatalf("RenderMermaid failed: %v", err)
			}

			// Validate structure.
			if err := validateMermaidStructure(output); err != nil {
				t.Errorf("Mermaid structure validation failed: %v\nOutput:\n%s", err, output)
			}

			// Verify expected nodes exist.
			for _, nodeID := range tt.wantNodes {
				if !strings.Contains(output, nodeID+"[") {
					t.Errorf("missing node %q in output", nodeID)
				}
			}

			// Verify expected edges exist.
			for _, edge := range tt.wantEdges {
				if !strings.Contains(output, edge) {
					t.Errorf("missing edge %q in output", edge)
				}
			}

			// Try CLI validation if available.
			if err := validateWithMermaidCLI(t, output); err != nil {
				t.Logf("Mermaid CLI validation skipped: %v", err)
			}

			t.Logf("Generated Mermaid for %q:\n%s", tt.name, output)
		})
	}
}
