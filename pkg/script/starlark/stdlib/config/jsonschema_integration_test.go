package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
	runtime "github.com/cloudposse/atmos/pkg/script/starlark"
)

func TestConfigFileSchemas(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"app.json":                    `{"database":{"host":"localhost","port":5432}}`,
		"app.yaml":                    "database:\n  host: localhost\n  port: 5432\n",
		"schema/app.schema.json":      `{"type":"object","properties":{"database":{"$ref":"./database.schema.yaml"}},"required":["database"],"additionalProperties":false}`,
		"schema/app.schema.yaml":      "type: object\nproperties:\n  database:\n    $ref: ./database.schema.yaml\nrequired: [database]\nadditionalProperties: false\n",
		"schema/database.schema.yaml": "type: object\nproperties:\n  host: {type: string}\n  port: {type: integer, minimum: 1, maximum: 65535}\nrequired: [host, port]\nadditionalProperties: false",
	})
	for _, data := range []string{"app.json", "app.yaml"} {
		for _, schema := range []string{"schema/app.schema.json", "schema/app.schema.yaml"} {
			t.Run(data+schema, func(t *testing.T) {
				t.Parallel()
				result, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: fmt.Sprintf(`
cfg = config.load(%q, schema=%q)
output = [cfg.database.port, config.validate({"database":{"host":"localhost","port":5432}}, schema=%q)]
`, data, schema, schema)})
				require.NoError(t, err)
				assert.JSONEq(t, `[5432,{"database":{"host":"localhost","port":5432}}]`, result.Value)
			})
		}
	}
}

func TestConfigFileSchemaErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, schema, data, want string }{
		{"type location", `{"type":"object","properties":{"port":{"type":"integer"}}}`, "port: bad", "app.yaml:1:7: port: expected integer"},
		{"missing", `{"type":"object","required":["port"]}`, "{}", "app.yaml:1:1: $: missing properties"},
		{"unknown", `{"type":"object","additionalProperties":false}`, "port: 1", "additionalProperties"},
		{"invalid schema", `{"type":"nonsense"}`, "{}", "schema"},
		{"missing ref", `{"$ref":"missing.json"}`, "{}", "missing.json"},
		{"remote ref", `{"$ref":"http://example.com/schema.json"}`, "{}", "schema references must use local files or HTTPS"},
		{"nested ref", `{"$defs":{"port":{"type":"integer"}},"type":"object","properties":{"port":{"$ref":"#/$defs/port"}}}`, "port: bad", "app.yaml:1:7: port: expected integer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := writeModules(t, map[string]string{"app.yaml": tc.data, "app.schema.json": tc.schema})
			_, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `config.load("app.yaml", schema="app.schema.json")`})
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestConfigFileSchemaDoesNotApplyDefaults(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"schema.json": `{"type":"object","properties":{"port":{"type":"integer","default":5432}}}`,
	})
	result, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `output = config.validate({}, schema="schema.json")`})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, result.Value)
}

func TestConfigParsing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, file, data, source, want, failure string }{
		{name: "explicit format", file: "app", data: "port: 42", source: `config.load("app", schema={"port":int}, format="yaml")`, want: `{"port":42}`},
		{name: "strict json", file: "app.json", data: "port: 42", failure: "invalid JSON"},
		{name: "multiple documents", file: "app.yaml", data: "{}\n---\n{}", failure: "one yaml document"},
		{name: "malformed yaml", file: "app.yaml", data: "port: [", failure: "invalid yaml"},
		{name: "empty", file: "app.yaml", data: "", failure: "invalid yaml"},
		{name: "nonstring key", file: "app.yaml", data: "1: value", failure: "keys must be strings"},
		{name: "tag", file: "app.yaml", data: "port: !env SECRET", failure: "unsupported scalar"},
		{name: "infinity", file: "app.yaml", data: "port: .inf", failure: "unsupported scalar"},
		{name: "alias cycle", file: "app.yaml", data: "port: &a [*a]", failure: "nesting or size limit"},
		{name: "duplicate json", file: "app.json", data: `{"port":1,"port":2}`, failure: "duplicate key"},
		{name: "extension", file: "app.txt", data: "{}", failure: "specify format"},
		{name: "huge integer", file: "app.json", data: `{"port":123456789012345678901234567890}`, want: `{"port":123456789012345678901234567890}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := writeModules(t, map[string]string{tc.file: tc.data})
			source := tc.source
			if source == "" {
				source = fmt.Sprintf(`config.load(%q, schema={"port":int})`, tc.file)
			}
			result, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: "output = " + source})
			if tc.failure != "" {
				require.ErrorContains(t, err, tc.failure)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, result.Value)
		})
	}
}

func TestConfigFileSchemaRecordsAndLocations(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"schema.json": `{"type":"object","properties":{"a/b~c":{"type":"integer"},"nested":{"type":"array"}}}`,
		"app.json":    "{\n  \"a/b~c\": \"bad\"\n}",
		"plain.yaml":  "nested: [{value: true}]\nnumber: 1.5\nmissing: null\ndate: 2026-10-23\n",
	})
	_, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `config.load("app.json",schema="schema.json")`})
	require.ErrorContains(t, err, "app.json:2:12: a/b~c: expected integer")
	result, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `
cfg = config.load("plain.yaml", schema="schema.json")
output = [cfg.nested[0].value,cfg.number,cfg.missing,cfg.date]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[true,1.5,null,"2026-10-23"]`, result.Value)
}

func TestConfigYAMLAliases(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{"app.yaml": "first: &a {port: 5432}\nsecond: *a\n"})
	result, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `
rule = {"port":int}
cfg = config.load("app.yaml",schema={"first":rule,"second":rule})
output = cfg.first == cfg.second
`})
	require.NoError(t, err)
	assert.Equal(t, "true", result.Value)
}
