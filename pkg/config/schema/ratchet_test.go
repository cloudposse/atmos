package configschema

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// This file is the "you can't forget the schema" ratchet for atmos.yaml. The
// schema itself is generated from schema.AtmosConfiguration, so it cannot drift
// structurally — but two curated lists in overrides.go still require a human
// decision when the code grows:
//
//  1. excludedRootFields — every AtmosConfiguration field must be authored
//     config, runtime-only (`yaml:"-"`), or explicitly excluded.
//  2. atmosConfigYamlFunctions — every AtmosYamlFunc* constant must be
//     classified as supported in atmos.yaml or stack-manifest-only.
//
// These tests fail the build until a new field or YAML function is classified.

// TestEveryRootFieldIsClassified asserts each AtmosConfiguration field is
// exactly one of: skipped (`yaml:"-"`), excluded as runtime-computed
// (excludedRootFields), or present in the generated schema.
func TestEveryRootFieldIsClassified(t *testing.T) {
	props := rootProperties(t)
	excluded := make(map[string]bool, len(excludedRootFields))
	for _, field := range excludedRootFields {
		excluded[field] = false
	}

	rt := reflect.TypeOf(schema.AtmosConfiguration{})
	for i := range rt.NumField() {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		name := yamlTagName(&field)
		if name == "-" {
			continue
		}

		_, inSchema := props[name]
		_, inExcluded := excluded[name]
		switch {
		case inExcluded && inSchema:
			t.Errorf("AtmosConfiguration.%s (%q) is listed in excludedRootFields but still appears in the generated schema; "+
				"remove it from one of the two", field.Name, name)
		case inExcluded:
			excluded[name] = true
		case !inSchema:
			t.Errorf("new AtmosConfiguration field %s (%q) is not classified: either it is authored atmos.yaml config "+
				"(run `go generate ./pkg/config/schema` and commit the artifact), it is runtime-only (tag it `yaml:\"-\"`), "+
				"or it is runtime-computed but serialized (add %q to excludedRootFields in pkg/config/schema/overrides.go)",
				field.Name, name, name)
		}
	}

	for field, used := range excluded {
		assert.True(t, used,
			"excludedRootFields entry %q matches no AtmosConfiguration field; remove the dead entry", field)
	}
}

// yamlTagName returns the property name reflection derives for a struct field —
// the first yaml tag segment, falling back to the Go field name.
func yamlTagName(field *reflect.StructField) string {
	name := strings.Split(field.Tag.Get("yaml"), ",")[0]
	if name == "" {
		return field.Name
	}
	return name
}

// yamlFunctionClassification maps every AtmosYamlFunc* constant name to its tag
// value. TestEveryYamlFunctionIsClassified fails when pkg/utils/yaml_utils.go
// declares a constant that is missing here, forcing a decision: add the value to
// atmosConfigYamlFunctions (overrides.go) when atmos.yaml preprocessing supports
// it (the dispatch in pkg/config/process_yaml.go), or list its name in
// stackManifestOnlyYamlFunctions when it is only valid in stack manifests.
var yamlFunctionClassification = map[string]string{
	"AtmosYamlFuncAppend":                  u.AtmosYamlFuncAppend,
	"AtmosYamlFuncAwsAccountID":            u.AtmosYamlFuncAwsAccountID,
	"AtmosYamlFuncAwsCallerIdentityArn":    u.AtmosYamlFuncAwsCallerIdentityArn,
	"AtmosYamlFuncAwsCallerIdentityUserID": u.AtmosYamlFuncAwsCallerIdentityUserID,
	"AtmosYamlFuncAwsOrganizationID":       u.AtmosYamlFuncAwsOrganizationID,
	"AtmosYamlFuncAwsRegion":               u.AtmosYamlFuncAwsRegion,
	"AtmosYamlFuncCEL":                     u.AtmosYamlFuncCEL,
	"AtmosYamlFuncCwd":                     u.AtmosYamlFuncCwd,
	"AtmosYamlFuncEmulator":                u.AtmosYamlFuncEmulator,
	"AtmosYamlFuncEnv":                     u.AtmosYamlFuncEnv,
	"AtmosYamlFuncExec":                    u.AtmosYamlFuncExec,
	"AtmosYamlFuncGitBranch":               u.AtmosYamlFuncGitBranch,
	"AtmosYamlFuncGitHost":                 u.AtmosYamlFuncGitHost,
	"AtmosYamlFuncGitName":                 u.AtmosYamlFuncGitName,
	"AtmosYamlFuncGitOwner":                u.AtmosYamlFuncGitOwner,
	"AtmosYamlFuncGitRef":                  u.AtmosYamlFuncGitRef,
	"AtmosYamlFuncGitRepository":           u.AtmosYamlFuncGitRepository,
	"AtmosYamlFuncGitRoot":                 u.AtmosYamlFuncGitRoot,
	"AtmosYamlFuncGitRootAlias":            u.AtmosYamlFuncGitRootAlias,
	"AtmosYamlFuncGitSha":                  u.AtmosYamlFuncGitSha,
	"AtmosYamlFuncGitUrl":                  u.AtmosYamlFuncGitUrl,
	"AtmosYamlFuncInclude":                 u.AtmosYamlFuncInclude,
	"AtmosYamlFuncIncludeRaw":              u.AtmosYamlFuncIncludeRaw,
	"AtmosYamlFuncLabels":                  u.AtmosYamlFuncLabels,
	"AtmosYamlFuncLabelsKeys":              u.AtmosYamlFuncLabelsKeys,
	"AtmosYamlFuncLabelsValues":            u.AtmosYamlFuncLabelsValues,
	"AtmosYamlFuncLiteral":                 u.AtmosYamlFuncLiteral,
	"AtmosYamlFuncRandom":                  u.AtmosYamlFuncRandom,
	"AtmosYamlFuncSecret":                  u.AtmosYamlFuncSecret,
	"AtmosYamlFuncStore":                   u.AtmosYamlFuncStore,
	"AtmosYamlFuncStoreGet":                u.AtmosYamlFuncStoreGet,
	"AtmosYamlFuncTags":                    u.AtmosYamlFuncTags,
	"AtmosYamlFuncTemplate":                u.AtmosYamlFuncTemplate,
	"AtmosYamlFuncTerraformOutput":         u.AtmosYamlFuncTerraformOutput,
	"AtmosYamlFuncTerraformState":          u.AtmosYamlFuncTerraformState,
	"AtmosYamlFuncUnset":                   u.AtmosYamlFuncUnset,
	"AtmosYamlFuncVersion":                 u.AtmosYamlFuncVersion,
}

// stackManifestOnlyYamlFunctions are YAML functions NOT supported when loading
// atmos.yaml (the dispatch in pkg/config/process_yaml.go rejects them), so the
// generated schema's yamlFunction pattern must not admit them.
var stackManifestOnlyYamlFunctions = map[string]bool{
	"AtmosYamlFuncAwsAccountID":            true,
	"AtmosYamlFuncAwsCallerIdentityArn":    true,
	"AtmosYamlFuncAwsCallerIdentityUserID": true,
	"AtmosYamlFuncAwsOrganizationID":       true,
	"AtmosYamlFuncAwsRegion":               true,
	"AtmosYamlFuncCEL":                     true,
	"AtmosYamlFuncEmulator":                true,
	"AtmosYamlFuncLabels":                  true,
	"AtmosYamlFuncLabelsKeys":              true,
	"AtmosYamlFuncLabelsValues":            true,
	"AtmosYamlFuncLiteral":                 true,
	"AtmosYamlFuncSecret":                  true,
	"AtmosYamlFuncStore":                   true,
	"AtmosYamlFuncStoreGet":                true,
	"AtmosYamlFuncTags":                    true,
	"AtmosYamlFuncTemplate":                true,
	"AtmosYamlFuncTerraformOutput":         true,
	"AtmosYamlFuncTerraformState":          true,
	"AtmosYamlFuncVersion":                 true,
}

// TestEveryYamlFunctionIsClassified asserts every AtmosYamlFunc* constant in
// pkg/utils/yaml_utils.go is classified as atmos.yaml-supported or
// stack-manifest-only, and that the classification is consistent with the
// generated yamlFunction pattern.
func TestEveryYamlFunctionIsClassified(t *testing.T) {
	declared := declaredYamlFunctionConstants(t)
	require.Positive(t, len(declared), "no AtmosYamlFunc* constants found; the AST scan is misconfigured")

	supported := make(map[string]bool, len(atmosConfigYamlFunctions))
	for _, tag := range atmosConfigYamlFunctions {
		supported[tag] = true
	}

	for _, name := range declared {
		value, classified := yamlFunctionClassification[name]
		if !classified {
			t.Errorf("new YAML function constant %s is not classified: add it to yamlFunctionClassification in this file, "+
				"then either add its value to atmosConfigYamlFunctions (pkg/config/schema/overrides.go) if atmos.yaml "+
				"preprocessing supports it (pkg/config/process_yaml.go), or to stackManifestOnlyYamlFunctions if it is "+
				"stack-manifest-only; regenerate with `go generate ./pkg/config/schema`", name)
			continue
		}
		manifestOnly := stackManifestOnlyYamlFunctions[name]
		if manifestOnly == supported[value] {
			t.Errorf("YAML function %s (%q) must be exactly one of atmosConfigYamlFunctions or stackManifestOnlyYamlFunctions", name, value)
		}
	}

	declaredSet := make(map[string]bool, len(declared))
	for _, name := range declared {
		declaredSet[name] = true
	}
	for name := range yamlFunctionClassification {
		assert.True(t, declaredSet[name],
			"yamlFunctionClassification entry %s matches no AtmosYamlFunc* constant; remove the dead entry", name)
	}
}

// TestYamlFunctionPatternMatchesSupportedForms asserts the generated pattern
// accepts authored function forms and rejects manifest-only and plain strings.
func TestYamlFunctionPatternMatchesSupportedForms(t *testing.T) {
	pattern := yamlFunctionPattern()

	accepted := []string{
		"!include shared.yaml",
		"!include.raw ./snippet.txt",
		"!env ATMOS_BASE_PATH",
		"!exec echo hello",
		"!repo-root",
		"!git.root .",
		"!cwd",
		"!random",
	}
	rejected := []string{
		"!terraform.output vpc dev vpc_id",
		"!store ssm dev vpc id",
		"!secret app/api_key",
		"!includexyz not-a-function",
		"plain string",
	}

	for _, form := range accepted {
		assert.Regexp(t, pattern, form, "atmos.yaml-supported form %q must match the yamlFunction pattern", form)
	}
	for _, form := range rejected {
		assert.NotRegexp(t, pattern, form, "form %q must not match the yamlFunction pattern", form)
	}
}

// declaredYamlFunctionConstants AST-parses pkg/utils/yaml_utils.go and returns
// the names of all AtmosYamlFunc* constants.
func declaredYamlFunctionConstants(t *testing.T) []string {
	t.Helper()

	repoRoot, err := RepoRoot()
	require.NoError(t, err)
	src := filepath.Join(repoRoot, "pkg", "utils", "yaml_utils.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, src, nil, 0)
	require.NoError(t, err)

	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, ident := range valueSpec.Names {
				if strings.HasPrefix(ident.Name, "AtmosYamlFunc") {
					names = append(names, ident.Name)
				}
			}
		}
	}
	return names
}
