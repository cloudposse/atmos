package starlark

//go:generate go run go.uber.org/mock/mockgen -typed -destination mock_ci_reporter_test.go -package starlark github.com/cloudposse/atmos/pkg/ci Reporter

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Compile-time sentinels: a rename of these request fields must fail the build here.
var (
	_ = ci.CommentRequest{Body: "", Key: "", Behavior: ci.CommentBehaviorUpsert, PR: 0}
	_ = ci.CheckRequest{Name: "", State: ci.CheckRunStatePending, Description: "", URL: ""}
	_ = ci.Annotation{Path: "", StartLine: 0, EndLine: 0, Level: ci.AnnotationError, Title: "", Message: ""}
	_ = ci.SARIFReport{Body: nil, Category: ""}
)

var ciFormatterOnce sync.Once

// initCIFormatter initializes the UI formatter that gate warnings render through.
func initCIFormatter(t *testing.T) {
	t.Helper()
	ciFormatterOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// newCIMock returns a Reporter mock whose WithOutput returns the mock itself.
func newCIMock(t *testing.T) *MockReporter {
	t.Helper()
	initCIFormatter(t)
	m := NewMockReporter(gomock.NewController(t))
	m.EXPECT().WithOutput(gomock.Any()).Return(m).AnyTimes()
	return m
}

// runCI executes source against the mock reporter.
func runCI(t *testing.T, m *MockReporter, source string) (stdout, stderr string, err error) {
	t.Helper()
	return executeSpec(t, script.Spec{CI: m, Source: source})
}

// useGenericOnly isolates the registry so the real reporter renders through the generic provider.
func useGenericOnly(t *testing.T) {
	t.Helper()
	initCIFormatter(t)
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ATMOS_CI_OUTPUT", "")
	t.Setenv("ATMOS_CI_SUMMARY", "")
	ci.Register(generic.NewProvider())
}

func TestCIModuleMembers(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Context().Times(0)
	stdout, _, err := runCI(t, m, `print(dir(ci))
print(ci)
print(type(ci))`)
	require.NoError(t, err)
	assert.Equal(t, `["annotate", "base", "check", "comment", "context", "env", "group", "mask", "output", "path", "sarif", "summary"]
<module ci>
module
`, stdout)
}

func TestCIUnknownMemberFails(t *testing.T) {
	t.Parallel()
	_, _, err := runCI(t, newCIMock(t), `ci.nope()`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "nope")
}

func TestCIBuiltinArgumentErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ source, message string }{
		{`ci.summary(1)`, "ci.summary"},
		{`ci.summary()`, "ci.summary"},
		{`ci.output("name", 1)`, "ci.output"},
		{`ci.output("", "v")`, "name must not be empty"},
		{`ci.env("name", True)`, "ci.env"},
		{`ci.env("", "v")`, "name must not be empty"},
		{`ci.path(1)`, "ci.path"},
		{`ci.path("")`, "dir must not be empty"},
		{`ci.mask(1)`, "ci.mask"},
		{`ci.annotate("fatal", "m")`, `level must be one of error, warning, notice, got "fatal"`},
		{`ci.annotate("error", "")`, "message must not be empty"},
		{`ci.annotate("error", "m", line=-1)`, "must not be negative"},
		{`ci.annotate("error", "m", end_line=-2)`, "must not be negative"},
		{`ci.annotate("error", "m", line="3")`, "ci.annotate"},
		{`ci.annotate(1, "m")`, "ci.annotate"},
		{`ci.comment("")`, "body must not be empty"},
		{`ci.comment(1)`, "ci.comment"},
		{`ci.comment("b", behavior="merge")`, `behavior must be one of create, update, upsert, got "merge"`},
		{`ci.comment("b", pr=-1)`, "pr must not be negative"},
		{`ci.comment("b", pr="1")`, "ci.comment"},
		{`ci.check("")`, "name must not be empty"},
		{`ci.check("n", state="done")`, "state must be one of pending, in_progress, success, failure, error, cancelled"},
		{`ci.check(1)`, "ci.check"},
		{`ci.group("t", 1)`, "fn must be callable, got int"},
		{`ci.group("t")`, "ci.group"},
		{`ci.sarif("")`, "path must not be empty"},
		{`ci.sarif(1)`, "ci.sarif"},
		{`ci.base(1)`, "ci.base"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			// A mock with no write expectations fails the test if validation lets a call through.
			_, _, err := runCI(t, newCIMock(t), tc.source)
			require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
			assert.ErrorContains(t, err, tc.message)
		})
	}
}

func TestCIWritesMapToReporterCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		expect       func(m *MockReporter)
	}{
		{"summary", `ci.summary("# Plan")`, func(m *MockReporter) {
			m.EXPECT().Summary("# Plan").Return(ci.Receipt{}, nil)
		}},
		{"output", `ci.output("image", "app:1")`, func(m *MockReporter) {
			m.EXPECT().Output("image", "app:1").Return(ci.Receipt{}, nil)
		}},
		{"output keywords", `ci.output(value = "v", name = "k")`, func(m *MockReporter) {
			m.EXPECT().Output("k", "v").Return(ci.Receipt{}, nil)
		}},
		{"env", `ci.env("REGION", "us-east-1")`, func(m *MockReporter) {
			m.EXPECT().Env("REGION", "us-east-1").Return(ci.Receipt{}, nil)
		}},
		{"path", `ci.path("/opt/bin")`, func(m *MockReporter) {
			m.EXPECT().Path("/opt/bin").Return(ci.Receipt{}, nil)
		}},
		{"mask", `ci.mask("s3cr3t")`, func(m *MockReporter) {
			m.EXPECT().Mask("s3cr3t").Return(ci.Receipt{}, nil)
		}},
		{"annotate defaults", `ci.annotate("notice", "heads up")`, func(m *MockReporter) {
			m.EXPECT().Annotate(ci.Annotation{Level: ci.AnnotationNotice, Message: "heads up"}).Return(ci.Receipt{}, nil)
		}},
		{"annotate all fields", `ci.annotate("warning", "deprecated", file="main.tf", line=3, end_line=5, title="Lint")`, func(m *MockReporter) {
			m.EXPECT().Annotate(ci.Annotation{
				Path: "main.tf", StartLine: 3, EndLine: 5, Level: ci.AnnotationWarning, Title: "Lint", Message: "deprecated",
			}).Return(ci.Receipt{}, nil)
		}},
		{"annotate error level", `ci.annotate("error", "broken", file="a.tf")`, func(m *MockReporter) {
			m.EXPECT().Annotate(ci.Annotation{Path: "a.tf", Level: ci.AnnotationError, Message: "broken"}).Return(ci.Receipt{}, nil)
		}},
		{"comment defaults", `ci.comment("hi")`, func(m *MockReporter) {
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "hi", Behavior: "upsert"}).Return(ci.Receipt{}, nil)
		}},
		{"comment all fields", `ci.comment("hi", key="k", behavior="create", pr=42)`, func(m *MockReporter) {
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "hi", Key: "k", Behavior: "create", PR: 42}).Return(ci.Receipt{}, nil)
		}},
		{"comment upsert", `ci.comment("hi", key="k", behavior="upsert")`, func(m *MockReporter) {
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "hi", Key: "k", Behavior: "upsert", PR: 0}).Return(ci.Receipt{}, nil)
		}},
		{"comment update", `ci.comment("hi", key="k", behavior="update")`, func(m *MockReporter) {
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "hi", Key: "k", Behavior: "update"}).Return(ci.Receipt{}, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			tc.expect(m)
			stdout, stderr, err := runCI(t, m, tc.source)
			require.NoError(t, err)
			assert.Empty(t, stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestCIWritesReturnNone(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{}, nil)
	m.EXPECT().Output(gomock.Any(), gomock.Any()).Return(ci.Receipt{}, nil)
	stdout, _, err := runCI(t, m, `print(ci.summary("a"), ci.output("k", "v"))`)
	require.NoError(t, err)
	assert.Equal(t, "None None\n", stdout)
}

func TestCIGateWarnsWhenRenderedLocally(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		expect       func(m *MockReporter, rc ci.Receipt)
		gate         ci.Feature
		want         string
	}{
		{"comment", `ci.comment("b")`, func(m *MockReporter, rc ci.Receipt) {
			m.EXPECT().Comment(gomock.Any(), gomock.Any()).Return(rc, nil)
		}, ci.FeatureComments, "ci.comment: ci.comments.enabled is off; rendered locally"},
		{"summary", `ci.summary("b")`, func(m *MockReporter, rc ci.Receipt) {
			m.EXPECT().Summary("b").Return(rc, nil)
		}, ci.FeatureSummary, "ci.summary: ci.summary.enabled is off; rendered locally"},
		{"output", `ci.output("k", "v")`, func(m *MockReporter, rc ci.Receipt) {
			m.EXPECT().Output("k", "v").Return(rc, nil)
		}, ci.FeatureOutput, "ci.output: ci.output.enabled is off; rendered locally"},
		{"annotate", `ci.annotate("error", "m")`, func(m *MockReporter, rc ci.Receipt) {
			m.EXPECT().Annotate(gomock.Any()).Return(rc, nil)
		}, ci.FeatureAnnotations, "ci.annotate: ci.annotations.enabled is off; rendered locally"},
		{"check", `ci.check("c")`, func(m *MockReporter, rc ci.Receipt) {
			m.EXPECT().Check(gomock.Any(), gomock.Any()).Return(rc, nil)
		}, ci.FeatureChecks, "ci.check: ci.checks.enabled is off; rendered locally"},
		{"sarif", `ci.sarif("r.sarif")`, func(m *MockReporter, rc ci.Receipt) {
			m.EXPECT().SARIF(gomock.Any(), gomock.Any()).Return(rc, nil)
		}, ci.FeatureResults, "ci.sarif: ci.results.enabled is off; rendered locally"},
	} {
		t.Run(tc.name+" gated", func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			tc.expect(m, ci.Receipt{Provider: "generic", Local: true, Gate: tc.gate})
			_, stderr, err := executeSpec(t, script.Spec{CI: m, Source: tc.source}, WithReadFile(func(string) ([]byte, error) { return []byte("{}"), nil }))
			require.NoError(t, err)
			assert.Contains(t, stderr, tc.want)
		})
		t.Run(tc.name+" local without gate", func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			tc.expect(m, ci.Receipt{Provider: "generic", Local: true})
			_, stderr, err := executeSpec(t, script.Spec{CI: m, Source: tc.source}, WithReadFile(func(string) ([]byte, error) { return []byte("{}"), nil }))
			require.NoError(t, err)
			assert.Empty(t, stderr)
		})
	}
}

func TestCIGateWarnsForGroup(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().Group("T").Return(func() {}, ci.Receipt{Local: true, Gate: ci.FeatureGroups}, nil)
	_, stderr, err := runCI(t, m, `ci.group("T", lambda: None)`)
	require.NoError(t, err)
	assert.Contains(t, stderr, "ci.group: ci.groups.mode is off; rendered locally")
}

func TestCICommentReturnsStruct(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		receipt ci.Receipt
		want    string
	}{
		{"posted", ci.Receipt{Comment: &ci.Comment{ID: 99, URL: "https://x/c/99", Created: true}}, `{"id":99,"url":"https://x/c/99","created":true}`},
		{"updated", ci.Receipt{Comment: &ci.Comment{ID: 5, URL: "https://x/c/5"}}, `{"id":5,"url":"https://x/c/5","created":false}`},
		{"local preview without comment", ci.Receipt{Local: true}, `{"id":0,"url":"","created":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "b", Behavior: "upsert"}).Return(tc.receipt, nil)
			stdout, _, err := runCI(t, m, `c = ci.comment("b")
print(json.encode(c))
print(type(c))`)
			require.NoError(t, err)
			encoded, kind, found := strings.Cut(stdout, "\n")
			require.True(t, found, stdout)
			assert.JSONEq(t, tc.want, encoded)
			assert.Equal(t, "struct\n", kind)
		})
	}
}

func TestCIBase(t *testing.T) {
	t.Parallel()
	t.Run("resolved", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Base().Return(&ci.BaseResolution{Ref: "origin/main", SHA: "abc", HeadSHA: "def", TargetBranch: "main", Source: "event"}, nil)
		stdout, _, err := runCI(t, m, `print(json.encode(ci.base()))`)
		require.NoError(t, err)
		assert.JSONEq(t, `{"ref":"origin/main","sha":"abc","head_sha":"def","target_branch":"main","source":"event"}`, stdout)
	})
	t.Run("error is None", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Base().Return(nil, errors.New("no base"))
		stdout, _, err := runCI(t, m, `print(ci.base())`)
		require.NoError(t, err)
		assert.Equal(t, "None\n", stdout)
	})
	t.Run("nil resolution is None", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Base().Return(nil, nil)
		stdout, _, err := runCI(t, m, `print(ci.base())`)
		require.NoError(t, err)
		assert.Equal(t, "None\n", stdout)
	})
}

func TestCIContextIsLazyAndCached(t *testing.T) {
	t.Parallel()
	t.Run("untouched context is never requested", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Context().Times(0)
		m.EXPECT().Summary("x").Return(ci.Receipt{}, nil)
		_, _, err := runCI(t, m, `ci.summary("x")`)
		require.NoError(t, err)
	})
	t.Run("two reads resolve once", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Context().Return(&ci.Context{Provider: "github-actions", SHA: "abc"}, nil).Times(1)
		stdout, _, err := runCI(t, m, `print(ci.context.provider, ci.context.sha)`)
		require.NoError(t, err)
		assert.Equal(t, "github-actions abc\n", stdout)
	})
}

func TestCIContextFields(t *testing.T) {
	t.Parallel()
	t.Run("pull request", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Context().Return(&ci.Context{
			Provider: "github-actions", EventName: "pull_request", SHA: "abc", Branch: "feat", Repository: "o/r",
			Actor: "erik", RunID: "9", RunURL: "https://run", ElevatedEvent: true,
			PullRequest: &ci.PRInfo{Number: 12, HeadRef: "feat", BaseRef: "main", URL: "https://pr"},
		}, nil).Times(1)
		stdout, _, err := runCI(t, m, `print(json.encode(ci.context))
print(type(ci.context))`)
		require.NoError(t, err)
		encoded, kind, found := strings.Cut(stdout, "\n")
		require.True(t, found, stdout)
		assert.JSONEq(t, `{"provider":"github-actions","local":false,"event":"pull_request","sha":"abc","branch":"feat","repo":"o/r",`+
			`"actor":"erik","run_id":"9","run_url":"https://run","elevated":true,`+
			`"pr":{"number":12,"head":"feat","base":"main","url":"https://pr"}}`, encoded)
		assert.Equal(t, "struct\n", kind)
	})
	t.Run("no pull request", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Context().Return(&ci.Context{Provider: "github-actions", EventName: "push"}, nil)
		stdout, _, err := runCI(t, m, `print(ci.context.pr, ci.context.event, ci.context.elevated)`)
		require.NoError(t, err)
		assert.Equal(t, "None push False\n", stdout)
	})
	t.Run("generic provider is local", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Context().Return(&ci.Context{Provider: generic.ProviderName}, nil)
		stdout, _, err := runCI(t, m, `print(ci.context.local)`)
		require.NoError(t, err)
		assert.Equal(t, "True\n", stdout)
	})
	t.Run("context is immutable", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Context().Return(&ci.Context{Provider: "p"}, nil)
		_, _, err := runCI(t, m, `ci.context.provider = "other"`)
		require.Error(t, err)
	})
	t.Run("context error is None", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Context().Return(nil, errUtils.ErrCIProviderNotDetected).Times(1)
		stdout, _, err := runCI(t, m, `print(ci.context)
print(ci.context)`)
		require.NoError(t, err)
		assert.Equal(t, "None\nNone\n", stdout)
	})
}

func TestCISARIFReadsThroughEngineFileReader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var read []string
	reader := WithReadFile(func(path string) ([]byte, error) {
		read = append(read, path)
		return []byte(`{"runs":[]}`), nil
	})
	m := newCIMock(t)
	m.EXPECT().SARIF(gomock.Any(), ci.SARIFReport{Body: []byte(`{"runs":[]}`), Category: "tf"}).Return(ci.Receipt{}, nil)
	m.EXPECT().SARIF(gomock.Any(), ci.SARIFReport{Body: []byte(`{"runs":[]}`), Category: ""}).Return(ci.Receipt{}, nil)
	absolute := filepath.Join(dir, "abs", "r.sarif")
	_, _, err := executeSpec(t, script.Spec{CI: m, WorkingDirectory: dir, Source: `ci.sarif("out/r.sarif", category="tf")
ci.sarif(` + `"` + strings.ReplaceAll(absolute, `\`, `\\`) + `"` + `)`}, reader)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "out", "r.sarif"), absolute}, read)
}

func TestCISARIFReadFailure(t *testing.T) {
	t.Parallel()
	_, _, err := executeSpec(t, script.Spec{CI: newCIMock(t), Source: `ci.sarif("missing.sarif")`},
		WithReadFile(func(string) ([]byte, error) { return nil, os.ErrNotExist }))
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.ErrorContains(t, err, "ci.sarif: cannot read file")
}

func TestCIReporterErrorsKeepTheirSentinel(t *testing.T) {
	t.Parallel()
	t.Run("pull request unknown carries a hint and traceback", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "b", Behavior: "upsert"}).
			Return(ci.Receipt{}, errUtils.Build(errUtils.ErrCIPullRequestUnknown).Err())
		_, _, err := runCI(t, m, `def post():
    ci.comment("b")
post()`)
		require.ErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
		require.ErrorIs(t, err, errUtils.ErrStarlark)
		assert.ErrorContains(t, err, "ci.comment")
		hints := cockroach.GetAllHints(err)
		require.NotEmpty(t, hints)
		assert.Contains(t, strings.Join(hints, "\n"), "pr=123")
		details := cockroach.GetAllDetails(err)
		require.NotEmpty(t, details)
		assert.Contains(t, details[0], "in post")
	})
	for _, tc := range []struct {
		name, source string
		expect       func(m *MockReporter, err error)
		sentinel     error
	}{
		{"summary", `ci.summary("x")`, func(m *MockReporter, err error) { m.EXPECT().Summary("x").Return(ci.Receipt{}, err) }, errUtils.ErrCISummaryWriteFailed},
		{"output", `ci.output("k", "v")`, func(m *MockReporter, err error) { m.EXPECT().Output("k", "v").Return(ci.Receipt{}, err) }, errUtils.ErrCIOutputWriteFailed},
		{"check", `ci.check("c")`, func(m *MockReporter, err error) {
			m.EXPECT().Check(gomock.Any(), gomock.Any()).Return(ci.Receipt{}, err)
		}, errUtils.ErrCICheckRunCreateFailed},
		{"annotate", `ci.annotate("error", "m")`, func(m *MockReporter, err error) { m.EXPECT().Annotate(gomock.Any()).Return(ci.Receipt{}, err) }, errUtils.ErrCIAnnotationFailed},
	} {
		t.Run(tc.name+" sentinel", func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			tc.expect(m, errors.Join(tc.sentinel, errors.New("provider said no")))
			_, _, err := runCI(t, m, tc.source)
			require.ErrorIs(t, err, tc.sentinel)
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.ErrorContains(t, err, "provider said no")
		})
	}
	t.Run("unclassified error is a script error", func(t *testing.T) {
		t.Parallel()
		m := newCIMock(t)
		m.EXPECT().Mask("v").Return(ci.Receipt{}, errors.New("boom"))
		_, _, err := runCI(t, m, `ci.mask("v")`)
		require.ErrorIs(t, err, errUtils.ErrStarlark)
		assert.ErrorContains(t, err, "ci.mask: boom")
		assert.NotErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
	})
}

func TestCIParallelTasksPrefixOutputAndKeepWorking(t *testing.T) {
	t.Parallel()
	m := NewMockReporter(gomock.NewController(t))
	initCIFormatter(t)
	m.EXPECT().WithOutput(gomock.Any()).DoAndReturn(func(w io.Writer) ci.Reporter {
		_, _ = io.WriteString(w, "local render\n")
		return m
	}).AnyTimes()
	m.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{Local: true, Gate: ci.FeatureSummary}, nil).Times(2)
	m.EXPECT().Context().Return(&ci.Context{Provider: "github-actions", SHA: "abc"}, nil).Times(1)
	_, stderr, err := runCI(t, m, `
def work(name):
    ci.summary(name)
steps.parallel(tasks = [steps.task(name = n, function = work, args = [n]) for n in ["a", "b"]], max_concurrency = 1)
print(ci.context.sha)
`)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	require.Len(t, lines, 4, stderr)
	warning := regexp.MustCompile(`^\[(a|b)\] .*ci\.summary: ci\.summary\.enabled is off; rendered locally$`)
	assert.Equal(t, "[a] local render", lines[0])
	assert.Regexp(t, warning, lines[1])
	assert.Equal(t, "[b] local render", lines[2])
	assert.Regexp(t, warning, lines[3])
	assert.True(t, strings.HasPrefix(lines[1], "[a]") && strings.HasPrefix(lines[3], "[b]"), stderr)
}

func TestCIMainThreadOutputIsUnprefixed(t *testing.T) {
	t.Parallel()
	m := NewMockReporter(gomock.NewController(t))
	initCIFormatter(t)
	m.EXPECT().WithOutput(gomock.Any()).DoAndReturn(func(w io.Writer) ci.Reporter {
		_, _ = io.WriteString(w, "local render\n")
		return m
	})
	m.EXPECT().Summary("x").Return(ci.Receipt{}, nil)
	_, stderr, err := runCI(t, m, `ci.summary("x")`)
	require.NoError(t, err)
	assert.Equal(t, "local render\n", stderr)
}

func TestCIWithRealReporterRunsLocallyWhenNoReporterIsSupplied(t *testing.T) {
	useGenericOnly(t)
	stdout, _, err := executeSpec(t, script.Spec{Source: `print(ci.context.provider, ci.context.local)`})
	require.NoError(t, err)
	assert.Equal(t, "generic True\n", stdout)
}

func TestCIRendersLocallyThroughGenericProvider(t *testing.T) {
	useGenericOnly(t)
	stdout, stderr, err := executeSpec(t, script.Spec{Source: `ci.summary("# Hi there")
c = ci.comment("comment body", key="k")
ci.output("a", "b")
print(c.id, c.created, repr(c.url))
`})
	require.NoError(t, err)
	assert.Equal(t, "1 True \"\"\n", stdout)
	for _, want := range []string{"Hi there", "PR comment preview (upsert)", "comment body", "a=b"} {
		assert.Contains(t, stderr, want)
		assert.NotContains(t, stdout, want)
	}
	assert.NotContains(t, stderr, "is off; rendered locally")
}
