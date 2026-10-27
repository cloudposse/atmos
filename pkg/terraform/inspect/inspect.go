package inspect

import (
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/pkg/perf"
)

// invalidProviderReferenceSummary is the diagnostic summary terraform-config-inspect emits for a
// `provider` argument it cannot read as a bare `name` or `name.alias` traversal.
const invalidProviderReferenceSummary = "Invalid provider reference"

// resourceBlocksSchema selects the blocks that can carry a `provider` argument.
var resourceBlocksSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "resource", LabelNames: []string{"type", "name"}},
		{Type: "data", LabelNames: []string{"type", "name"}},
	},
}

// providerArgumentSchema selects the `provider` argument of a resource or data block.
var providerArgumentSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{{Name: "provider"}},
}

// LoadModule loads the module in dir with tfconfig.LoadModule and splits its diagnostics in two.
//
// The diags result holds every diagnostic except those verified to be OpenTofu provider iteration,
// which are returned in providerIteration instead. Terraform-config-inspect records the provider
// iteration diagnostic and keeps loading, so the returned module is complete apart from those
// resources' provider references. module.Diagnostics still holds the library's full list.
func LoadModule(dir string) (module *tfconfig.Module, diags, providerIteration tfconfig.Diagnostics) {
	defer perf.Track(nil, "terraform.inspect.LoadModule")()

	module, all := tfconfig.LoadModule(dir)

	var verified map[lineKey]bool
	for _, diag := range all {
		if !isInvalidProviderReference(diag) {
			diags = append(diags, diag)
			continue
		}
		if verified == nil {
			verified = providerIterationLines(dir, all)
		}
		if verified[lineKey{diag.Pos.Filename, diag.Pos.Line}] {
			providerIteration = append(providerIteration, diag)
		} else {
			diags = append(diags, diag)
		}
	}

	return module, diags, providerIteration
}

// lineKey identifies a source line. tfconfig.Diagnostic keeps only the filename and start line of a
// diagnostic's range, so this is the finest key LoadModule's diagnostics can be matched on.
type lineKey struct {
	filename string
	line     int
}

// isInvalidProviderReference reports whether diag is terraform-config-inspect's "Invalid provider
// reference" error with a source position.
func isInvalidProviderReference(diag tfconfig.Diagnostic) bool {
	return diag.Severity == tfconfig.DiagError && diag.Summary == invalidProviderReferenceSummary && diag.Pos != nil
}

// providerIterationLines returns the source lines at which every "Invalid provider reference"
// diagnostic is an indexed provider alias. Each file named by such a diagnostic is parsed once.
func providerIterationLines(dir string, diags tfconfig.Diagnostics) map[lineKey]bool {
	lines := make(map[lineKey]bool)
	parsed := make(map[string]bool)
	parser := hclparse.NewParser()

	for _, diag := range diags {
		if !isInvalidProviderReference(diag) || parsed[diag.Pos.Filename] {
			continue
		}
		parsed[diag.Pos.Filename] = true
		for key, ok := range fileProviderIterationLines(parser, dir, diag.Pos.Filename) {
			lines[key] = ok
		}
	}

	return lines
}

// fileProviderIterationLines parses filename and reruns tfconfig.LoadModuleFromFile on it, which
// returns hcl.Diagnostics with full source ranges. It maps the start line of each "Invalid provider
// reference" diagnostic to whether every such diagnostic on that line is an indexed provider alias.
func fileProviderIterationLines(parser *hclparse.Parser, dir, filename string) map[lineKey]bool {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}

	isJSON := strings.HasSuffix(filename, ".json")
	var file *hcl.File
	var parseDiags hcl.Diagnostics
	if isJSON {
		file, parseDiags = parser.ParseJSON(src, filename)
	} else {
		file, parseDiags = parser.ParseHCL(src, filename)
	}
	if file == nil || parseDiags.HasErrors() {
		return nil
	}

	indexed := indexedProviderAliasRanges(file, isJSON)
	lines := make(map[lineKey]bool)
	for _, diag := range tfconfig.LoadModuleFromFile(file, tfconfig.NewModule(dir)) {
		if diag.Severity != hcl.DiagError || diag.Summary != invalidProviderReferenceSummary || diag.Subject == nil {
			continue
		}
		key := lineKey{filename, diag.Subject.Start.Line}
		ok, seen := lines[key]
		lines[key] = indexed[*diag.Subject] && (ok || !seen)
	}

	return lines
}

// indexedProviderAliasRanges returns the ranges of the `provider` expressions in file's resource and
// data blocks that select an indexed provider alias.
func indexedProviderAliasRanges(file *hcl.File, isJSON bool) map[hcl.Range]bool {
	ranges := make(map[hcl.Range]bool)

	content, _, _ := file.Body.PartialContent(resourceBlocksSchema)
	for _, block := range content.Blocks {
		blockContent, _, _ := block.Body.PartialContent(providerArgumentSchema)
		attr, ok := blockContent.Attributes["provider"]
		if !ok {
			continue
		}
		if isIndexedProviderAlias(providerExpression(attr.Expr, isJSON)) {
			ranges[attr.Expr.Range()] = true
		}
	}

	return ranges
}

// providerExpression returns the native-syntax expression of a `provider` argument. In native
// syntax that is the expression itself. In JSON the argument is a string holding the expression, as
// in `"provider": "aws.by_account[each.key]"`, so the string is parsed as native syntax. It returns
// nil if the expression cannot be read.
func providerExpression(expr hcl.Expression, isJSON bool) hclsyntax.Expression {
	if !isJSON {
		native, _ := expr.(hclsyntax.Expression)
		return native
	}

	val, diags := expr.Value(nil)
	if diags.HasErrors() || !val.IsKnown() || val.IsNull() || val.Type() != cty.String {
		return nil
	}

	native, diags := hclsyntax.ParseExpression([]byte(val.AsString()), expr.Range().Filename, expr.Range().Start)
	if diags.HasErrors() {
		return nil
	}
	return native
}

// isIndexedProviderAlias reports whether expr is a provider alias selected by index, as in
// `aws.by_account[each.key]` (OpenTofu 1.9+ provider iteration). The whole expression must be the
// index, so a conditional, function call or list that merely contains one does not match. A literal
// index such as `aws.by_account["dev"]` parses as a plain traversal, which terraform-config-inspect
// already accepts, so it never reaches this check.
func isIndexedProviderAlias(expr hclsyntax.Expression) bool {
	index, ok := expr.(*hclsyntax.IndexExpr)
	if !ok {
		return false
	}

	alias, ok := index.Collection.(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(alias.Traversal) != 2 {
		return false
	}

	_, ok = alias.Traversal[1].(hcl.TraverseAttr)
	return ok
}
