package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
	runtime "github.com/cloudposse/atmos/pkg/script/starlark"
)

func TestConfigLoadFormats(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"app.yaml": "database:\n  host: localhost\n  port: 5432\ntags: [api, web]\nenvironment: dev\napi-key: token\n",
		"app.yml":  "database: {host: localhost, port: 5432}\ntags: [api, web]\nenvironment: dev\napi-key: token",
		"app.json": `{"database":{"host":"localhost","port":5432},"tags":["api","web"],"environment":"dev","api-key":"token"}`,
	}
	dir := writeModules(t, files)
	for name := range files {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: fmt.Sprintf(`
app_schema = {
 "database": {"host": str, "port": schema.field(int, default=5432, min=1, max=65535)},
 "debug": schema.field(bool, default=False),
 "tags": schema.list(str),
 "environment": schema.enum("dev", "staging", "prod"),
 "description": schema.optional(str),
 "api-key": str,
}
cfg = config.load(%q, schema=app_schema)
output = [cfg.database.host, cfg.database.port, cfg["api-key"], cfg.debug, cfg.description, cfg.tags, cfg.environment, type(cfg)]
`, name)})
			require.NoError(t, err)
			assert.JSONEq(t, `["localhost",5432,"token",false,null,["api","web"],"dev","config"]`, result.Value)
		})
	}
}

func TestConfigDefaultsAndCopies(t *testing.T) {
	t.Parallel()
	result, err := runtime.New().Execute(t.Context(), script.Spec{Source: `
defaults = {"names": ["api"]}
rule = schema.field({"names": schema.list(str)}, default=defaults)
defaults["names"].append("worker")
source = {"nested": {"port": 5432}}
cfg = config.validate(source, schema={"nested": {"port": int}, "other": rule})
source["nested"]["port"] = 9999
output = [cfg, dir(cfg), len(cfg), "nested" in cfg, dict(cfg)]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"nested":{"port":5432},"other":{"names":["api"]}},["merge","nested","other"],2,true,{"nested":{"port":5432},"other":{"names":["api"]}}]`, result.Value)
}

func TestConfigValidationFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, want string }{
		{"validate missing schema", `config.validate({})`, "missing argument for schema"},
		{"list missing type", `schema.list()`, "missing argument for type"},
		{"list invalid type", `schema.list("str")`, "expected str, int"},
		{"optional missing type", `schema.optional()`, "missing argument for type"},
		{"optional invalid type", `schema.optional("str")`, "expected str, int"},
		{"field invalid type", `schema.field("str")`, "expected str, int"},
		{"nonfinite enum", `schema.enum(float("inf"))`, "finite numbers"},
		{"nonfinite bound", `schema.field(float, max=float("inf"))`, "finite numeric bounds"},
		{"nonfinite value", `config.validate(float("nan"), schema=float)`, "expected float"},
		{"inherited default", `schema.field(schema.field(int, default=1), min=2)`, "default: expected at least 2"},
		{"schema as key", `{schema.optional(str): "value"}`, "unhashable: schema"},
		{"insecure schema", `config.validate({}, schema="http://example.com/schema.json")`, "must use HTTPS"},
		{"credentialed schema", `config.validate({}, schema="https://user:password@example.com/schema.json")`, "without embedded credentials"},
		{"malformed schema URL", `config.validate({}, schema="https://example.com/%zz")`, "invalid URL escape"},
		{"required", `config.validate({}, schema={"port": int})`, "port: required field is missing"},
		{"unknown", `config.validate({"porrt": 1}, schema={"port": int})`, "porrt: unknown field"},
		{"strict int", `config.validate({"port": "5432"}, schema={"port": int})`, "port: expected int, got string"},
		{"bool is not int", `config.validate(True, schema=int)`, "expected int, got bool"},
		{"float is not int", `config.validate(1.0, schema=int)`, "expected int, got float"},
		{"int is not float", `config.validate(1, schema=float)`, "expected float, got int"},
		{"null required", `config.validate(None, schema=str)`, "expected string, got NoneType"},
		{"record", `config.validate([], schema={})`, "expected object, got list"},
		{"list", `config.validate("x", schema=schema.list(str))`, "expected list, got string"},
		{"list item", `config.validate([1, "bad"], schema=schema.list(int))`, "1: expected int, got string"},
		{"enum", `config.validate("bad", schema=schema.enum("dev", "prod"))`, "expected one of"},
		{"min", `config.validate(0, schema=schema.field(int, min=1))`, "expected at least 1"},
		{"max", `config.validate(65536, schema=schema.field(int, max=65535))`, "expected at most 65535"},
		{"default type", `schema.field(int, default="bad")`, "default: expected int, got string"},
		{"default bound", `schema.field(int, default=0, min=1)`, "default: expected at least 1"},
		{"invalid bounds", `schema.field(int, min=2, max=1)`, "min must not exceed max"},
		{"nonnumeric bound", `schema.field(int, min="1")`, "finite numeric bounds"},
		{"nonnumeric field", `schema.field(str, min=1)`, "numeric type"},
		{"bad schema", `config.validate({}, schema={"port": "int"})`, "expected str, int"},
		{"schema key", `config.validate({}, schema={1: int})`, "field names must be strings"},
		{"data key", `config.validate({1: "x"}, schema={})`, "object keys must be strings"},
		{"empty enum", `schema.enum()`, "one or more positional"},
		{"enum list", `schema.enum([])`, "choices must be"},
		{"enum kwargs", `schema.enum(value="dev")`, "positional scalar choices"},
		{"field positional", `schema.field(int, 1)`, "keyword arguments"},
		{"empty path", `config.load("", schema={})`, "path must not be empty"},
		{"empty schema path", `config.validate({}, schema="")`, "schema path must not be empty"},
		{"missing schema", `config.load("app.yaml")`, "missing argument for schema"},
		{"bad keyword", `schema.field(int, typo=1)`, "unexpected keyword"},
		{"frozen field", `cfg = config.validate({"port": 1}, schema={"port": int})
cfg.port = 2`, "assign"},
		{"frozen key", `cfg = config.validate({"port": 1}, schema={"port": int})
cfg["port"] = 2`, "frozen"},
		{"frozen list", `cfg = config.validate({"tags": []}, schema={"tags": schema.list(str)})
cfg.tags.append("x")`, "frozen"},
		{"missing attribute", `cfg = config.validate({}, schema={})
print(cfg.missing)`, "missing"},
		{"nested descriptor limit", `def deeply_nested():
    rule = int
    value = 1
    for _ in range(102):
        rule = schema.list(rule)
        value = [value]
    return config.validate(value, schema=rule)
deeply_nested()`, "configuration exceeds nesting limit"},
		{"schema cycle", `rule = {}
rule["self"] = rule
config.validate({}, schema=rule)`, "cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runtime.New().Execute(t.Context(), script.Spec{Source: tc.source})
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestConfigSourceLocations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, data, schema, want string }{
		{"yaml type", "database:\n  port: oops\n", `{"database": {"port": int}}`, ":2:9: database.port: expected int"},
		{"json type", "{\n  \"database\": {\n    \"port\": \"oops\"\n  }\n}", `{"database": {"port": int}}`, ":3:13: database.port: expected int"},
		{"missing", "database:\n  host: localhost\n", `{"database": {"host": str, "port": int}}`, ":2:3: database.port: required"},
		{"unknown", "database:\n  porrt: 1\n", `{"database": {"port": int}}`, ":2:10: database.porrt: unknown"},
		{"list", "ports:\n  - 1\n  - bad\n", `{"ports": schema.list(int)}`, ":3:5: ports.1: expected int"},
		{"duplicate", "port: 1\nport: 2\n", `{"port": int}`, ":2:1: port: duplicate key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name := "app.yaml"
			if strings.HasPrefix(tc.name, "json") {
				name = "app.json"
			}
			dir := writeModules(t, map[string]string{name: tc.data})
			_, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: fmt.Sprintf(`config.load(%q, schema=%s)`, name, tc.schema)})
			require.ErrorContains(t, err, name+tc.want)
		})
	}
}

func TestConfigPathsAndParallel(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"lib/app.yaml": "port: 42",
		"lib/reader.star": `rule = {"port": int}
def read():
    return config.load("app.yaml", schema=rule).port`,
		"elsewhere/unused": "",
	})
	for _, tc := range []struct{ name, source, working, sourcePath, want string }{
		{"entry", `output = config.load("lib/app.yaml", schema={"port": int}).port`, filepath.Join(dir, "elsewhere"), filepath.Join(dir, "main.star"), "42"},
		{"inline", `output = config.load("lib/app.yaml", schema={"port": int}).port`, dir, "", "42"},
		{"absolute", fmt.Sprintf(`output = config.load(%q, schema={"port": int}).port`, filepath.Join(dir, "lib", "app.yaml")), dir, "", "42"},
		{"parallel loaded function", `reader = module.load("lib/reader.star")
output = steps.parallel(functions=[reader.read, reader.read])`, dir, "", "[42,42]"},
		{"parallel shared config", `cfg = config.load("lib/app.yaml", schema={"port": int})
def read():
    return cfg.port
output = steps.parallel(functions=[read, read])`, dir, "", "[42,42]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := runtime.New().Execute(t.Context(), script.Spec{WorkingDirectory: tc.working, SourcePath: tc.sourcePath, Source: tc.source})
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, result.Value)
		})
	}
}

func TestConfigDryRunAndCancellation(t *testing.T) {
	t.Parallel()
	engine := runtime.New(runtime.WithReadFile(func(string) ([]byte, error) { t.Error("unexpected file read"); return nil, os.ErrNotExist }))
	_, err := engine.Execute(t.Context(), script.Spec{DryRun: true, Source: `config.load("missing.yaml", schema="missing.schema.json")`})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	engine = runtime.New(runtime.WithReadFile(func(string) ([]byte, error) { cancel(); return []byte("port: 1"), nil }))
	_, err = engine.Execute(ctx, script.Spec{Source: `config.load("app.yaml", schema={"port": int})`})
	require.ErrorIs(t, err, context.Canceled)
}

func TestConfigRecordOperations(t *testing.T) {
	t.Parallel()
	result, err := runtime.New().Execute(t.Context(), script.Spec{Source: `
rule = {"items": str, "get": int, "optional": schema.optional(str), "enabled": bool,
        "ratio": float, "list": schema.list({"name": str})}
raw = {"items":"api", "get":3, "optional":None, "enabled":True, "ratio":1.5, "list":[{"name":"api"}]}
a = config.validate(raw, schema=rule)
b = config.validate(raw, schema=rule)
output = [a == b, a != b, a.items, a.get, a.list[0].name, hasattr(a,"missing"),
          getattr(a,"missing","fallback"), json.decode(json.encode(a))]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[true,false,"api",3,"api",false,"fallback",{"items":"api","get":3,"optional":null,"enabled":true,"ratio":1.5,"list":[{"name":"api"}]}]`, result.Value)
}

func TestConfigSchemaCompositions(t *testing.T) {
	t.Parallel()
	result, err := runtime.New().Execute(t.Context(), script.Spec{Source: `
rule = {
 "nested":schema.field({"port":schema.field(int,default=5432)},default={}),
 "optional":schema.optional(schema.field(str,default="ok")),
 "nullable":schema.field(schema.optional(str),default=None),
 "choice":schema.field(schema.enum(True, "dev", 1),default="dev"),
 "ports":schema.field(schema.list(schema.field(int,min=1)),default=[80,443]),
}
cfg = config.validate({},schema=rule)
def read():
    return config.validate({},schema=rule)
output = [cfg, steps.parallel(functions=[read,read])]
`})
	require.NoError(t, err)
	expected := `{"nested":{"port":5432},"optional":"ok","nullable":null,"choice":"dev","ports":[80,443]}`
	assert.JSONEq(t, "["+expected+",["+expected+","+expected+"]]", result.Value)
}

func TestConfigHelpersAreAutomationOnly(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`return config`, `return schema`} {
		_, err := runtime.New().EvaluateValue(t.Context(), script.Evaluation{Source: source, Filename: "stack.yaml"})
		require.ErrorContains(t, err, "undefined")
	}
}

// Keep binding integration tests beside the implementation while exercising the
// real automation parser, caller-relative paths, and parallel task lifecycle.
func writeModules(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, source := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	}
	return dir
}

// Schema descriptors behave as immutable Starlark values and cannot be dictionary keys.
func TestConfigSchemaDescriptorValues(t *testing.T) {
	t.Parallel()
	result, err := runtime.New().Execute(t.Context(), script.Spec{Source: `
rule = schema.optional(str)
output = [str(rule), type(rule), bool(rule), config.validate(None, schema=rule)]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `["<schema str>","schema",true,null]`, result.Value)
}
