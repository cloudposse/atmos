package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSharedContextAcrossEngines(t *testing.T) {
	uid, gid := 42, 43
	username := "operator"
	capacity := uint64(8000000000)
	snapshot := &RuntimeContext{
		Env: map[string]string{"CLAUDECODE": "1"}, Atmos: AtmosFacts{Version: "1.233.0", FIPS: true},
		Runtime: HostFacts{OS: "linux", Arch: "arm64", User: UserFacts{UID: &uid, GID: &gid, Username: &username}, Memory: MemoryFacts{AvailableBytes: &capacity}, Disk: DiskFacts{AvailableBytes: &capacity, Path: "fixture"}},
		Time:    ClockFacts{Now: "2026-10-20T15:00:00Z", Date: "2026-10-20", Weekday: "Tuesday", Timezone: "UTC"},
	}
	checks := map[string]ContextCheck{
		"version":    {Kind: "semver", Path: "/atmos/version", Constraint: ">= 1.233.0"},
		"window":     {Kind: "schedule", Timezone: "America/New_York", Weekdays: []string{"Tuesday"}, StartTime: "09:00", EndTime: "17:00"},
		"expiration": {Kind: "date", Path: "/input/expires", Format: "date", Start: "2026-10-01", End: "2026-11-01"},
	}
	event := Dataset{Status: Available, Complete: true, Data: json.RawMessage(`{"pull_request":{"labels":[{"name":"preview"}]}}`)}
	ci := &CISnapshot{Provider: "github-actions", Schema: "github-actions/v1", Event: event, Facts: map[string]Dataset{"approvers": ApproverDataset([]string{"alice"}, Provenance{})}}
	schema := `{"type":"object","properties":{"env":{"properties":{"CLAUDECODE":{"const":"1"}},"required":["CLAUDECODE"]},"atmos":{"properties":{"fips":{"const":true}}},"runtime":{"properties":{"os":{"const":"linux"},"arch":{"const":"arm64"},"user":{"properties":{"uid":{"const":42},"gid":{"const":43},"username":{"const":"operator"}}},"memory":{"properties":{"available_bytes":{"minimum":8000000000}}},"disk":{"properties":{"available_bytes":{"minimum":8000000000}}}}},"ci":{"properties":{"labels":{"contains":{"const":"preview"}},"approvers":{"contains":{"const":"alice"}}}},"checks":{"properties":{"version":{"const":true},"window":{"const":true},"expiration":{"const":true}}}}}`
	// Schema selects effects on violations, so invert the satisfying schema.
	adapters := []Rule{
		{Engine: CELEngine, Expression: `env.CLAUDECODE == '1' && atmos.fips && runtime.os == 'linux' && runtime.arch == 'arm64' && runtime.user.uid == 42 && runtime.user.gid == 43 && runtime.user.username == 'operator' && runtime.memory.available_bytes >= 8000000000 && runtime.disk.available_bytes >= 8000000000 && 'preview' in ci.labels && 'alice' in ci.approvers && checks.version && checks.window && checks.expiration`},
		{Engine: StarlarkEngine, Expression: `return ctx.env["CLAUDECODE"] == "1" and ctx.atmos["fips"] and ctx.runtime["os"] == "linux" and ctx.runtime["arch"] == "arm64" and ctx.runtime["user"]["uid"] == 42 and ctx.runtime["user"]["gid"] == 43 and ctx.runtime["user"]["username"] == "operator" and ctx.runtime["memory"]["available_bytes"] >= 8000000000 and ctx.runtime["disk"]["available_bytes"] >= 8000000000 and "preview" in ctx.ci["labels"] and "alice" in ctx.ci["approvers"] and ctx.checks["version"] and ctx.checks["window"] and ctx.checks["expiration"]`},
		{Engine: OPAEngine, OPA: &OPAConfig{Query: "data.test.deny", Modules: map[string]string{"test.rego": `package test
 default deny = false
 deny { c := data.atmos_context; c.env.CLAUDECODE == "1"; c.atmos.fips; c.runtime.os == "linux"; c.runtime.arch == "arm64"; c.runtime.user.uid == 42; c.runtime.user.gid == 43; c.runtime.user.username == "operator"; c.runtime.memory.available_bytes >= 8000000000; c.runtime.disk.available_bytes >= 8000000000; c.ci.labels[_] == "preview"; c.ci.approvers[_] == "alice"; c.checks.version; c.checks.window; c.checks.expiration }`}}},
		{Engine: JSONSchemaEngine, Schema: &SchemaConfig{Target: "context", Entry: schemaBase + "test.json", Resources: map[string]json.RawMessage{schemaBase + "test.json": json.RawMessage(`{"not":` + schema + `}`)}}},
	}
	provider := NewMockContextProvider(gomock.NewController(t))
	calls := 0
	provider.EXPECT().Collect(gomock.Any()).DoAndReturn(func(context.Context) (*RuntimeContext, error) { calls++; return snapshot, nil }).Times(2)
	engine := NewEngine(WithContextProvider(provider))
	for i := range adapters {
		adapters[i].ID = adapters[i].Engine
		adapters[i].On = Command
		adapters[i].Checks = checks
		adapters[i].Effects.Deny = true
	}
	request := &Request{Kind: Command, Input: json.RawMessage(`{"expires":"2026-10-31"}`), CI: ci}
	decision, err := engine.Evaluate(t.Context(), request, adapters)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Len(t, decision.Findings, 4)
	for _, finding := range decision.Findings {
		require.Equal(t, Deny, finding.Status, finding.RuleID)
	}
	require.Nil(t, request.Context)
	snapshot.Env["CLAUDECODE"] = "0"
	decision, err = engine.Evaluate(t.Context(), request, adapters)
	require.NoError(t, err)
	for _, finding := range decision.Findings {
		require.Equal(t, NotApplicable, finding.Status, finding.RuleID)
	}
}

func TestSharedCheckBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		check ContextCheck
		value string
		want  bool
	}{
		{"numeric version", ContextCheck{Kind: "semver", Path: "/input/value", Constraint: "> 1.99.0"}, "1.100.0", true},
		{"prerelease", ContextCheck{Kind: "semver", Path: "/input/value", Constraint: ">= 1.233.0"}, "1.233.0-rc.1", false},
		{"unknown build", ContextCheck{Kind: "semver", Path: "/input/value", Constraint: ">= 1.233.0"}, "test", false},
		{"invalid leap day", ContextCheck{Kind: "date", Path: "/input/value", Format: "date"}, "2025-02-29", false},
		{"valid leap day", ContextCheck{Kind: "date", Path: "/input/value", Format: "date"}, "2024-02-29", true},
		{"range start", ContextCheck{Kind: "date", Path: "/input/value", Format: "date", Start: "2026-01-01", End: "2026-02-01"}, "2026-01-01", true},
		{"range end", ContextCheck{Kind: "date", Path: "/input/value", Format: "date", Start: "2026-01-01", End: "2026-02-01"}, "2026-02-01", false},
		{"DST before spring", ContextCheck{Kind: "schedule", Timezone: "America/New_York", StartTime: "09:00", EndTime: "17:00"}, "2026-03-06T14:00:00Z", true},
		{"DST after spring", ContextCheck{Kind: "schedule", Timezone: "America/New_York", StartTime: "09:00", EndTime: "17:00"}, "2026-03-09T13:00:00Z", true},
		{"exclusive closing", ContextCheck{Kind: "schedule", Timezone: "America/New_York", StartTime: "09:00", EndTime: "17:00"}, "2026-03-09T21:00:00Z", false},
		{"weekday", ContextCheck{Kind: "schedule", Weekdays: []string{"Monday"}}, "2026-10-20T15:00:00Z", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.check.validate())
			require.Equal(t, tc.want, tc.check.matches(tc.value))
		})
	}
	for _, check := range []ContextCheck{
		{Kind: "semver", Path: "/input/value", Constraint: "broken"},
		{Kind: "date", Path: "/input/value", Format: "unknown"},
		{Kind: "schedule", Timezone: "Not/AZone"},
		{Kind: "schedule", Timezone: "Local"},
		{Kind: "schedule", Weekdays: []string{"Moonday"}},
		{Kind: "schedule", StartTime: "17:00", EndTime: "09:00"},
		{Kind: "date", Path: "/input/value", Start: "tomorrow"},
		{Kind: "date", Path: "/checks/foo"},
	} {
		require.Error(t, validateChecks(map[string]ContextCheck{"test": check}))
	}
	_, err := evaluateChecks(map[string]ContextCheck{"test": {Kind: "date", Path: "/input/missing"}}, map[string]any{"input": map[string]any{}})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestHostContext(t *testing.T) {
	t.Setenv("ATMOS_CONTEXT_TEST", "value")
	snapshot, err := (HostContextProvider{}).Collect(t.Context())
	require.NoError(t, err)
	require.Equal(t, "value", snapshot.Env["ATMOS_CONTEXT_TEST"])
	require.NotEmpty(t, snapshot.Runtime.OS)
	require.NotEmpty(t, snapshot.Runtime.Arch)
	_, err = time.Parse(time.RFC3339Nano, snapshot.Time.Now)
	require.NoError(t, err)
	require.Equal(t, "UTC", snapshot.Time.Timezone)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = (HostContextProvider{}).Collect(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestContextMissingAndIsolation(t *testing.T) {
	snapshot := &RuntimeContext{Env: map[string]string{"SECRET": "never-print-this"}}
	engine := NewEngine()
	adapter := NewMockEvaluator(gomock.NewController(t))
	require.NoError(t, engine.Register("mutating-test", adapter))
	adapter.EXPECT().Evaluate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, evaluation Evaluation) (bool, error) {
		require.Equal(t, "never-print-this", evaluation.Request.Context.Env["SECRET"])
		evaluation.Request.Context.Env["SECRET"] = "changed"
		// Altering the source after materialization must not alter this adapter's copy.
		snapshot.Env["OTHER"] = "source-change"
		_, exists := evaluation.Request.Context.Env["OTHER"]
		require.False(t, exists)
		return false, nil
	})
	request := &Request{Kind: Command, Input: json.RawMessage(`{"env":{"SECRET":"spoofed"}}`), Context: snapshot}
	rules := []Rule{{ID: "mutator", On: Command, Engine: "mutating-test", Expression: "true"}, {ID: "verify", On: Command, Engine: CELEngine, Expression: `env.SECRET == 'never-print-this'`, Effects: Effects{Deny: true}}}
	decision, err := engine.Evaluate(t.Context(), request, rules)
	require.NoError(t, err)
	require.Equal(t, Deny, decision.Status)
	require.Equal(t, "never-print-this", snapshot.Env["SECRET"])
	_, err = engine.Evaluate(t.Context(), request, []Rule{{ID: "missing", On: Command, Engine: CELEngine, Expression: `env.SECRET / 2 > 0`}})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "never-print-this")
	for _, name := range []string{CELEngine, StarlarkEngine, OPAEngine, JSONSchemaEngine} {
		rule := Rule{ID: name, On: Command, Engine: name, Expression: "true", Checks: map[string]ContextCheck{"missing": {Kind: "date", Path: "/input/missing"}}}
		switch name {
		case StarlarkEngine:
			rule.Expression = "return True"
		case OPAEngine:
			rule.Expression = ""
			rule.OPA = &OPAConfig{Query: "data.test.ok", Modules: map[string]string{"test.rego": "package test\nok = true"}}
		case JSONSchemaEngine:
			rule.Expression = ""
			rule.Schema = &SchemaConfig{Entry: schemaBase + "test.json", Resources: map[string]json.RawMessage{schemaBase + "test.json": json.RawMessage(`{}`)}}
		}
		_, err = engine.Evaluate(t.Context(), request, []Rule{rule})
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable, name)
	}
}

func TestContextCIAvailability(t *testing.T) {
	cases := []struct {
		name, event string
		complete    bool
		want        any
	}{
		{"unknown event", `{}`, true, nil},
		{"missing labels", `{"pull_request":{}}`, true, nil},
		{"empty labels", `{"pull_request":{"labels":[]}}`, true, []any{}},
		{"sorted labels", `{"pull_request":{"labels":[{"name":"z"},{"name":"preview"}]}}`, true, []any{"preview", "z"}},
		{"partial event", `{"pull_request":{"labels":[]}}`, false, nil},
		{"malformed labels", `{"pull_request":{"labels":[{}]}}`, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &Request{Kind: Command, Input: json.RawMessage(`{}`), CI: &CISnapshot{Provider: "github-actions", Schema: "github-actions/v1", Event: Dataset{Data: json.RawMessage(tc.event), Status: Available, Complete: tc.complete}}}
			activation, err := evaluationActivation(Evaluation{Request: request})
			require.NoError(t, err)
			ci := activation["ci"].(map[string]any)
			require.Equal(t, tc.want, ci["labels"])
			require.Nil(t, ci["approvers"])
		})
	}
	require.Nil(t, normalizedCI(nil))
	unsupported := normalizedCI(map[string]any{"provider": "other"}).(map[string]any)
	require.Nil(t, unsupported["labels"])
}

func TestGitHubApproverSummary(t *testing.T) {
	now := time.Now().UTC()
	reviews := []GitHubReview{
		{ID: 1, Login: "ALICE", State: "APPROVED", CommitID: "head", SubmittedAt: now},
		{ID: 2, Login: "bob", State: "APPROVED", CommitID: "old", SubmittedAt: now},
		{ID: 3, Login: "carol", State: "APPROVED", CommitID: "head", SubmittedAt: now},
		{ID: 4, Login: "carol", State: "DISMISSED", CommitID: "head", SubmittedAt: now.Add(time.Second)},
		{ID: 5, Login: "author", State: "APPROVED", CommitID: "head", SubmittedAt: now},
		{ID: 6, Login: "ALICE", State: "COMMENTED", CommitID: "head", SubmittedAt: now.Add(time.Second)},
	}
	names, err := GitHubApprovers("head", "author", reviews)
	require.NoError(t, err)
	require.Equal(t, []string{"alice"}, names)
	_, err = GitHubApprovers("", "author", reviews)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	_, err = GitHubApprovers("head", "author", append(reviews, reviews[0]))
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestContainerMemoryLimits(t *testing.T) {
	root := t.TempDir()
	total, available := uint64(100), uint64(70)
	for _, tc := range []struct {
		limit, usage     string
		total, available uint64
		scope            string
	}{
		{"80", "30", 80, 50, "container"},
		{"80", "90", 80, 0, "container"},
		{"max", "30", 100, 70, "host"},
		{"200", "30", 100, 70, "host"},
		{"80", "invalid", 100, 70, "host"},
		{"80", "0", 80, 70, "container"},
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "limit"), []byte(tc.limit), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(root, "usage"), []byte(tc.usage), 0o600))
		facts := MemoryFacts{AvailableBytes: &available, TotalBytes: &total, Scope: "host"}
		applyMemoryLimit(&facts, root, "limit", "usage")
		require.Equal(t, tc.total, *facts.TotalBytes)
		require.Equal(t, tc.available, *facts.AvailableBytes)
		require.Equal(t, tc.scope, facts.Scope)
	}
}

func TestContainerMemoryParentLimit(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	for path, value := range map[string]string{filepath.Join(root, "limit"): "50", filepath.Join(root, "usage"): "40", filepath.Join(child, "limit"): "80", filepath.Join(child, "usage"): "20"} {
		require.NoError(t, os.WriteFile(path, []byte(value), 0o600))
	}
	total, available := uint64(100), uint64(90)
	facts := MemoryFacts{TotalBytes: &total, AvailableBytes: &available, Scope: "host"}
	applyMemoryHierarchy(&facts, root, "/child", "limit", "usage")
	require.Equal(t, uint64(50), *facts.TotalBytes)
	require.Equal(t, uint64(10), *facts.AvailableBytes)
	require.Equal(t, root, cgroupPath(root, "/../../"))
}

func TestEngineCollectors(t *testing.T) {
	controller := gomock.NewController(t)
	provider := NewMockContextProvider(controller)
	collector := NewMockFactCollector(controller)
	engine := NewEngine(WithContextProvider(provider), WithFactCollector(collector))
	request := &Request{Kind: Command, Input: json.RawMessage(`{}`), CI: &CISnapshot{Provider: "github-actions", Schema: "github-actions/v1", Facts: map[string]Dataset{}}}
	rule := Rule{ID: "review", On: Command, Engine: CELEngine, Expression: `'alice' in ci.approvers`, Effects: Effects{Deny: true}}
	// Unrelated capabilities do not collect runtime or network evidence.
	decision, err := engine.Evaluate(t.Context(), request, []Rule{{ID: "other", On: File}})
	require.NoError(t, err)
	require.Equal(t, NotApplicable, decision.Status)
	provider.EXPECT().Collect(gomock.Any()).Return(&RuntimeContext{Env: map[string]string{}}, nil)
	collector.EXPECT().CollectFacts(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, detached *Request, _ []Rule) error {
		detached.CI.Facts["approvers"] = ApproverDataset([]string{"alice"}, Provenance{})
		return nil
	})
	decision, err = engine.Evaluate(t.Context(), request, []Rule{rule})
	require.NoError(t, err)
	require.Equal(t, Deny, decision.Status)
	require.Empty(t, request.CI.Facts)
	provider.EXPECT().Collect(gomock.Any()).Return(nil, errUtils.ErrPolicyInputUnavailable)
	decision, err = engine.Evaluate(t.Context(), request, []Rule{rule})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Equal(t, EvaluationError, decision.Status)
	provider.EXPECT().Collect(gomock.Any()).Return(&RuntimeContext{}, nil)
	collector.EXPECT().CollectFacts(gomock.Any(), gomock.Any(), gomock.Any()).Return(errUtils.ErrPolicyInputUnavailable)
	_, err = engine.Evaluate(t.Context(), request, []Rule{rule})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestContextPointerAndCheckValidation(t *testing.T) {
	value := map[string]any{"items": []any{map[string]any{"a/b~c": "2026-10-20"}}}
	got, ok := contextPointer(value, "/items/0/a~1b~0c")
	require.True(t, ok)
	require.Equal(t, "2026-10-20", got)
	for _, path := range []string{"/items/1", "/items/-1", "/items/01", "/items/+0", "/items/nope", "/items/0/a~1b~0c/child"} {
		_, ok = contextPointer(value, path)
		require.False(t, ok, path)
	}
	for _, path := range []string{"relative", "/checks/x", "/env/~", "/env/~2"} {
		require.False(t, validContextPath(path), path)
	}
	require.True(t, validContextPath("/env/a~1b~0c"))
	checks := map[string]ContextCheck{"invalid-name": {Kind: "schedule"}}
	require.Error(t, validateChecks(checks))
	for _, check := range []ContextCheck{
		{Kind: "unknown"},
		{Kind: "semver", Constraint: ">=1.0.0"},
		{Kind: "date", Path: "/input/a", Weekdays: []string{"Monday"}},
		{Kind: "date", Path: "/input/a", Format: "date", Start: "2026-02-01", End: "2026-01-01"},
		{Kind: "date", Path: "/input/a", End: "invalid"},
		{Kind: "schedule", Format: "date"},
		{Kind: "schedule", StartTime: "09:00"},
		{Kind: "schedule", StartTime: "25:00", EndTime: "17:00"},
		{Kind: "schedule", StartTime: "09:00", EndTime: "invalid"},
	} {
		require.Error(t, check.validate())
	}
	clock := ContextCheck{Kind: "date", Path: "/input/clock", Format: "time", Start: "09:00:00", End: "17:00:00"}
	require.NoError(t, clock.validate())
	require.True(t, clock.matches("09:00:00"))
	require.False(t, clock.matches("17:00:00"))
}
