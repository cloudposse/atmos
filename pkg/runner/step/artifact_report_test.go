package step

//go:generate go run go.uber.org/mock/mockgen -typed -destination mock_ci_reporter_test.go -package step github.com/cloudposse/atmos/pkg/ci Reporter

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	star "github.com/cloudposse/atmos/pkg/script/starlark"
)

// summaryRecorder captures what the report session writes through a generated ci.Reporter mock. The
// mock's Summary and Output callbacks record the assertions' state, and any other reporter call fails
// loudly as an unexpected call.
type summaryRecorder struct {
	reporter *MockReporter

	mu        sync.Mutex
	summaries []string
	outputs   map[string]string
	builds    int
}

func newSummaryRecorder(t *testing.T) *summaryRecorder {
	t.Helper()
	r := &summaryRecorder{reporter: NewMockReporter(gomock.NewController(t))}
	r.reporter.EXPECT().Summary(gomock.Any()).DoAndReturn(func(markdown string) (ci.Receipt, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.summaries = append(r.summaries, markdown)
		return ci.Receipt{}, nil
	}).AnyTimes()
	r.reporter.EXPECT().Output(gomock.Any(), gomock.Any()).DoAndReturn(func(key, value string) (ci.Receipt, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.outputs == nil {
			r.outputs = map[string]string{}
		}
		r.outputs[key] = value
		return ci.Receipt{}, nil
	}).AnyTimes()
	return r
}

func (r *summaryRecorder) option() report.Option {
	return report.WithReporter(func(*schema.AtmosConfiguration) ci.Reporter {
		r.builds++
		return r.reporter
	})
}

func ciEnabled() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
}

// publishStep returns a publish step whose input differs by version, so each is its own operation.
func publishStep(name, version string) *schema.WorkflowStep {
	step := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository, invoke.FieldVersion)
	step.Name, step.Version = name, version
	return step
}

// expectPublishes makes the service answer every publish with a created result for its version.
func expectPublishes(env *artifactEnv) {
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		op := completeOperation(artifact.ActionPublish, artifact.OutcomeCreated)
		op.OperationID, op.Ref = r.Execution.OperationID, "team/app:"+r.Version
		return op, nil
	}).AnyTimes()
}

var reportedRefs = regexp.MustCompile("(?m)^### \\S+ \\d+\\. Publish `([^`]+)`$")

func refsOf(markdown string) []string {
	var refs []string
	for _, match := range reportedRefs.FindAllStringSubmatch(markdown, -1) {
		refs = append(refs, match[1])
	}
	return refs
}

func TestArtifactReport_SequentialStepsProduceOneOrderedReport(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())

	for _, version := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		_, err := env.run(publishStep("publish-"+version, version))
		require.NoError(t, err)
	}
	assert.Empty(t, recorder.summaries, "nothing is written while steps run")
	flush()

	require.Len(t, recorder.summaries, 1, "exactly one report for the whole execution")
	assert.Equal(t, []string{"team/app:1.0.0", "team/app:2.0.0", "team/app:3.0.0"}, refsOf(recorder.summaries[0]))
	assert.Equal(t, "team/app:3.0.0", recorder.outputs["artifact_ref"])
	assert.Equal(t, "team/app:1.0.0", recorder.outputs["artifact_publish_1_0_0_ref"])
}

func TestArtifactReport_RetriesReportTheFinalAttemptOnce(t *testing.T) {
	env := newArtifactEnv(t)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())

	calls := 0
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		calls++
		if calls < 3 {
			return &artifact.OperationResult{OperationID: r.Execution.OperationID, Action: artifact.ActionPublish}, errors.New("temporary registry failure")
		}
		op := completeOperation(artifact.ActionPublish, artifact.OutcomeCreated)
		op.OperationID = r.Execution.OperationID
		return op, nil
	}).Times(3)

	step := publishStep("publish", "1.0.0")
	step.Retry = &schema.RetryConfig{}
	for attempt := 1; attempt <= 3; attempt++ {
		_, _ = env.run(step)
	}
	flush()

	require.Len(t, recorder.summaries, 1)
	assert.Contains(t, recorder.summaries[0], "1 operation: 1 succeeded.")
	assert.Contains(t, recorder.summaries[0], "**Attempts:** 3")
	assert.NotContains(t, recorder.summaries[0], "temporary registry failure", "only the final attempt is reported")
}

func TestArtifactReport_ParallelBranchesFollowTheSchedule(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())

	_, err := env.run(publishStep("before", "0.0.0"))
	require.NoError(t, err)

	// Four branches are scheduled together and finish in the reverse of their scheduled order.
	var wg sync.WaitGroup
	release := make([]chan struct{}, 4)
	finished := make([]chan struct{}, 4)
	for index := range release {
		release[index], finished[index] = make(chan struct{}), make(chan struct{})
	}
	group := env.vars.ScheduleGroup()
	for index := range release {
		wg.Add(1)
		branch := group.ForkAt(index)
		go func() {
			defer wg.Done()
			defer close(finished[index])
			<-release[index]
			step := publishStep(fmt.Sprintf("branch-%d", index), fmt.Sprintf("1.%d.0", index))
			step.WorkingDirectory = env.dir
			_, runErr := (&ArtifactHandler{}).Execute(context.Background(), step, branch)
			assert.NoError(t, runErr)
		}()
	}
	for index := len(release) - 1; index >= 0; index-- {
		close(release[index])
		<-finished[index]
	}
	wg.Wait()

	_, err = env.run(publishStep("after", "9.0.0"))
	require.NoError(t, err)
	flush()

	require.Len(t, recorder.summaries, 1)
	assert.Equal(t, []string{
		"team/app:0.0.0", "team/app:1.0.0", "team/app:1.1.0", "team/app:1.2.0", "team/app:1.3.0", "team/app:9.0.0",
	}, refsOf(recorder.summaries[0]), "the group sits between the operation before it and the one after it, in branch order")
}

func TestArtifactReport_NestedBranchesNestUnderTheirGroup(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())

	group := env.vars.ScheduleGroup()
	outer := group.ForkAt(1)
	innerGroup := outer.ScheduleGroup()
	inner := innerGroup.ForkAt(0)
	innerAfter := innerGroup.ForkAt(1)
	for _, run := range []struct {
		vars    *Variables
		version string
	}{{innerAfter, "2.1.0"}, {inner, "2.0.0"}, {group.ForkAt(0), "1.0.0"}, {group.ForkAt(2), "3.0.0"}} {
		step := publishStep("n-"+run.version, run.version)
		step.WorkingDirectory = env.dir
		_, err := (&ArtifactHandler{}).Execute(context.Background(), step, run.vars)
		require.NoError(t, err)
	}
	flush()

	assert.Equal(t, []string{"team/app:1.0.0", "team/app:2.0.0", "team/app:2.1.0", "team/app:3.0.0"}, refsOf(recorder.summaries[0]))
}

func TestArtifactReport_FailureAndCancellationAreReportedByTheDeferredFlush(t *testing.T) {
	env := newArtifactEnv(t)
	recorder := newSummaryRecorder(t)
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(nil, errors.New("registry unavailable")).Times(2)

	execute := func() {
		defer env.vars.UseArtifactReport(ciEnabled(), recorder.option())()
		_, err := env.run(publishStep("fails", "1.0.0"))
		require.Error(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		step := publishStep("canceled", "2.0.0")
		step.WorkingDirectory = env.dir
		_, err = (&ArtifactHandler{}).Execute(ctx, step, env.vars)
		require.Error(t, err)
	}
	execute()

	require.Len(t, recorder.summaries, 1, "the report is written even though the run failed")
	assert.Contains(t, recorder.summaries[0], "2 operations: 1 failed, 1 canceled.")
	assert.Contains(t, recorder.summaries[0], "registry unavailable")
}

func TestArtifactReport_DisabledReportingInstallsNothing(t *testing.T) {
	cases := map[string]*schema.AtmosConfiguration{
		"nil config":  nil,
		"ci disabled": {},
		"both gates off": {CI: schema.CIConfig{
			Enabled: true,
			Summary: schema.CISummaryConfig{Enabled: ptrTo(false)},
			Output:  schema.CIOutputConfig{Enabled: ptrTo(false)},
		}},
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			env := newArtifactEnv(t)
			expectPublishes(env)
			recorder := newSummaryRecorder(t)
			env.vars.SetArtifactCollector(nil)
			flush := env.vars.UseArtifactReport(config, recorder.option())
			_, err := env.run(publishStep("publish", "1.0.0"))
			require.NoError(t, err)
			flush()
			assert.Zero(t, recorder.builds, "no reporter is built when every surface is disabled")
			assert.Empty(t, recorder.summaries)
			assert.IsType(t, report.Discard{}, env.vars.artifactSession().collector)
		})
	}
}

func ptrTo[T any](value T) *T { return &value }

func TestArtifactReport_NestedBoundaryReusesTheOwnersSession(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	outer := env.vars.UseArtifactReport(ciEnabled(), recorder.option())
	inner := env.vars.UseArtifactReport(ciEnabled(), recorder.option())

	_, err := env.run(publishStep("publish", "1.0.0"))
	require.NoError(t, err)
	inner()
	assert.Empty(t, recorder.summaries, "a nested boundary does not write a second copy")
	outer()
	require.Len(t, recorder.summaries, 1)
	outer()
	assert.Len(t, recorder.summaries, 1, "flushing twice never repeats the report")
}

func TestArtifactReport_ConcurrentRecordingIsRaceFree(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())
	var wg sync.WaitGroup
	group := env.vars.ScheduleGroup()
	for index := 0; index < 8; index++ {
		wg.Add(1)
		branch := group.ForkAt(index)
		go func() {
			defer wg.Done()
			step := publishStep(fmt.Sprintf("b%d", index), fmt.Sprintf("1.%d.0", index))
			step.WorkingDirectory = env.dir
			_, err := (&ArtifactHandler{}).Execute(context.Background(), step, branch)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	flush()
	require.Len(t, recorder.summaries, 1)
	want := make([]string, 8)
	for index := range want {
		want[index] = fmt.Sprintf("team/app:1.%d.0", index)
	}
	assert.Equal(t, want, refsOf(recorder.summaries[0]))
}

func TestArtifactReport_FlushArtifactReportWritesTheInstalledSession(t *testing.T) {
	t.Run("writes once before a process replacement and the deferred flush adds nothing", func(t *testing.T) {
		env := newArtifactEnv(t)
		expectPublishes(env)
		recorder := newSummaryRecorder(t)
		flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())
		_, err := env.run(publishStep("publish", "1.0.0"))
		require.NoError(t, err)

		env.vars.FlushArtifactReport()
		require.Len(t, recorder.summaries, 1, "the report is written before the process is replaced")
		assert.Equal(t, []string{"team/app:1.0.0"}, refsOf(recorder.summaries[0]))
		assert.Equal(t, "team/app:1.0.0", recorder.outputs["artifact_ref"])

		flush()
		assert.Len(t, recorder.summaries, 1, "the deferred flush never repeats the report")
	})

	t.Run("covers a session that a nested boundary reuses", func(t *testing.T) {
		env := newArtifactEnv(t)
		expectPublishes(env)
		recorder := newSummaryRecorder(t)
		_ = env.vars.UseArtifactReport(ciEnabled(), recorder.option())
		nested := env.vars.UseArtifactReport(ciEnabled(), recorder.option())
		_, err := env.run(publishStep("publish", "1.0.0"))
		require.NoError(t, err)

		nested()
		assert.Empty(t, recorder.summaries, "the nested closure writes nothing")
		env.vars.FlushArtifactReport()
		assert.Len(t, recorder.summaries, 1, "the helper flushes the installed session whichever boundary owns it")
	})

	t.Run("does nothing when no report session is installed", func(t *testing.T) {
		env := newArtifactEnv(t)
		expectPublishes(env)
		recorder := newSummaryRecorder(t)
		_, err := env.run(publishStep("publish", "1.0.0"))
		require.NoError(t, err)

		assert.NotPanics(t, env.vars.FlushArtifactReport)
		assert.Zero(t, recorder.builds, "no reporter is built when no report session is installed")
		assert.Empty(t, recorder.summaries)
	})
}

func TestArtifactReport_ConsecutiveGroupsKeepTheirScheduledOrder(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())

	// Two groups start with no artifact operation between them; the second group's branches finish first.
	first, second := env.vars.ScheduleGroup(), env.vars.ScheduleGroup()
	for _, run := range []struct {
		vars    *Variables
		version string
	}{{second.ForkAt(0), "2.0.0"}, {second.ForkAt(1), "2.1.0"}, {first.ForkAt(1), "1.1.0"}, {first.ForkAt(0), "1.0.0"}} {
		step := publishStep("g-"+run.version, run.version)
		step.WorkingDirectory = env.dir
		_, err := (&ArtifactHandler{}).Execute(context.Background(), step, run.vars)
		require.NoError(t, err)
	}
	flush()

	assert.Equal(t, []string{"team/app:1.0.0", "team/app:1.1.0", "team/app:2.0.0", "team/app:2.1.0"}, refsOf(recorder.summaries[0]),
		"every branch of the first group precedes every branch of the second")
}

func TestArtifactReport_IdenticalStepsInSeparateBranchesAreSeparateOperations(t *testing.T) {
	env := newArtifactEnv(t)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())

	// Both branches are inside the same operation at once: the service holds each call until the
	// other has started, so neither operation has finished when the other begins.
	var started sync.WaitGroup
	started.Add(2)
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		started.Done()
		started.Wait()
		op := completeOperation(artifact.ActionPublish, artifact.OutcomeCreated)
		op.OperationID, op.Ref = r.Execution.OperationID, "team/app:"+r.Version
		return op, nil
	}).Times(2)

	group := env.vars.ScheduleGroup()
	step := publishStep("publish", "1.0.0")
	step.WorkingDirectory = env.dir
	var wg sync.WaitGroup
	for index := range 2 {
		wg.Add(1)
		branch := group.ForkAt(index)
		go func() {
			defer wg.Done()
			_, err := (&ArtifactHandler{}).Execute(context.Background(), step, branch)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	flush()

	require.Len(t, recorder.summaries, 1)
	assert.Contains(t, recorder.summaries[0], "2 operations: 2 succeeded.", "each branch's result is reported")
	assert.NotContains(t, recorder.summaries[0], "**Attempts:** 2", "no branch inherits the other's attempt counter")
}

func TestArtifactReport_RetriesInOneBranchStayOneOperation(t *testing.T) {
	env := newArtifactEnv(t)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())
	calls := 0
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		calls++
		if calls == 1 {
			return &artifact.OperationResult{OperationID: r.Execution.OperationID, Action: artifact.ActionPublish}, errors.New("temporary registry failure")
		}
		op := completeOperation(artifact.ActionPublish, artifact.OutcomeCreated)
		op.OperationID = r.Execution.OperationID
		return op, nil
	}).Times(2)

	group := env.vars.ScheduleGroup()
	step := publishStep("publish", "1.0.0")
	step.WorkingDirectory = env.dir
	step.Retry = &schema.RetryConfig{}
	// Each attempt of a retrying branch forks again at the same index.
	_, err := (&ArtifactHandler{}).Execute(context.Background(), step, group.ForkAt(0))
	require.Error(t, err)
	_, err = (&ArtifactHandler{}).Execute(context.Background(), step, group.ForkAt(0))
	require.NoError(t, err)
	flush()

	assert.Contains(t, recorder.summaries[0], "1 operation: 1 succeeded.")
	assert.Contains(t, recorder.summaries[0], "**Attempts:** 2")
}

func TestArtifactReport_ExecStepWritesTheReportBeforeReplacingTheProcess(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())
	_, err := env.run(publishStep("publish", "1.0.0"))
	require.NoError(t, err)

	// A working directory that does not exist makes the replacement fail after the flush on every
	// platform, so the handler returns instead of replacing the test process.
	step := &schema.WorkflowStep{Name: "replace", Type: schema.TaskTypeExec, Command: "echo replaced", WorkingDirectory: filepath.Join(env.dir, "missing")}
	_, err = (&ExecHandler{}).Execute(context.Background(), step, env.vars)
	require.Error(t, err)

	require.Len(t, recorder.summaries, 1, "the report is written before the process replacement is attempted")
	assert.Equal(t, []string{"team/app:1.0.0"}, refsOf(recorder.summaries[0]))
	flush()
	assert.Len(t, recorder.summaries, 1, "the deferred flush adds nothing after the early write")
}

func TestArtifactReport_StarlarkParallelFollowsSchedule(t *testing.T) {
	env := newArtifactEnv(t)
	expectPublishes(env)
	recorder := newSummaryRecorder(t)
	flush := env.vars.UseArtifactReport(ciEnabled(), recorder.option())
	_, err := star.New().Execute(t.Context(), script.Spec{Source: `
def publish(version):
    steps.artifact(action="publish", source="dist", repository="dir", artifact_name="team/app", version=version)
steps.parallel(tasks=[steps.task(name=v, function=publish, args=[v]) for v in ["1.0.0", "2.0.0", "3.0.0"]])
`, WorkingDirectory: env.dir, Steps: NewAutomationLibrary(env.vars, nil)})
	require.NoError(t, err)
	flush()
	require.Len(t, recorder.summaries, 1)
	assert.Equal(t, []string{"team/app:1.0.0", "team/app:2.0.0", "team/app:3.0.0"}, refsOf(recorder.summaries[0]))
}

func TestArtifactReport_ScheduledIdenticalFailuresKeepSeparateIdentities(t *testing.T) {
	env := newArtifactEnv(t)
	var ids []string
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		ids = append(ids, r.Execution.OperationID)
		return &artifact.OperationResult{OperationID: r.Execution.OperationID, Action: artifact.ActionPublish}, errors.New("failed")
	}).Times(4)
	step := publishStep("", "1.0.0")
	step.Retry = &schema.RetryConfig{}
	for range 2 {
		restore := env.vars.BeginArtifactStep()
		for range 2 {
			_, err := env.run(step)
			require.Error(t, err)
		}
		restore()
	}
	require.Len(t, ids, 4)
	assert.Equal(t, ids[0], ids[1])
	assert.Equal(t, ids[2], ids[3])
	assert.NotEqual(t, ids[0], ids[2])
}
