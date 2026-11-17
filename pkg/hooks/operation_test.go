package hooks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
)

// requestRecorder records the request paths an httptest server receives.
type requestRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (r *requestRecorder) handler(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, req.URL.Path)
	w.WriteHeader(http.StatusNoContent)
}

func (r *requestRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func newRecorderServer(t *testing.T) (*requestRecorder, string) {
	t.Helper()
	recorder := &requestRecorder{}
	server := httptest.NewServer(http.HandlerFunc(recorder.handler))
	t.Cleanup(server.Close)
	return recorder, server.URL
}

func httpStepHook(url string, extra map[string]any) map[string]any {
	hook := map[string]any{
		"kind": "step", "type": "http",
		"with": map[string]any{"url": url, "method": "POST"},
	}
	for k, v := range extra {
		hook[k] = v
	}
	return hook
}

func docsTestOperation() OperationContext {
	return OperationContext{
		Type: "docs", Name: "readme",
		Env:  map[string]string{DocsTypeEnvVar: "readme", DocsOutputPathEnvVar: "/out/README.md"},
		Data: map[string]any{"type": "readme", "output_path": "/out/README.md"},
	}
}

func TestNewOperationHooksDecodesAndValidates(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		wantErr error
		items   int
	}{
		{name: "nil hooks", raw: nil, items: 0},
		{name: "valid hook", raw: map[string]any{"publish": httpStepHook("http://x", map[string]any{"events": []any{"after.docs.generate"}})}, items: 1},
		{name: "before event", raw: map[string]any{"pre": httpStepHook("http://x", map[string]any{"events": []any{"before.docs.generate"}})}, items: 1},
		{name: "unregistered event", raw: map[string]any{"bad": httpStepHook("http://x", map[string]any{"events": []any{"after.docs.publish"}})}, wantErr: errUtils.ErrUnsupportedHookEvent},
		{name: "malformed hook", raw: map[string]any{"bad": "not-a-map"}, wantErr: errUtils.ErrInvalidHooksSection},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewOperationHooks(&schema.AtmosConfiguration{}, tt.raw, docsTestOperation())
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, h)
				return
			}
			require.NoError(t, err)
			assert.Len(t, h.items, tt.items)
			assert.Equal(t, "docs", h.info.ComponentType)
			assert.Equal(t, "readme", h.info.ComponentFromArg)
			assert.Empty(t, h.info.Stack)
		})
	}
}

func TestOperationHooksRunsOnlyMatchingEvent(t *testing.T) {
	recorder, url := newRecorderServer(t)
	raw := map[string]any{
		"before-hook": httpStepHook(url+"/before", map[string]any{"events": []any{"before.docs.generate"}}),
		"after-hook":  httpStepHook(url+"/after", map[string]any{"events": []any{"after.docs.generate"}}),
		// Hooks for another operation must never fire for docs events (negative path).
		"other-hook": httpStepHook(url+"/other", map[string]any{"events": []any{"after.terraform.apply"}}),
	}
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, docsTestOperation())
	require.NoError(t, err)

	require.NoError(t, h.RunOperation(BeforeDocsGenerate, nil))
	assert.Equal(t, []string{"/before"}, recorder.seen())

	require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
	assert.Equal(t, []string{"/before", "/after"}, recorder.seen())
}

func TestOperationHooksFilterByOutcome(t *testing.T) {
	tests := []struct {
		name    string
		when    any
		outcome Outcome
		fires   bool
	}{
		{name: "default runs on success", when: nil, outcome: Outcome{Status: RunSuccess}, fires: true},
		{name: "default skipped on failure", when: nil, outcome: Outcome{Status: RunFailure, ExitCode: 1}, fires: false},
		{name: "failure runs on failure", when: "failure", outcome: Outcome{Status: RunFailure, ExitCode: 1}, fires: true},
		{name: "failure skipped on success", when: "failure", outcome: Outcome{Status: RunSuccess}, fires: false},
		{name: "always runs on failure", when: "always", outcome: Outcome{Status: RunFailure, ExitCode: 1}, fires: true},
		{name: "always runs on success", when: "always", outcome: Outcome{Status: RunSuccess}, fires: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder, url := newRecorderServer(t)
			extra := map[string]any{"events": []any{"after.docs.generate"}}
			if tt.when != nil {
				extra["when"] = tt.when
			}
			h, err := NewOperationHooks(&schema.AtmosConfiguration{}, map[string]any{"notify": httpStepHook(url+"/notify", extra)}, docsTestOperation())
			require.NoError(t, err)
			h.SetOutcome(tt.outcome)
			require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
			if tt.fires {
				assert.Equal(t, []string{"/notify"}, recorder.seen())
			} else {
				assert.Empty(t, recorder.seen())
			}
		})
	}
}

func TestOperationHooksSkipHooksFlag(t *testing.T) {
	recorder, url := newRecorderServer(t)
	raw := map[string]any{
		"skipped": httpStepHook(url+"/skipped", map[string]any{"events": []any{"after.docs.generate"}}),
		"kept":    httpStepHook(url+"/kept", map[string]any{"events": []any{"after.docs.generate"}}),
	}
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, docsTestOperation())
	require.NoError(t, err)

	cmd := &cobra.Command{}
	cmd.Flags().String("skip-hooks", "", "")
	require.NoError(t, cmd.Flags().Set("skip-hooks", "skipped"))
	require.NoError(t, h.RunOperation(AfterDocsGenerate, cmd))
	assert.Equal(t, []string{"/kept"}, recorder.seen())

	require.NoError(t, cmd.Flags().Set("skip-hooks", "*"))
	require.NoError(t, h.RunOperation(AfterDocsGenerate, cmd))
	assert.Equal(t, []string{"/kept"}, recorder.seen(), "skip-hooks=* skips every user hook")
}

func TestOperationHooksFailureHonorsOnFailure(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	failing := func(onFailure string) map[string]any {
		return map[string]any{
			"kind": "command", "command": exe, "events": []any{"before.docs.generate"},
			"env": map[string]any{"_ATMOS_TEST_EXIT_ONE": "1"}, "on_failure": onFailure,
		}
	}
	tests := []struct {
		name      string
		onFailure string
		wantErr   bool
	}{
		{name: "fail propagates", onFailure: OnFailureFail, wantErr: true},
		{name: "warn is swallowed", onFailure: OnFailureWarn, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewOperationHooks(&schema.AtmosConfiguration{}, map[string]any{"gate": failing(tt.onFailure)}, docsTestOperation())
			require.NoError(t, err)
			err = h.RunOperation(BeforeDocsGenerate, nil)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestOperationHooksTemplateData(t *testing.T) {
	recorder, url := newRecorderServer(t)
	raw := map[string]any{"publish": httpStepHook(url, map[string]any{"events": []any{"after.docs.generate"}})}
	raw["publish"].(map[string]any)["with"] = map[string]any{"url": url + "/{{ .docs.type }}", "method": "POST"}
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, docsTestOperation())
	require.NoError(t, err)
	require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
	assert.Equal(t, []string{"/readme"}, recorder.seen())

	// SetOperationContext replaces the data exposed to templates.
	h.SetOperationContext(OperationContext{Type: "docs", Data: map[string]any{"type": "changelog"}})
	require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
	assert.Equal(t, []string{"/readme", "/changelog"}, recorder.seen())
}

func TestOperationHooksExportEnvironment(t *testing.T) {
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, nil, docsTestOperation())
	require.NoError(t, err)
	assert.Equal(t, "/out/README.md", h.operationEnv[DocsOutputPathEnvVar])

	ctx := &ExecContext{OperationEnv: h.operationEnv, Info: h.info}
	env := buildAtmosEnv(ctx, "", "")
	assert.Equal(t, "readme", env[DocsTypeEnvVar])
	assert.Equal(t, "/out/README.md", env[DocsOutputPathEnvVar])
	assert.Equal(t, "readme", env["ATMOS_COMPONENT"])
	assert.Empty(t, env["ATMOS_STACK"])

	// Component lifecycles carry no operation variables.
	plain := buildAtmosEnv(&ExecContext{Info: &schema.ConfigAndStacksInfo{}}, "", "")
	assert.NotContains(t, plain, DocsTypeEnvVar)
}

// docsPolicyHooks attaches a governing policy requiring a callback hook for event.
func docsPolicyHooks(t *testing.T, event HookEvent, url string, deny bool) *Hooks {
	t.Helper()
	definition, err := json.Marshal(map[string]any{"kind": "step", "type": "http", "with": map[string]any{"url": url + "/policy", "method": "POST"}})
	require.NoError(t, err)
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, nil, docsTestOperation())
	require.NoError(t, err)
	rules := []policy.Rule{{
		ID: "docs-callback", On: policy.Operation, Engine: policy.CELEngine,
		Expression: "input.event == '" + string(event) + "' && input.component_type == 'docs' && input.component == 'readme' && input.stack == ''",
		Effects: policy.Effects{Hooks: []policy.HookRequirement{
			{Ref: "test/callback", Event: string(event), Required: true, RequireSuccess: true},
		}},
	}}
	if deny {
		rules = append(rules, policy.Rule{ID: "deny", On: policy.Operation, Engine: policy.CELEngine, Expression: "true", Effects: policy.Effects{Deny: true}})
	}
	h.policyBundle = &policy.Bundle{Hooks: map[string]json.RawMessage{"test/callback": definition}, Rules: rules}
	return h
}

func TestOperationPolicyHooksFireOnDocsEvents(t *testing.T) {
	recorder, url := newRecorderServer(t)
	h := docsPolicyHooks(t, BeforeDocsGenerate, url, false)
	assert.True(t, h.HasHooks(), "a governing policy counts as hooks even with no user hooks")

	// Policy input carries the docs operation with an empty stack.
	require.NoError(t, h.RunOperation(BeforeDocsGenerate, nil))
	assert.Equal(t, []string{"/policy"}, recorder.seen())

	// The rule targets the before event only (negative path).
	require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
	assert.Equal(t, []string{"/policy"}, recorder.seen())
}

func TestOperationPolicyDenialBlocks(t *testing.T) {
	recorder, url := newRecorderServer(t)
	h := docsPolicyHooks(t, BeforeDocsGenerate, url, true)
	require.ErrorIs(t, h.RunOperation(BeforeDocsGenerate, nil), errUtils.ErrPolicyDenied)
	assert.Empty(t, recorder.seen(), "a denial must prevent policy hooks from running")
}

func TestOperationPolicyRequestToleratesEmptyStack(t *testing.T) {
	run := &hookRunContext{
		event: BeforeDocsGenerate,
		info:  &schema.ConfigAndStacksInfo{ComponentType: "docs", ComponentFromArg: "readme"},
	}
	request, err := operationPolicyRequest(run, nil)
	require.NoError(t, err)
	var input policy.OperationInput
	require.NoError(t, json.Unmarshal(request.Input, &input))
	assert.Equal(t, policy.OperationInput{Event: "before.docs.generate", ComponentType: "docs", Command: "generate", Component: "readme", Stack: "", Status: ""}, input)
}

// docsEnvOperation returns an operation whose environment the dump hook must observe.
func docsEnvOperation() OperationContext {
	return OperationContext{
		Type: "docs", Name: "readme",
		Env: map[string]string{
			DocsTypeEnvVar: "readme", DocsOutputPathEnvVar: "/out/README.md", DocsBaseDirEnvVar: "/base",
		},
		Data: map[string]any{"type": "readme"},
	}
}

func readDump(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Split(string(data), "\n")
}

func TestOperationEnvReachesCommandHookSubprocess(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	want := []string{"readme", "/out/README.md", "/base"}

	t.Run("user hook", func(t *testing.T) {
		dump := filepath.Join(t.TempDir(), "env.txt")
		raw := map[string]any{"dump": map[string]any{
			"kind": "command", "command": exe, "events": []any{"after.docs.generate"},
			"env": map[string]any{"_ATMOS_TEST_DUMP_DOCS_ENV": dump}, "on_failure": OnFailureFail,
		}}
		h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, docsEnvOperation())
		require.NoError(t, err)
		require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
		assert.Equal(t, want, readDump(t, dump))
	})

	t.Run("policy hook", func(t *testing.T) {
		dump := filepath.Join(t.TempDir(), "env.txt")
		definition, err := json.Marshal(map[string]any{
			"kind": "command", "command": exe, "env": map[string]any{"_ATMOS_TEST_DUMP_DOCS_ENV": dump},
		})
		require.NoError(t, err)
		h, err := NewOperationHooks(&schema.AtmosConfiguration{}, nil, docsEnvOperation())
		require.NoError(t, err)
		h.policyBundle = &policy.Bundle{
			Hooks: map[string]json.RawMessage{"test/callback": definition},
			Rules: []policy.Rule{{
				ID: "cb", On: policy.Operation, Engine: policy.CELEngine, Expression: "true",
				Effects: policy.Effects{Hooks: []policy.HookRequirement{{Ref: "test/callback", Event: string(AfterDocsGenerate), Required: true, RequireSuccess: true}}},
			}},
		}
		require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
		assert.Equal(t, want, readDump(t, dump))
	})
}

func TestOperationHooksRunInSortedNameOrder(t *testing.T) {
	recorder, url := newRecorderServer(t)
	raw := map[string]any{}
	var names []string
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("hook-%02d", 24-i) // Declared in reverse so source order cannot pass by accident.
		names = append(names, name)
		raw[name] = httpStepHook(url+"/"+name, map[string]any{"events": []any{"after.docs.generate"}})
	}
	sort.Strings(names)
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, docsTestOperation())
	require.NoError(t, err)

	for run := 0; run < 5; run++ {
		before := len(recorder.seen())
		require.NoError(t, h.RunOperation(AfterDocsGenerate, nil))
		got := recorder.seen()[before:]
		require.Len(t, got, len(names))
		assert.Equal(t, "/"+names[0], got[0])
		assert.Equal(t, "/"+names[len(names)-1], got[len(got)-1])
		for i, name := range names {
			require.Equal(t, "/"+name, got[i], "run %d position %d", run, i)
		}
	}
}

func TestOperationHookFailureNamesHookAndKeepsExitCode(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	raw := map[string]any{"publish-wiki": map[string]any{
		"kind": "command", "command": exe, "events": []any{"after.docs.generate"},
		"env": map[string]any{"_ATMOS_TEST_EXIT_CODE": "3"}, "on_failure": OnFailureFail,
	}}
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, docsTestOperation())
	require.NoError(t, err)

	err = h.RunOperation(AfterDocsGenerate, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish-wiki")
	assert.Contains(t, err.Error(), "after.docs.generate")
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 3, exitErr.ExitCode())
}

func TestMissingHookBinaryHintIsOperationAware(t *testing.T) {
	raw := map[string]any{"publish": map[string]any{
		"kind": "command", "command": "atmos-test-no-such-binary", "events": []any{"after.docs.generate"},
	}}
	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, docsTestOperation())
	require.NoError(t, err)

	err = h.RunOperation(AfterDocsGenerate, nil)

	require.Error(t, err)
	hints := strings.ToLower(strings.Join(errUtils.AllHints(err), "\n"))
	assert.NotEmpty(t, hints)
	assert.NotContains(t, hints, "terraform")
	assert.NotContains(t, hints, "dependencies.tools", "operations have no component dependencies section")
}

func TestOperationHooksNonMapHooksGetClearError(t *testing.T) {
	op := docsTestOperation()
	op.ConfigPath = "docs.generate.readme.hooks"
	raw := map[string]any{"publish": []any{map[string]any{"kind": "command"}}, "other": "text"}

	h, err := NewOperationHooks(&schema.AtmosConfiguration{}, raw, op)

	require.ErrorIs(t, err, errUtils.ErrInvalidHooksSection)
	assert.Nil(t, h)
	assert.Contains(t, err.Error(), "docs.generate.readme.hooks")
	assert.Contains(t, err.Error(), "docs.generate.readme.hooks.other", "the first offending entry in name order is named")
	hints := strings.Join(errUtils.AllHints(err), "\n")
	assert.Contains(t, hints, "kind: step")
	for _, leaked := range []string{"hookAlias", "hooks.Hook", "cannot unmarshal"} {
		assert.NotContains(t, err.Error(), leaked)
		assert.NotContains(t, hints, leaked)
	}
}

func TestOperationHooksValidatedAtConstruction(t *testing.T) {
	tests := []struct {
		name    string
		hook    map[string]any
		wantErr bool
		contain []string
	}{
		{name: "step missing required field", hook: map[string]any{"kind": "step", "type": "http", "events": []any{"after.docs.generate"}, "with": map[string]any{"method": "POST"}}, wantErr: true, contain: []string{"publish", "url"}},
		{name: "unknown kind", hook: map[string]any{"kind": "no-such-kind", "events": []any{"after.docs.generate"}}, wantErr: true, contain: []string{"publish", "no-such-kind"}},
		{name: "unknown step type", hook: map[string]any{"kind": "step", "type": "no-such-step", "events": []any{"after.docs.generate"}}, wantErr: true, contain: []string{"publish", "no-such-step"}},
		{name: "valid step with templated field", hook: map[string]any{"kind": "step", "type": "http", "events": []any{"after.docs.generate"}, "with": map[string]any{"url": "http://x/{{ .docs.type }}"}}},
		{name: "valid command", hook: map[string]any{"kind": "command", "command": "some-tool", "events": []any{"after.docs.generate"}}},
		{name: "valid steps", hook: map[string]any{"kind": "steps", "events": []any{"after.docs.generate"}, "with": []any{map[string]any{"type": "http", "url": "http://x"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewOperationHooks(&schema.AtmosConfiguration{}, map[string]any{"publish": tt.hook}, docsTestOperation())
			if !tt.wantErr {
				require.NoError(t, err)
				assert.NotNil(t, h)
				return
			}
			require.Error(t, err)
			assert.Nil(t, h)
			full := err.Error() + "\n" + strings.Join(errUtils.AllHints(err), "\n")
			for _, want := range tt.contain {
				assert.Contains(t, full, want)
			}
		})
	}
}

func TestForeignOperationEvents(t *testing.T) {
	items := map[string]Hook{
		"ok":      {Events: []string{"before.docs.generate", "after.docs.generate"}},
		"foreign": {Events: []string{"after.terraform.apply", "after.docs.generate"}},
		"empty":   {},
	}
	got := foreignOperationEvents(items, "docs")
	assert.Equal(t, map[string][]string{"foreign": {"after.terraform.apply"}}, got)

	// Declaring a foreign event is valid: construction succeeds.
	_, err := NewOperationHooks(&schema.AtmosConfiguration{}, map[string]any{
		"foreign": map[string]any{"kind": "command", "command": "tool", "events": []any{"after.terraform.apply"}},
	}, docsTestOperation())
	require.NoError(t, err)
}
