package step

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/script"
	star "github.com/cloudposse/atmos/pkg/script/starlark"
)

func runLibrarySource(t *testing.T, source string) (script.Result, string, error) {
	t.Helper()
	initShellTestIO(t)
	var out bytes.Buffer
	result, err := star.New().Execute(t.Context(), script.Spec{
		Name: "library.star", Source: source, WorkingDirectory: t.TempDir(),
		Steps: NewAutomationLibrary(nil, nil), Stdout: &out, Stderr: &out,
	})
	return result, out.String(), err
}

func TestScriptLibraryRegistry(t *testing.T) {
	result, _, err := runLibrarySource(t, `output = dir(steps)`)
	require.NoError(t, err)
	var names []string
	require.NoError(t, json.Unmarshal([]byte(result.Value), &names))
	for _, name := range NewAutomationLibrary(nil, nil).Names() {
		assert.Contains(t, names, strings.ReplaceAll(name, "-", "_"))
	}
	assert.Contains(t, names, "run")
	assert.Contains(t, names, "task")
}

func TestScriptLibraryResultsAndOutputs(t *testing.T) {
	result, _, err := runLibrarySource(t, `
r = steps.join(name = "joined", options = ["api", "worker"], separator = ",", outputs = {"services": "{{ .value }}"})
s = steps.run("join", content = "{{ .steps.joined.outputs.services }}")
output = [r.value, r.outputs, r.values, r.metadata, r.skipped, r.error, s.value]
`)
	require.NoError(t, err)
	assert.JSONEq(t, `["api,worker",{"services":"api,worker"},[],{},false,"","api,worker"]`, result.Value)
}

func TestScriptLibraryPromptDefaults(t *testing.T) {
	t.Setenv("ATMOS_FORCE_TTY", "false")
	result, _, err := runLibrarySource(t, `
a = steps.input(prompt = "Service?", default = "api")
b = steps.choose(prompt = "Environment?", options = ["dev", "prod"], default = "dev")
c = steps.confirm(prompt = "Deploy?", default = "no")
output = [a.value, b.value, c.value]
`)
	require.NoError(t, err)
	assert.JSONEq(t, `["api","dev","false"]`, result.Value)
	_, _, err = runLibrarySource(t, `steps.input(prompt = "Service?")`)
	require.ErrorIs(t, err, errUtils.ErrStepTTYRequired)
}

func TestScriptLibraryParallelIsolation(t *testing.T) {
	result, output, err := runLibrarySource(t, `
steps.env(vars = {"SERVICE": "parent"})
def work(name):
    steps.env(vars = {"SERVICE": name})
    steps.join(name = "own", content = name)
    steps.script(interpreter = "starlark", script = 'print("hello")')
    return steps.join(content = "{{ .env.SERVICE }}:{{ .steps.own.value }}").value
values = steps.parallel(tasks = [steps.task(name = n, function = work, args = [n]) for n in ["api", "worker"]])
output = [values, steps.join(content = "{{ .env.SERVICE }}").value]
`)
	require.NoError(t, err)
	assert.JSONEq(t, `[["api:api","worker:worker"],"parent"]`, result.Value)
	assert.Contains(t, output, "[api] hello\n")
	assert.Contains(t, output, "[worker] hello\n")
}

func TestScriptLibraryInvalidCalls(t *testing.T) {
	for _, tc := range []struct{ source, message string }{
		{`steps.input(promtp="typo")`, `unknown field "promtp" for step type "input"`},
		{`steps.http()`, "url"},
		{`steps.run("missing")`, "unknown step type"},
		{`steps.join("hello")`, "keyword arguments"},
		{`steps.run()`, "one positional step type"},
		{`steps.run(42)`, "must be a string"},
		{`steps.join(type="input")`, "type is selected"},
		{`steps.join(content="hello", when="never")`, `when is not supported in a direct step call`},
		{`steps.join(content="hello", identity="admin")`, `identity is not supported in a direct step call`},
		{`steps.join(content="hello", container={"image":"alpine"})`, "container overrides"},
		{`steps.parallel(functions=[lambda: steps.input(prompt="x", default="y")])`, "exclusive terminal"},
		{`steps.parallel(functions=[lambda: steps.exec(command="unused")])`, "exclusive terminal"},
		{`steps.parallel(functions=[lambda: steps.cast(steps=[])])`, "exclusive terminal"},
		{`steps.parallel(functions=[lambda: steps.script(interpreter="starlark", script='steps.input(prompt="nested", default="no")')])`, "exclusive terminal"},
		{`steps.wait_all()`, "workflow executor context"},
		{`steps.join(options=["a"]).outputs.update({"x":"y"})`, "frozen"},
		{`steps.join(content=lambda: None)`, "cannot encode"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			_, _, err := runLibrarySource(t, tc.source)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestScriptLibraryHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "from-script", r.Header.Get("X-Source"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ready":true}`))
	}))
	defer server.Close()
	result, _, err := runLibrarySource(t, fmt.Sprintf(`r = steps.webhook(url = %q, headers = {"X-Source": "from-script"}, outputs = {"code": "{{ .status_code }}"})
output = [json.decode(r.value), r.metadata["status_code"], r.outputs]`, server.URL))
	require.NoError(t, err)
	assert.JSONEq(t, `[{"ready":true},200,{"code":"200"}]`, result.Value)
	assert.EqualValues(t, 1, calls.Load())
}

func TestScriptLibraryHTTPDoesNotMultiplyRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	_, _, err := runLibrarySource(t, fmt.Sprintf(`steps.http(url=%q, retry={"max_attempts":2, "initial_delay":"1ms"})`, server.URL))
	require.Error(t, err)
	assert.EqualValues(t, 2, calls.Load())
}

func TestScriptLibraryPolymorphicConfiguration(t *testing.T) {
	call := &automation.StepCall{Type: "container", Configuration: stepConfig(t, `{"action":"build","with":{"dockerfile":"Dockerfile","tags":["demo:dev"]}}`)}
	step, err := decodeAutomationStep(call)
	require.NoError(t, err)
	require.NotNil(t, step.Build)
	assert.Equal(t, "Dockerfile", step.Build.Dockerfile)
	assert.Equal(t, []string{"demo:dev"}, step.Build.Tags)
	_, _, err = runLibrarySource(t, `steps.container(action="build", with_={"tags": []}, **{"with": {}})`)
	require.ErrorContains(t, err, "duplicate step field")
}

func TestScriptLibraryCancellationAndDryRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_, err := star.New().Execute(ctx, script.Spec{Source: `steps.sleep(timeout="1h")`, Steps: NewAutomationLibrary(nil, nil)})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = star.New().Execute(t.Context(), script.Spec{Source: `steps.http(url="must-not-run")`, DryRun: true, Steps: NewAutomationLibrary(nil, nil)})
	require.NoError(t, err)
}

func TestScriptLibraryDirectoryAndProcessEnvironment(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)
	dir := t.TempDir()
	nested := filepath.Join(dir, "child")
	step, err := decodeAutomationStep(&automation.StepCall{Type: "join", Configuration: stepConfig(t, `{"content":"ok","working_directory":"child"}`)})
	require.NoError(t, err)
	require.NoError(t, library.prepareCall(step, &automation.StepCall{WorkingDirectory: dir, ProcessEnv: []string{"PARENT=inherited"}}))
	assert.Equal(t, nested, step.WorkingDirectory)
	assert.Equal(t, map[string]string{"PARENT": "inherited"}, library.vars.Env)
	result, err := library.Run(t.Context(), &automation.StepCall{Type: "env", Configuration: stepConfig(t, `{"vars":{"LOCAL":"updated"}}`), ProcessEnv: []string{"PARENT=inherited"}})
	require.NoError(t, err)
	require.NotNil(t, result)
	_, err = library.Run(t.Context(), &automation.StepCall{Type: "join", Configuration: stepConfig(t, `{"content":"ok"}`), ProcessEnv: []string{"PARENT=new", "PATH=installed-tool"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"PARENT": "new", "LOCAL": "updated", "PATH": "installed-tool"}, library.vars.Env)
}

func stepConfig(t *testing.T, value string) map[string]any {
	t.Helper()
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(value), &fields))
	return fields
}

func TestScriptLibraryNestedCallTimeout(t *testing.T) {
	_, _, err := runLibrarySource(t, `steps.script(interpreter="starlark", script="while True: pass", timeout="10ms")`)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, _, err = runLibrarySource(t, `steps.join(content="hello",timeout="invalid")`)
	require.ErrorIs(t, err, errUtils.ErrStepTimeoutInvalid)
}
