package starlark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/castplan"
	"github.com/cloudposse/atmos/pkg/script"
)

// fakeCastLibrary is a step library that also provides cast services.
type fakeCastLibrary struct {
	*MockStepLibrary
	generate func(context.Context, *castplan.Plan, *automation.StepCall, func(string) error) (string, error)
	calls    []*automation.StepCall
}

func (f *fakeCastLibrary) ValidateCast(_ *castplan.Plan, call *automation.StepCall) error {
	f.calls = append(f.calls, call)
	return nil
}

func (f *fakeCastLibrary) GenerateCast(ctx context.Context, plan *castplan.Plan, call *automation.StepCall, validate func(string) error) (string, error) {
	f.calls = append(f.calls, call)
	return f.generate(ctx, plan, call, validate)
}

func newFakeCastLibrary(t *testing.T) *fakeCastLibrary {
	t.Helper()
	fake := &fakeCastLibrary{MockStepLibrary: NewMockStepLibrary(gomock.NewController(t))}
	fake.EXPECT().Names().Return(nil).AnyTimes()
	fake.EXPECT().Fork().Return(fake).AnyTimes()
	return fake
}

func executeCastScript(t *testing.T, library automation.StepLibrary, dir, source string) (script.Result, error) {
	t.Helper()
	return New().Execute(t.Context(), script.Spec{Source: source, WorkingDirectory: dir, Steps: library})
}

// stagedRecording writes a minimal valid recording for plan whose only output is text.
func stagedRecording(t *testing.T, path string, plan *castplan.Plan, text string) {
	t.Helper()
	hash, err := plan.Fingerprint()
	require.NoError(t, err)
	content := `{"version":3,"width":80,"height":24}` + "\n"
	for _, event := range [][]any{
		{0, "m", castplan.Marker{Kind: "plan", Fingerprint: hash}.Encode()},
		{0.1, "o", text},
		{0.1, "m", castplan.Marker{Kind: "complete", Fingerprint: hash}.Encode()},
	} {
		encoded, marshalErr := json.Marshal(event)
		require.NoError(t, marshalErr)
		content += string(encoded) + "\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestCastRequiresHostCastServices(t *testing.T) {
	declaration := `rec = cast.recorder(output="demo.cast")
rec.prompt()
`
	t.Run("no step library", func(t *testing.T) {
		_, err := executeCastScript(t, nil, t.TempDir(), declaration+"rec.validate()")
		require.ErrorContains(t, err, "host has not provided cast services")
	})

	t.Run("step library without cast support", func(t *testing.T) {
		library := NewMockStepLibrary(gomock.NewController(t))
		library.EXPECT().Names().Return(nil).AnyTimes()
		library.EXPECT().Fork().Return(library).AnyTimes()
		for _, call := range []string{"validate", "generate"} {
			_, err := executeCastScript(t, library, t.TempDir(), declaration+"rec."+call+"()")
			require.ErrorContains(t, err, "host has not provided cast services", call)
		}
	})
}

func TestCastHostMethodsRequireCastServices(t *testing.T) {
	host := &castHost{}
	thread := &starlark.Thread{Name: "test"}
	plan := &castplan.Plan{}
	require.ErrorContains(t, host.Check(thread, plan), "cast services")
	_, err := host.Generate(thread, plan, nil)
	require.ErrorContains(t, err, "cast services")
}

func TestCastValidateRequiresOutputPath(t *testing.T) {
	library := newFakeCastLibrary(t)
	_, err := executeCastScript(t, library, t.TempDir(), `rec = cast.recorder()
rec.prompt()
rec.validate()`)
	require.ErrorContains(t, err, "requires output.cast for saved validation")
}

func TestCastValidateResolvesRelativeOutputAgainstWorkingDirectory(t *testing.T) {
	library := newFakeCastLibrary(t)
	dir := t.TempDir()
	_, err := executeCastScript(t, library, dir, `rec = cast.recorder(output="demo.cast")
rec.prompt()
rec.validate()`)
	// The saved recording does not exist, so saved validation fails after the structural check.
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, filepath.Join(dir, "demo.cast"))
	require.NotEmpty(t, library.calls)
	require.Equal(t, "cast", library.calls[0].Type)
	require.Equal(t, dir, library.calls[0].WorkingDirectory)
}

func TestCastGenerateNormalizesBeforeRunningThePolicy(t *testing.T) {
	dir := t.TempDir()

	t.Run("policy sees normalized output", func(t *testing.T) {
		library := newFakeCastLibrary(t)
		var recordedPlan *castplan.Plan
		staged := filepath.Join(dir, "staged.cast")
		library.generate = func(_ context.Context, plan *castplan.Plan, _ *automation.StepCall, validate func(string) error) (string, error) {
			recordedPlan = plan
			stagedRecording(t, staged, plan, "working in /checkout/project")
			if err := validate(staged); err != nil {
				return "", err
			}
			normalized, err := os.ReadFile(staged)
			require.NoError(t, err)
			require.Contains(t, string(normalized), "working in /repo/project")
			return staged, nil
		}
		result, err := executeCastScript(t, library, dir, `def policy(text):
    if "/checkout" in text:
        fail("machine path leaked: " + text)
rec = cast.recorder(output="demo.cast", policy=policy, replacements={"/checkout": "/repo"})
output = rec.generate()`)
		require.NoError(t, err)
		require.Equal(t, staged, result.Value)
		require.Equal(t, map[string]string{"/checkout": "/repo"}, recordedPlan.Replacements)
	})

	t.Run("normalization failure aborts before the policy", func(t *testing.T) {
		library := newFakeCastLibrary(t)
		library.generate = func(_ context.Context, _ *castplan.Plan, _ *automation.StepCall, validate func(string) error) (string, error) {
			return "", validate(filepath.Join(dir, "missing-staged.cast"))
		}
		_, err := executeCastScript(t, library, dir, `def policy(text):
    fail("policy must not run")
rec = cast.recorder(output="demo.cast", policy=policy, replacements={"/checkout": "/repo"})
rec.generate()`)
		require.ErrorIs(t, err, os.ErrNotExist)
		require.NotContains(t, err.Error(), "policy must not run")
	})

	t.Run("recording evidence mismatch is reported", func(t *testing.T) {
		library := newFakeCastLibrary(t)
		library.generate = func(_ context.Context, _ *castplan.Plan, _ *automation.StepCall, validate func(string) error) (string, error) {
			bad := filepath.Join(dir, "bad.cast")
			require.NoError(t, os.WriteFile(bad, []byte(`{"version":3,"width":80,"height":24}`+"\n"), 0o600))
			return "", validate(bad)
		}
		_, err := executeCastScript(t, library, dir, `rec = cast.recorder(output="demo.cast")
rec.generate()`)
		require.ErrorIs(t, err, errUtils.ErrAutomation)
		require.ErrorContains(t, err, "regenerate the cast")
	})
}
