package autoinit

// pipeline_test.go covers the exported orchestration API in pipeline.go that has no test
// coverage yet: Recover, Recovery.summary, ApplyRecovery, and RecoverInit. These are
// documented, exported package API (Recover/RecoverInit both have callers-never-reimplement
// doc comments) even though internal/exec currently hand-rolls its own equivalent of Recover
// (recoverFromInitRequired in terraform_execute_helpers_exec.go) instead of calling it -- that
// doc/reality mismatch is out of scope for a coverage task, but the exported functions
// themselves still need direct tests exactly like diagnostics_test.go already tests
// Classify/ShouldRecover.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ──────────────────────────────────────────────────────────────────────────────
// ApplyRecovery
// ──────────────────────────────────────────────────────────────────────────────

func TestApplyRecovery(t *testing.T) {
	tests := []struct {
		name string
		args []string
		rec  Recovery
		want []string
	}{
		{
			name: "no recovery flags requested leaves args unchanged",
			args: []string{"init"},
			rec:  Recovery{Run: true},
			want: []string{"init"},
		},
		{
			name: "adds -upgrade when requested and absent",
			args: []string{"init"},
			rec:  Recovery{Run: true, WithUpgrade: true},
			want: []string{"init", "-upgrade"},
		},
		{
			name: "adds -reconfigure when requested and absent",
			args: []string{"init"},
			rec:  Recovery{Run: true, WithReconfigure: true},
			want: []string{"init", "-reconfigure"},
		},
		{
			name: "adds both when both requested",
			args: []string{"init"},
			rec:  Recovery{Run: true, WithUpgrade: true, WithReconfigure: true},
			want: []string{"init", "-upgrade", "-reconfigure"},
		},
		{
			name: "does not duplicate -upgrade already present",
			args: []string{"init", "-upgrade"},
			rec:  Recovery{Run: true, WithUpgrade: true},
			want: []string{"init", "-upgrade"},
		},
		{
			name: "does not duplicate -reconfigure already present",
			args: []string{"init", "-reconfigure"},
			rec:  Recovery{Run: true, WithReconfigure: true},
			want: []string{"init", "-reconfigure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyRecovery(tt.args, tt.rec)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestApplyRecovery_DoesNotMutateInput verifies ApplyRecovery returns a clone, not an
// alias into the caller's slice -- appending a recovery flag must never silently grow
// (or corrupt, on a later append) the caller's own args slice.
func TestApplyRecovery_DoesNotMutateInput(t *testing.T) {
	original := []string{"init"}
	got := ApplyRecovery(original, Recovery{Run: true, WithUpgrade: true})

	assert.Equal(t, []string{"init"}, original, "input slice must be unchanged")
	assert.Equal(t, []string{"init", "-upgrade"}, got)
}

// ──────────────────────────────────────────────────────────────────────────────
// Recovery.summary
// ──────────────────────────────────────────────────────────────────────────────

func TestRecoverySummary(t *testing.T) {
	tests := []struct {
		name string
		rec  Recovery
		want string
	}{
		{
			name: "reconfigure and upgrade both required",
			rec:  Recovery{Run: true, WithReconfigure: true, WithUpgrade: true},
			want: "backend configuration changed and provider/module constraints require -upgrade",
		},
		{
			name: "reconfigure only",
			rec:  Recovery{Run: true, WithReconfigure: true},
			want: "backend configuration changed",
		},
		{
			name: "upgrade only",
			rec:  Recovery{Run: true, WithUpgrade: true},
			want: "provider/module constraints require -upgrade",
		},
		{
			name: "neither flag set falls back to the generic message",
			rec:  Recovery{Run: true},
			want: "init is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.rec.summary())
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Recover
// ──────────────────────────────────────────────────────────────────────────────

// TestRecover_NilErrIsNoOp verifies Recover never calls RunInit/Retry when there was no
// failure to recover from in the first place.
func TestRecover_NilErrIsNoOp(t *testing.T) {
	p := RecoverParams{
		Err:     nil,
		RunInit: func(Recovery) error { t.Fatal("RunInit must not be called when Err is nil"); return nil },
		Retry:   func() error { t.Fatal("Retry must not be called when Err is nil"); return nil },
	}

	assert.NoError(t, Recover(p))
}

// TestRecover_SkipReturnsErrUnchanged verifies that p.Skip (the command that failed was
// itself `init`) short-circuits before any classification or recovery attempt.
func TestRecover_SkipReturnsErrUnchanged(t *testing.T) {
	mainErr := errors.New("boom")
	p := RecoverParams{
		Output:  "Error: Backend configuration changed",
		Err:     mainErr,
		Skip:    true,
		RunInit: func(Recovery) error { t.Fatal("RunInit must not be called when Skip is set"); return nil },
		Retry:   func() error { t.Fatal("Retry must not be called when Skip is set"); return nil },
	}

	err := Recover(p)
	require.Error(t, err)
	assert.Same(t, mainErr, err)
}

// TestRecover_NoDiagnosisReturnsErrUnchanged verifies that output containing no known
// "init required" signature leaves the original error untouched and never invokes RunInit.
func TestRecover_NoDiagnosisReturnsErrUnchanged(t *testing.T) {
	mainErr := errors.New("some unrelated validation failure")
	p := RecoverParams{
		Output:  "Error: some unrelated validation failure",
		Err:     mainErr,
		Mode:    schema.TerraformInitModeAuto,
		RunInit: func(Recovery) error { t.Fatal("RunInit must not be called without a diagnosis"); return nil },
	}

	err := Recover(p)
	require.Error(t, err)
	assert.Same(t, mainErr, err)
}

// TestRecover_PolicyErrorIsJoinedNotSwallowed verifies that when ShouldRecover reports a
// policy error (e.g. the caller explicitly opted out via --skip-init), Recover joins it
// with the original failure instead of silently forcing an init anyway.
func TestRecover_PolicyErrorIsJoinedNotSwallowed(t *testing.T) {
	mainErr := errors.New("plan failed")
	p := RecoverParams{
		Output:   "Error: Backend configuration changed",
		Err:      mainErr,
		Mode:     schema.TerraformInitModeAuto,
		OptedOut: true,
		RunInit:  func(Recovery) error { t.Fatal("RunInit must not be called on a policy error"); return nil },
	}

	err := Recover(p)
	require.Error(t, err)
	assert.True(t, errors.Is(err, mainErr), "joined error must still satisfy errors.Is against the original failure")
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInitRequired), "joined error must surface the opted-out policy error")
}

// TestRecover_RunInitFailureIsJoined verifies that a failed forced init is joined with the
// original main-command error rather than replacing it, and that Retry is never reached.
func TestRecover_RunInitFailureIsJoined(t *testing.T) {
	mainErr := errors.New("plan failed")
	initErr := errors.New("init failed too")
	var warned string

	p := RecoverParams{
		Output: "Error: Backend configuration changed",
		Err:    mainErr,
		Mode:   schema.TerraformInitModeAuto,
		Warn:   func(msg string) { warned = msg },
		RunInit: func(rec Recovery) error {
			assert.True(t, rec.WithReconfigure)
			return initErr
		},
		Retry: func() error { t.Fatal("Retry must not be called when RunInit fails"); return nil },
	}

	err := Recover(p)
	require.Error(t, err)
	assert.True(t, errors.Is(err, mainErr))
	assert.True(t, errors.Is(err, initErr))
	assert.Contains(t, warned, "backend configuration changed")
}

// TestRecover_SuccessfulRecoveryRetriesOnce verifies the full happy path: a diagnosed
// failure recovers via RunInit and then retries the main command exactly once, returning
// whatever Retry itself returns.
func TestRecover_SuccessfulRecoveryRetriesOnce(t *testing.T) {
	mainErr := errors.New("plan failed")
	var runInitCalls, retryCalls int

	p := RecoverParams{
		Output: "Error: this configuration must use terraform init -upgrade",
		Err:    mainErr,
		Mode:   schema.TerraformInitModeAuto,
		Warn:   func(string) {},
		RunInit: func(rec Recovery) error {
			runInitCalls++
			assert.True(t, rec.WithUpgrade)
			return nil
		},
		Retry: func() error {
			retryCalls++
			return nil
		},
	}

	err := Recover(p)
	require.NoError(t, err)
	assert.Equal(t, 1, runInitCalls)
	assert.Equal(t, 1, retryCalls)
}

// TestRecover_RetryErrorPropagates verifies that a retry which fails again after a
// successful forced init surfaces the retry's own error (not the original mainErr) --
// the retry is a fresh attempt, so its own failure is what matters to the caller.
func TestRecover_RetryErrorPropagates(t *testing.T) {
	retryErr := errors.New("still failing after retry")
	p := RecoverParams{
		Output:  "Error: Backend configuration changed",
		Err:     errors.New("plan failed"),
		Mode:    schema.TerraformInitModeAuto,
		RunInit: func(Recovery) error { return nil },
		Retry:   func() error { return retryErr },
	}

	err := Recover(p)
	require.Error(t, err)
	assert.Same(t, retryErr, err)
}

// TestRecover_NilWarnIsSafe verifies that Recover never dereferences a nil Warn func --
// callers are not required to supply one.
func TestRecover_NilWarnIsSafe(t *testing.T) {
	p := RecoverParams{
		Output:  "Error: Backend configuration changed",
		Err:     errors.New("plan failed"),
		Mode:    schema.TerraformInitModeAuto,
		Warn:    nil,
		RunInit: func(Recovery) error { return nil },
		Retry:   func() error { return nil },
	}

	assert.NotPanics(t, func() {
		err := Recover(p)
		assert.NoError(t, err)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// RecoverInit
// ──────────────────────────────────────────────────────────────────────────────

// TestRecoverInit_NilErrIsNoOp verifies RecoverInit never calls Rerun when init itself
// didn't fail.
func TestRecoverInit_NilErrIsNoOp(t *testing.T) {
	p := RecoverInitParams{
		Err:   nil,
		Rerun: func(Recovery) error { t.Fatal("Rerun must not be called when Err is nil"); return nil },
	}

	rec, err := RecoverInit(p)
	require.NoError(t, err)
	assert.Equal(t, Recovery{}, rec)
}

// TestRecoverInit_NoDiagnosisReturnsErrUnchanged verifies that output with no recognized
// "init required" signature leaves the original init error untouched.
func TestRecoverInit_NoDiagnosisReturnsErrUnchanged(t *testing.T) {
	initErr := errors.New("some other init failure")
	p := RecoverInitParams{
		Output: "Error: some other init failure",
		Err:    initErr,
		Mode:   schema.TerraformInitModeAuto,
		Rerun:  func(Recovery) error { t.Fatal("Rerun must not be called without a diagnosis"); return nil },
	}

	rec, err := RecoverInit(p)
	require.Error(t, err)
	assert.Same(t, initErr, err)
	assert.Equal(t, Recovery{}, rec)
}

// TestRecoverInit_PolicyErrorIsJoined verifies a policy error (e.g. -upgrade required but
// disabled by config) is joined with the original init error, not swallowed.
func TestRecoverInit_PolicyErrorIsJoined(t *testing.T) {
	initErr := errors.New("init failed")
	p := RecoverInitParams{
		Output:  "Error: this configuration must use terraform init -upgrade",
		Err:     initErr,
		Mode:    schema.TerraformInitModeAuto,
		Upgrade: schema.TerraformInitUpgradeNever,
		Rerun:   func(Recovery) error { t.Fatal("Rerun must not be called on a policy error"); return nil },
	}

	rec, err := RecoverInit(p)
	require.Error(t, err)
	assert.True(t, errors.Is(err, initErr))
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInitUpgradeRequired))
	assert.Equal(t, Recovery{}, rec)
}

// TestRecoverInit_RerunSuccess verifies the happy path: a diagnosed -upgrade requirement
// reruns init once with the flag and returns the applied Recovery alongside a nil error.
func TestRecoverInit_RerunSuccess(t *testing.T) {
	var warnedMsg string
	var warnedKV []any
	p := RecoverInitParams{
		Output: "Error: this configuration must use terraform init -upgrade",
		Err:    errors.New("init failed"),
		Mode:   schema.TerraformInitModeAuto,
		Warn: func(msg string, kv ...any) {
			warnedMsg = msg
			warnedKV = kv
		},
		Rerun: func(rec Recovery) error {
			assert.True(t, rec.WithUpgrade)
			return nil
		},
	}

	rec, err := RecoverInit(p)
	require.NoError(t, err)
	assert.True(t, rec.WithUpgrade)
	assert.Contains(t, warnedMsg, "additional flags")
	assert.Contains(t, warnedKV, true) // upgrade=true is present among the kv pairs
}

// TestRecoverInit_RerunFailureReturnsAppliedRecovery verifies that when Rerun itself
// fails, RecoverInit still returns the Recovery it attempted (so a caller can log which
// flags were tried) alongside the rerun's own error -- unlike Recover, RecoverInit does
// NOT join the original error with the rerun error, since Rerun's failure is the more
// relevant one for a caller retrying init itself.
func TestRecoverInit_RerunFailureReturnsAppliedRecovery(t *testing.T) {
	rerunErr := errors.New("rerun failed")
	p := RecoverInitParams{
		Output: "Error: Backend configuration changed",
		Err:    errors.New("init failed"),
		Mode:   schema.TerraformInitModeAuto,
		Rerun:  func(Recovery) error { return rerunErr },
	}

	rec, err := RecoverInit(p)
	require.Error(t, err)
	assert.Same(t, rerunErr, err)
	assert.True(t, rec.WithReconfigure)
}

// TestRecoverInit_NilWarnIsSafe verifies RecoverInit never dereferences a nil Warn func.
func TestRecoverInit_NilWarnIsSafe(t *testing.T) {
	p := RecoverInitParams{
		Output: "Error: Backend configuration changed",
		Err:    errors.New("init failed"),
		Mode:   schema.TerraformInitModeAuto,
		Warn:   nil,
		Rerun:  func(Recovery) error { return nil },
	}

	assert.NotPanics(t, func() {
		_, err := RecoverInit(p)
		assert.NoError(t, err)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// OptedOut (partial coverage: exercised in diagnostics/pipeline flows elsewhere, but the
// deploy_run_init:false branch had no direct test)
// ──────────────────────────────────────────────────────────────────────────────

// TestOptedOut covers every opt-out signal directly against the exported OptedOut
// function, table-driven, including the deploy_run_init:false-on-a-deploy branch that
// wasn't exercised anywhere in the package's own tests.
func TestOptedOut(t *testing.T) {
	tests := []struct {
		name string
		cfg  schema.AtmosConfiguration
		info schema.ConfigAndStacksInfo
		want bool
	}{
		{
			name: "skip-init flag opts out",
			info: schema.ConfigAndStacksInfo{SkipInit: true},
			want: true,
		},
		{
			name: "init.mode: never opts out",
			cfg: schema.AtmosConfiguration{
				Components: schema.Components{Terraform: schema.Terraform{Init: schema.TerraformInit{Mode: schema.TerraformInitModeNever}}},
			},
			want: true,
		},
		{
			name: "deploy without deploy_run_init opts out",
			info: schema.ConfigAndStacksInfo{SubCommand: "deploy"},
			cfg: schema.AtmosConfiguration{
				Components: schema.Components{Terraform: schema.Terraform{DeployRunInit: false}},
			},
			want: true,
		},
		{
			name: "deploy with deploy_run_init does not opt out",
			info: schema.ConfigAndStacksInfo{SubCommand: "deploy"},
			cfg: schema.AtmosConfiguration{
				Components: schema.Components{Terraform: schema.Terraform{DeployRunInit: true}},
			},
			want: false,
		},
		{
			name: "plan with no opt-out signals does not opt out",
			info: schema.ConfigAndStacksInfo{SubCommand: "plan"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OptedOut(&tt.cfg, &tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}
