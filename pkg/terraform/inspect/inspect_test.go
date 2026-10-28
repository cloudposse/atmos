package inspect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeModule writes files into a new temporary module directory, with a local module the configurations
// can call, and returns the directory.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules", "example"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modules", "example", "main.tf"), []byte("# Local module.\n"), 0o644))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	return dir
}

// hclDataSource returns a native-syntax data source whose `provider` argument is providerExpr.
func hclDataSource(providerExpr string) string {
	return `
data "aws_caller_identity" "each" {
  for_each = toset(["dev", "prod"])
  provider = ` + providerExpr + `
}

module "example" {
  source = "./modules/example"
}
`
}

// hcl2Only is native syntax the legacy HCL parser cannot read. Terraform-config-inspect falls back to that
// parser when the HCL 2 loader reports errors, and for a JSON-only module the fallback succeeds and drops
// every diagnostic, so JSON test modules include this file to keep the HCL 2 diagnostics.
const hcl2Only = `
locals {
  accounts = [for account in ["dev", "prod"] : upper(account)]
}
`

// jsonDataSource returns the JSON-syntax equivalent of hclDataSource. The providerValue argument is a JSON value.
func jsonDataSource(providerValue string) string {
	return `{
  "data": {
    "aws_caller_identity": {
      "each": {
        "for_each": "${toset([\"dev\", \"prod\"])}",
        "provider": ` + providerValue + `
      }
    }
  },
  "module": {
    "example": {
      "source": "./modules/example"
    }
  }
}
`
}

// summaries returns the summaries of the error-severity diagnostics in diags.
func summaries(diags tfconfig.Diagnostics) []string {
	var out []string
	for _, diag := range diags {
		if diag.Severity == tfconfig.DiagError {
			out = append(out, diag.Summary)
		}
	}
	return out
}

// TestLoadModule tests that only an indexed provider alias is set aside as provider iteration, in both
// native and JSON syntax, and that every other diagnostic is returned to the caller.
func TestLoadModule(t *testing.T) {
	tests := []struct {
		name              string
		files             map[string]string
		providerIteration bool
		errors            []string
	}{
		{
			name:              "indexed provider alias",
			files:             map[string]string{"main.tf": hclDataSource(`aws.by_account[each.value]`)},
			providerIteration: true,
		},
		{
			name:              "indexed provider alias in JSON",
			files:             map[string]string{"main.tf.json": jsonDataSource(`"aws.by_account[each.value]"`), "locals.tf": hcl2Only},
			providerIteration: true,
		},
		{
			name:              "indexed provider alias with a nested index",
			files:             map[string]string{"main.tf": hclDataSource(`aws.by_account[local.ids["dev"]]`)},
			providerIteration: true,
		},
		{
			name: "indexed provider alias split across lines with comments and brackets in strings",
			files: map[string]string{"main.tf": hclDataSource(`aws.by_account[ # ]
    local.ids["]"] // ]
  ]`)},
			providerIteration: true,
		},
		{
			name:  "literal index is a plain traversal",
			files: map[string]string{"main.tf": hclDataSource(`aws.by_account["dev"]`)},
		},
		{
			name:   "conditional provider expression",
			files:  map[string]string{"main.tf": hclDataSource(`var.use_dev ? aws.dev : aws.prod`)},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name:   "conditional provider expression in JSON",
			files:  map[string]string{"main.tf.json": jsonDataSource(`"var.use_dev ? aws.dev : aws.prod"`), "locals.tf": hcl2Only},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name:   "conditional after an indexed provider alias",
			files:  map[string]string{"main.tf": hclDataSource(`aws.by_account[each.value] ? aws.dev : aws.prod`)},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name: "comment bracket before a conditional",
			files: map[string]string{"main.tf": hclDataSource(`aws.by_account[ # ]
    each.value
  ] ? aws.dev : aws.prod`)},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name:   "function call",
			files:  map[string]string{"main.tf": hclDataSource(`lookup(local.providers, "dev")`)},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name:   "list",
			files:  map[string]string{"main.tf": hclDataSource(`[aws.dev]`)},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name:   "deeper traversal before the index",
			files:  map[string]string{"main.tf": hclDataSource(`aws.by_account.extra[each.value]`)},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name:   "non-string provider in JSON",
			files:  map[string]string{"main.tf.json": jsonDataSource(`42`), "locals.tf": hcl2Only},
			errors: []string{invalidProviderReferenceSummary},
		},
		{
			name: "provider iteration plus an unrelated error",
			files: map[string]string{
				"main.tf": hclDataSource(`aws.by_account[each.value]`),
				"other.tf": `
terraform {
  required_version = var.terraform_version
}
`,
			},
			providerIteration: true,
			errors:            []string{"Variables not allowed", "Unsuitable value type"},
		},
		{
			name: "provider iteration in one file and a conditional in another",
			files: map[string]string{
				"main.tf": hclDataSource(`aws.by_account[each.value]`),
				"other.tf": `
resource "aws_s3_bucket" "bad" {
  provider = var.use_dev ? aws.dev : aws.prod
}
`,
			},
			providerIteration: true,
			errors:            []string{invalidProviderReferenceSummary},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeModule(t, tt.files)

			module, diags, providerIteration := LoadModule(dir)
			require.NotNil(t, module)

			assert.Equal(t, tt.errors, summaries(diags))
			if tt.providerIteration {
				require.Len(t, providerIteration, 1)
				assert.Equal(t, invalidProviderReferenceSummary, providerIteration[0].Summary)
				assert.True(t, strings.HasPrefix(filepath.Base(providerIteration[0].Pos.Filename), "main.tf"))
			} else {
				assert.Empty(t, providerIteration)
			}

			// Terraform-config-inspect keeps loading after the diagnostic, so the module call is still found.
			require.Contains(t, module.ModuleCalls, "example")
			assert.Equal(t, "./modules/example", module.ModuleCalls["example"].Source)
		})
	}
}

// TestLoadModuleMissingDirectory tests that a missing directory is reported as a diagnostic, not set aside.
func TestLoadModuleMissingDirectory(t *testing.T) {
	_, diags, providerIteration := LoadModule(filepath.Join(t.TempDir(), "missing"))

	assert.Equal(t, []string{"Failed to read directory"}, summaries(diags))
	assert.Empty(t, providerIteration)
}

// TestIsIndexedProviderAlias tests the expression shape accepted as provider iteration.
func TestIsIndexedProviderAlias(t *testing.T) {
	tests := []struct {
		expr     string
		expected bool
	}{
		{expr: `aws.by_account[each.key]`, expected: true},
		{expr: `aws.by_account[local.ids[each.key]]`, expected: true},
		{expr: `aws[each.key]`, expected: false},
		{expr: `aws.by_account.extra[each.key]`, expected: false},
		{expr: `aws.by_account[each.key].extra`, expected: false},
		{expr: `aws.by_account[each.key] ? aws.dev : aws.prod`, expected: false},
		{expr: `(aws.by_account)[each.key]`, expected: false},
		{expr: `aws.dev`, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			expr, diags := hclsyntax.ParseExpression([]byte(tt.expr), "test.tf", hcl.InitialPos)
			require.False(t, diags.HasErrors(), diags.Error())
			assert.Equal(t, tt.expected, isIndexedProviderAlias(expr))
		})
	}

	assert.False(t, isIndexedProviderAlias(nil))
}
