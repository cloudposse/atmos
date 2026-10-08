package starlark

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
)

func TestCICheckCreatesAndUpdatesTheSameCheck(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	gomock.InOrder(
		m.EXPECT().Check(gomock.Any(), ci.CheckRequest{
			Name: "build", State: ci.CheckRunStateInProgress, Description: "compiling", URL: "https://run/1",
		}).Return(ci.Receipt{Check: &ci.CheckRun{ID: 7, Name: "build", DetailsURL: "https://checks/7"}}, nil),
		m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{
			Name: "build", ID: 7, State: ci.CheckRunStateSuccess, Description: "done", URL: "",
		}).Return(ci.Receipt{Check: &ci.CheckRun{ID: 7, Name: "build", DetailsURL: "https://checks/7/final"}}, nil),
	)
	stdout, _, err := runCI(t, m, `
h = ci.check("build", state="in_progress", description="compiling", url="https://run/1")
print(h.name, h.state, h.id, h.url)
print(h.update("success", description="done"))
print(h.name, h.state, h.id, h.url)
print(h, type(h))
`)
	require.NoError(t, err)
	assert.Equal(t, `build in_progress 7 https://checks/7
None
build success 7 https://checks/7/final
check(name = "build", state = "success") check
`, stdout)
}

// TestCICheckUpdateSendsTheStoredIDEachTime proves two checks that share a name stay separate: the
// update addresses the check by the id the create returned.
func TestCICheckUpdateSendsTheStoredIDEachTime(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Check(gomock.Any(), ci.CheckRequest{Name: "dup", State: ci.CheckRunStatePending}).
		Return(ci.Receipt{Check: &ci.CheckRun{ID: 1}}, nil)
	m.EXPECT().Check(gomock.Any(), ci.CheckRequest{Name: "dup", State: ci.CheckRunStatePending}).
		Return(ci.Receipt{Check: &ci.CheckRun{ID: 2}}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{Name: "dup", ID: 2, State: ci.CheckRunStateSuccess}).Return(ci.Receipt{}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{Name: "dup", ID: 1, State: ci.CheckRunStateFailure}).Return(ci.Receipt{}, nil)
	_, _, err := runCI(t, m, `a = ci.check("dup")
b = ci.check("dup")
b.update("success")
a.update("failure")`)
	require.NoError(t, err)
}

func TestCICheckUpdateReceiptWithoutIDKeepsTheStoredID(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Check(gomock.Any(), gomock.Any()).Return(ci.Receipt{Check: &ci.CheckRun{ID: 9}}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{Name: "n", ID: 9, State: ci.CheckRunStateInProgress}).Return(ci.Receipt{Check: &ci.CheckRun{}}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{Name: "n", ID: 9, State: ci.CheckRunStateSuccess}).Return(ci.Receipt{}, nil)
	stdout, _, err := runCI(t, m, `h = ci.check("n")
h.update("in_progress")
h.update("success")
print(h.id)`)
	require.NoError(t, err)
	assert.Equal(t, "9\n", stdout)
}

func TestCICheckDefaultsToPendingAndZeroIDLocally(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Check(gomock.Any(), ci.CheckRequest{Name: "lint", State: ci.CheckRunStatePending}).Return(ci.Receipt{Local: true}, nil)
	stdout, _, err := runCI(t, m, `h = ci.check("lint")
print(h.state, h.id, repr(h.url))`)
	require.NoError(t, err)
	assert.Equal(t, "pending 0 \"\"\n", stdout)
}

func TestCICheckUpdateKeywordsAndURL(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Check(gomock.Any(), ci.CheckRequest{Name: "n", State: ci.CheckRunStatePending}).Return(ci.Receipt{Local: true}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{
		Name: "n", State: ci.CheckRunStateCancelled, Description: "stopped", URL: "https://x",
	}).Return(ci.Receipt{Local: true}, nil)
	stdout, _, err := runCI(t, m, `h = ci.check("n")
h.update(state="cancelled", description="stopped", url="https://x")
print(h.state, h.url)`)
	require.NoError(t, err)
	assert.Equal(t, "cancelled https://x\n", stdout)
}

func TestCICheckUpdateEveryState(t *testing.T) {
	t.Parallel()
	for _, state := range []ci.CheckRunState{
		ci.CheckRunStatePending, ci.CheckRunStateInProgress, ci.CheckRunStateSuccess,
		ci.CheckRunStateFailure, ci.CheckRunStateError, ci.CheckRunStateCancelled,
	} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			m.EXPECT().Check(gomock.Any(), gomock.Any()).Return(ci.Receipt{}, nil)
			m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{Name: "n", State: state}).Return(ci.Receipt{}, nil)
			stdout, _, err := runCI(t, m, `h = ci.check("n")
h.update("`+string(state)+`")
print(h.state)`)
			require.NoError(t, err)
			assert.Equal(t, string(state)+"\n", stdout)
		})
	}
}

func TestCICheckUpdateRejectsBadStateWithoutCalling(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Check(gomock.Any(), gomock.Any()).Return(ci.Receipt{}, nil)
	_, _, err := runCI(t, m, `h = ci.check("n")
h.update("done")`)
	require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
	assert.ErrorContains(t, err, "state must be one of pending, in_progress, success, failure, error, cancelled")
}

func TestCICheckUpdateWrongTypeAndMissingState(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`h.update(1)`, `h.update()`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			m.EXPECT().Check(gomock.Any(), gomock.Any()).Return(ci.Receipt{}, nil)
			_, _, err := runCI(t, m, "h = ci.check(\"n\")\n"+source)
			require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
		})
	}
}

func TestCICheckUpdateErrorKeepsSentinel(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Check(gomock.Any(), gomock.Any()).Return(ci.Receipt{}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), gomock.Any()).Return(ci.Receipt{}, errors.Join(errUtils.ErrCICheckRunUpdateFailed, errors.New("api down")))
	_, _, err := runCI(t, m, `h = ci.check("n")
h.update("success")`)
	require.ErrorIs(t, err, errUtils.ErrCICheckRunUpdateFailed)
	assert.ErrorContains(t, err, "ci.check.update")
}

func TestCICheckUpdateWorksInsideFrozenParallelTasks(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Check(gomock.Any(), ci.CheckRequest{Name: "deploy", State: ci.CheckRunStatePending}).Return(ci.Receipt{}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{Name: "deploy", State: ci.CheckRunStateInProgress}).Return(ci.Receipt{}, nil)
	m.EXPECT().UpdateCheck(gomock.Any(), ci.CheckRequest{Name: "deploy", State: ci.CheckRunStateSuccess}).Return(ci.Receipt{}, nil)
	m.EXPECT().Context().Return(&ci.Context{Provider: "github-actions", Branch: "main"}, nil).Times(1)
	stdout, _, err := runCI(t, m, `
h = ci.check("deploy")
def start():
    h.update("in_progress")
def finish():
    h.update("success")
steps.parallel(tasks = [steps.task(name = "start", function = start), steps.task(name = "finish", function = finish)], max_concurrency = 1)
print(h.state, ci.context.branch)
`)
	require.NoError(t, err)
	assert.Equal(t, "success main\n", stdout)
}

func TestCIGroupRunsFunctionInsideTheGroup(t *testing.T) {
	t.Parallel()
	var events []string
	m := newCIMock(t)
	m.EXPECT().Group("Plan").DoAndReturn(func(string) (func(), ci.Receipt, error) {
		events = append(events, "start")
		return func() { events = append(events, "end") }, ci.Receipt{}, nil
	})
	m.EXPECT().Output("k", "v").DoAndReturn(func(string, string) (ci.Receipt, error) {
		events = append(events, "output")
		return ci.Receipt{}, nil
	})
	stdout, _, err := runCI(t, m, `
def body():
    ci.output("k", "v")
    return 42
print(ci.group("Plan", body))
`)
	require.NoError(t, err)
	assert.Equal(t, "42\n", stdout)
	assert.Equal(t, []string{"start", "output", "end"}, events)
}

func TestCIGroupEndsWhenFunctionFails(t *testing.T) {
	t.Parallel()
	var events []string
	m := newCIMock(t)
	m.EXPECT().Group("Apply").DoAndReturn(func(string) (func(), ci.Receipt, error) {
		events = append(events, "start")
		return func() { events = append(events, "end") }, ci.Receipt{}, nil
	})
	_, _, err := runCI(t, m, `
def body():
    fail("boom")
ci.group("Apply", body)
`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "boom")
	assert.Equal(t, []string{"start", "end"}, events)
}

func TestCIGroupStartFailureSkipsFunction(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Group("G").Return(func() {}, ci.Receipt{}, errors.New("cannot open group"))
	stdout, _, err := runCI(t, m, `
def body():
    print("ran")
ci.group("G", body)
`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "ci.group: cannot open group")
	assert.Empty(t, stdout)
}

func TestCIGroupAcceptsBuiltinsAndLambdas(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Group("L").Return(func() {}, ci.Receipt{}, nil)
	stdout, _, err := runCI(t, m, `print(ci.group("L", lambda: "result"))`)
	require.NoError(t, err)
	assert.Equal(t, "result\n", stdout)
}
