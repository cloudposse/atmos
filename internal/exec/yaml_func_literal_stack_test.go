package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestStackRandomAndCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	input := explicitYAMLFunctionsForTest(map[string]any{"random": "!random 49152 65535", "cwd": "!cwd", "nested": []any{"!cwd child"}})
	got, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", nil, nil)
	require.NoError(t, err)
	port, ok := got["random"].(int)
	require.True(t, ok, "random must produce a number, got %T", got["random"])
	assert.GreaterOrEqual(t, port, 49152)
	assert.LessOrEqual(t, port, 65535)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, cwd, got["cwd"])
	assert.Equal(t, []any{filepath.Join(cwd, "child")}, got["nested"])
	skipped, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", []string{"random", "cwd"}, nil)
	require.NoError(t, err)
	assert.Equal(t, input, skipped)
	_, err = ProcessCustomYamlTags(&schema.AtmosConfiguration{}, explicitYAMLFunctionsForTest(map[string]any{"x": "!random 3 2"}), "dev", nil, nil)
	assert.ErrorIs(t, err, u.ErrInvalidAtmosYAMLFunction)
}

func TestStackLiteralSurvivesEvaluation(t *testing.T) {
	for _, templates := range []bool{false, true} {
		for _, functions := range []bool{false, true} {
			t.Run(fmtBoolPair(templates, functions), func(t *testing.T) {
				writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        external: !literal "{{external.email}}"
        helm: !literal "{{ .Values.ingress.class }}"
        function: !literal "!env ATMOS_LITERAL_TEST"
        whitespace: !literal "  {{ external }}  \n"
        list: [!literal "{{external.email}}", plain]
`)
				t.Setenv("ATMOS_LITERAL_TEST", "must-not-resolve")
				got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
					Component: "app", Stack: "dev", ProcessTemplates: templates, ProcessYamlFunctions: functions,
				})
				require.NoError(t, err)
				vars := got["vars"].(map[string]any)
				assert.Equal(t, "{{external.email}}", vars["external"])
				assert.Equal(t, "{{ .Values.ingress.class }}", vars["helm"])
				assert.Equal(t, "!env ATMOS_LITERAL_TEST", vars["function"])
				assert.Equal(t, "  {{ external }}  \n", vars["whitespace"])
				assert.Equal(t, []any{"{{external.email}}", "plain"}, vars["list"])
			})
		}
	}
}

func fmtBoolPair(templates, functions bool) string {
	if templates {
		if functions {
			return "templates-and-functions"
		}
		return "templates-only"
	}
	if functions {
		return "functions-only"
	}
	return "no-evaluation"
}

func TestStackLiteralImportsAndOverrides(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml", `import: [catalog/base]
vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        overridden: "{{ .vars.stage }}"
        new_literal: !literal "{{external.email}}"
        included: !include ../values.yaml | eval
        reference: '{{ .vars.inherited }}'
        length: '{{ len .vars.inherited }}'
`)
	require.NoError(t, os.MkdirAll("stacks/catalog", 0o755))
	require.NoError(t, os.WriteFile("stacks/catalog/base.yaml", []byte(`components:
  terraform:
    app:
      vars:
        inherited: !literal "hello"
        overridden: !literal "{{external.email}}"
        new_literal: "{{ .vars.stage }}"
`), 0o600))
	require.NoError(t, os.WriteFile("stacks/values.yaml", []byte("nested: !literal '{{external.email}}'\n"), 0o600))
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	vars := got["vars"].(map[string]any)
	assert.Equal(t, "hello", vars["inherited"])
	assert.Equal(t, "dev", vars["overridden"])
	assert.Equal(t, "{{external.email}}", vars["new_literal"])
	assert.Equal(t, map[string]any{"nested": "{{external.email}}"}, vars["included"])
	assert.Equal(t, "hello", vars["reference"])
	assert.Equal(t, "5", vars["length"])
}

func TestStackLiteralTemplateFile(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml.tmpl", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
{{ if true }}
        value: !literal "{{external.email}}"
{{ end }}
`)
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "{{external.email}}", got["vars"].(map[string]any)["value"])
}

func TestStackLiteralInlineTemplateSource(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml.tmpl", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        {{ if true }}value: !literal "{{external.email}}"{{ end }}
        {{ printf "dynamic" }}: {{ printf "%q" "rendered" }}
        block: !literal |-
          {{external.body}}
`)
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	vars := got["vars"].(map[string]any)
	assert.Equal(t, "{{external.email}}", vars["value"])
	assert.Equal(t, "rendered", vars["dynamic"])
	assert.Equal(t, "{{external.body}}", vars["block"])
}

func TestStackLiteralMultiplePasses(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        value: !literal "{{external.email}}"
        computed: !starlark 'return len(ctx.vars["value"])'
`)
	file, err := os.OpenFile("atmos.yaml", os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteString("    evaluations: 3\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "{{external.email}}", got["vars"].(map[string]any)["value"])
	assert.EqualValues(t, len("{{external.email}}"), got["vars"].(map[string]any)["computed"])
}

func TestStackLiteralIdentity(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: !literal dev
components:
  terraform:
    app:
      metadata:
        component: !literal base
      vars:
        value: !literal "{{external.email}}"
`)
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "dev", got["vars"].(map[string]any)["stage"])
	assert.Equal(t, "base", got["metadata"].(map[string]any)["component"])
}

func TestStackLiteralDescribeStacks(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        value: !literal "{{external.email}}"
`)
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	for _, process := range []bool{false, true} {
		got, err := ExecuteDescribeStacks(&config, "dev", nil, nil, nil, false, process, process, false, nil, nil)
		require.NoError(t, err)
		stack := got["dev"].(map[string]any)
		components := stack["components"].(map[string]any)["terraform"].(map[string]any)
		assert.Equal(t, "{{external.email}}", components["app"].(map[string]any)["vars"].(map[string]any)["value"])
	}
}

func TestStackLiteralCustomDelimiters(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        value: !literal "<< external.email >>"
        normal: "<< .vars.stage >>"
`)
	file, err := os.OpenFile("atmos.yaml", os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteString("    delimiters: ['<<', '>>']\n    evaluations: 2\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "<< external.email >>", got["vars"].(map[string]any)["value"])
	assert.Equal(t, "dev", got["vars"].(map[string]any)["normal"])
}

func TestStackLiteralGenerators(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      backend_type: local
      backend:
        local:
          path: !literal "{{external.path}}"
      vars:
        value: !literal "{{external.email}}"
`)
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	varfile := filepath.Join(t.TempDir(), "vars.json")
	require.NoError(t, ExecuteTerraformGenerateVarfiles(&config, varfile, "json", nil, []string{"app"}))
	data, err := os.ReadFile(varfile)
	require.NoError(t, err)
	var vars map[string]any
	require.NoError(t, json.Unmarshal(data, &vars))
	assert.Equal(t, "{{external.email}}", vars["value"])
	backend := filepath.Join(t.TempDir(), "backend.json")
	require.NoError(t, ExecuteTerraformGenerateBackends(&config, backend, "json", nil, []string{"app"}))
	data, err = os.ReadFile(backend)
	require.NoError(t, err)
	assert.Contains(t, string(data), "{{external.path}}")
	assert.NotContains(t, string(data), "ATMOS_LITERAL_V1")
}

func TestStackLiteralSources(t *testing.T) {
	for _, templates := range []bool{false, true} {
		for _, functions := range []bool{false, true} {
			t.Run(fmtBoolPair(templates, functions), func(t *testing.T) {
				writeManifestLoadFixture(t, "dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        scalar: !literal "{{external.email}}"
        list: [!literal "!env EXTERNAL", plain]
        nested:
          value: !literal "{{external.value}}"
      env:
        EXTERNAL: !literal "{{external.env}}"
      settings:
        example:
          value: !literal "{{external.setting}}"
      backend_type: local
      backend:
        local:
          path: !literal "{{external.path}}"
`)
				config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
				require.NoError(t, err)
				stacks, raw, _, err := FindStacksMap(&config, false)
				require.NoError(t, err)
				before, err := json.Marshal([]any{stacks, raw})
				require.NoError(t, err)
				for range 2 {
					got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
						AtmosConfig: &config, Component: "app", Stack: "dev",
						ProcessTemplates: templates, ProcessYamlFunctions: functions,
					})
					require.NoError(t, err)
					sources, ok := got[sourcesSectionName].(schema.ConfigSources)
					require.True(t, ok, "expected typed provenance, got %T", got[sourcesSectionName])
					assert.Equal(t, "{{external.email}}", sources["vars"]["scalar"].FinalValue)
					assert.Equal(t, []any{"!env EXTERNAL", "plain"}, sources["vars"]["list"].FinalValue)
					assert.Equal(t, map[string]any{"value": "{{external.value}}"}, sources["vars"]["nested"].FinalValue)
					assert.Equal(t, "{{external.env}}", sources["env"]["EXTERNAL"].FinalValue)
					assert.Equal(t, map[string]any{"value": "{{external.setting}}"}, sources["settings"]["example"].FinalValue)
					assert.Equal(t, "{{external.path}}", sources["backend"]["path"].FinalValue)
					output, err := json.Marshal(got)
					require.NoError(t, err)
					assert.NotContains(t, string(output), "ATMOS_LITERAL_V1:")
					// Mutating returned provenance must not change the source cache or the next description.
					sources["vars"]["nested"].FinalValue.(map[string]any)["value"] = "output mutation"
					cached, rawCached, _, err := FindStacksMap(&config, false)
					require.NoError(t, err)
					after, err := json.Marshal([]any{cached, rawCached})
					require.NoError(t, err)
					assert.JSONEq(t, string(before), string(after))
				}
			})
		}
	}
}

func TestRestoreConfigSourceLiteralsIsolation(t *testing.T) {
	encoded := literalsource.Encode("{{external.value}}")
	final := map[string]any{"list": []any{encoded}}
	dependency := map[string]any{"value": encoded}
	sources := schema.ConfigSources{"vars": {"value": {
		Name: "value", FinalValue: final,
		StackDependencies: schema.ConfigSourcesStackDependencies{{
			DependencyType: "inline", StackFile: "dev", StackFileSection: "vars", VariableValue: dependency,
		}},
	}}}
	restored := restoreConfigSourceLiterals(sources).(schema.ConfigSources)
	item := restored["vars"]["value"]
	assert.Equal(t, "value", item.Name)
	assert.Equal(t, []any{"{{external.value}}"}, item.FinalValue.(map[string]any)["list"])
	assert.Equal(t, schema.ConfigSourcesStackDependencies{{
		DependencyType: "inline", StackFile: "dev", StackFileSection: "vars",
		VariableValue: map[string]any{"value": "{{external.value}}"},
	}}, item.StackDependencies)
	assert.Equal(t, encoded, final["list"].([]any)[0])
	assert.Equal(t, encoded, dependency["value"])
	item.FinalValue.(map[string]any)["list"].([]any)[0] = "output final"
	item.StackDependencies[0].VariableValue.(map[string]any)["value"] = "output dependency"
	assert.Equal(t, encoded, final["list"].([]any)[0])
	assert.Equal(t, encoded, dependency["value"])
	final["list"].([]any)[0] = "source final"
	dependency["value"] = "source dependency"
	sources["vars"]["value"].StackDependencies[0].StackFile = "changed source"
	assert.Equal(t, "output final", item.FinalValue.(map[string]any)["list"].([]any)[0])
	assert.Equal(t, "output dependency", item.StackDependencies[0].VariableValue.(map[string]any)["value"])
	assert.Equal(t, "dev", item.StackDependencies[0].StackFile)
	delete(restored["vars"], "value")
	require.Contains(t, sources["vars"], "value")
	assert.Nil(t, restoreConfigSourceLiterals(schema.ConfigSources(nil)))
	assert.Equal(t, schema.ConfigSources{"vars": nil}, restoreConfigSourceLiterals(schema.ConfigSources{"vars": nil}))
	assert.Equal(t, map[string]any{"value": "{{external.value}}"}, restoreConfigSourceLiterals(map[string]any{"value": encoded}))
}

func TestStackLiteralGeneratedByTemplate(t *testing.T) {
	writeManifestLoadFixture(t, "dev.yaml.tmpl", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        value: {{ printf "!literal %q" "{{external.email}}" }}
        normal: '{{ printf "dev" }}'
`)
	got, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component: "app", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	vars := got["vars"].(map[string]any)
	assert.Equal(t, "{{external.email}}", vars["value"])
	assert.Equal(t, "dev", vars["normal"])
}
