package starlark

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
)

func TestCISummaryTemplates(t *testing.T) {
	t.Parallel()
	nested := map[string]any{"stack": "dev", "items": []any{"a", "b"}, "counts": map[string]any{"add": int64(2)}}
	for _, tc := range []struct {
		name, source string
		expect       func(m *MockReporter)
	}{
		{"explicit template", `ci.summary(template="plan.md", data={"n": 3})`, func(m *MockReporter) {
			m.EXPECT().RenderSummary("plan.md", map[string]any{"n": int64(3)}).Return("rendered", nil)
			m.EXPECT().Summary("rendered").Return(ci.Receipt{}, nil)
		}},
		{"explicit template without data", `ci.summary(template="plan.md")`, func(m *MockReporter) {
			m.EXPECT().RenderSummary("plan.md", nil).Return("rendered", nil)
			m.EXPECT().Summary("rendered").Return(ci.Receipt{}, nil)
		}},
		{"data is None", `ci.summary(template="plan.md", data=None)`, func(m *MockReporter) {
			m.EXPECT().RenderSummary("plan.md", nil).Return("rendered", nil)
			m.EXPECT().Summary("rendered").Return(ci.Receipt{}, nil)
		}},
		{"template is None", `ci.summary("# Plan", template=None)`, func(m *MockReporter) {
			m.EXPECT().Summary("# Plan").Return(ci.Receipt{}, nil)
		}},
		{"nested data converts to plain Go values", `ci.summary(template="t", data={"stack": "dev", "items": ["a", "b"], "counts": {"add": 2}})`, func(m *MockReporter) {
			m.EXPECT().RenderSummary("t", nested).Return("rendered", nil)
			m.EXPECT().Summary("rendered").Return(ci.Receipt{}, nil)
		}},
		{"positional markdown is unchanged", `ci.summary("# Plan")`, func(m *MockReporter) {
			m.EXPECT().Summary("# Plan").Return(ci.Receipt{}, nil)
		}},
		{"markdown keyword is unchanged", `ci.summary(markdown="# Plan")`, func(m *MockReporter) {
			m.EXPECT().Summary("# Plan").Return(ci.Receipt{}, nil)
		}},
		{"empty markdown is still written", `ci.summary("")`, func(m *MockReporter) {
			m.EXPECT().Summary("").Return(ci.Receipt{}, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			tc.expect(m)
			_, _, err := runCI(t, m, tc.source)
			require.NoError(t, err)
		})
	}
}

func TestCICommentTemplates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		expect       func(m *MockReporter)
	}{
		{"explicit template forwards key", `ci.comment(template="pr.md", data={"n": 3}, key="plan")`, func(m *MockReporter) {
			m.EXPECT().RenderComment("pr.md", map[string]any{"n": int64(3)}).Return("rendered", nil)
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "rendered", Key: "plan", Behavior: "upsert", Target: ci.CommentTargetAuto}).Return(ci.Receipt{}, nil)
		}},
		{"template with create behavior and explicit pr", `ci.comment(template="pr.md", data={"n": 1}, key="k", behavior="create", pr=7)`, func(m *MockReporter) {
			m.EXPECT().RenderComment("pr.md", map[string]any{"n": int64(1)}).Return("from template", nil)
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "from template", Key: "k", Behavior: "create", PR: 7, Target: ci.CommentTargetAuto}).Return(ci.Receipt{}, nil)
		}},
		{"positional body is unchanged", `ci.comment("hi", key="k")`, func(m *MockReporter) {
			m.EXPECT().Comment(gomock.Any(), ci.CommentRequest{Body: "hi", Key: "k", Behavior: "upsert", Target: ci.CommentTargetAuto}).Return(ci.Receipt{}, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newCIMock(t)
			tc.expect(m)
			_, _, err := runCI(t, m, tc.source)
			require.NoError(t, err)
		})
	}
}

func TestCITemplateArgumentErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, message string }{
		{"summary markdown and template", `ci.summary("x", template="plan.md")`, "ci.summary: markdown and template are mutually exclusive"},
		{"summary markdown and data", `ci.summary("x", data={"n": 1})`, "ci.summary: markdown and data are mutually exclusive"},
		{"summary data must be a dict", `ci.summary(template="plan.md", data=[1])`, "ci.summary: for parameter data: got list, want dict"},
		{"summary data holds unsupported value", `ci.summary(template="plan.md", data={"f": ci.summary})`, "ci.summary: data:"},
		{"summary nothing to write", `ci.summary()`, "ci.summary: markdown is required unless template= is given"},
		{"summary empty markdown and template", `ci.summary("", template="plan.md")`, "ci.summary: markdown and template are mutually exclusive"},
		{"summary empty markdown and data", `ci.summary("", data={"n": 1})`, "ci.summary: markdown and data are mutually exclusive"},
		{"summary empty template", `ci.summary(template="")`, "ci.summary: template must not be empty"},
		{"summary template must be a string", `ci.summary(template=1)`, "ci.summary: for parameter template: got int, want string"},
		{"summary data without template", `ci.summary(data={"n": 1})`, "ci.summary: data requires template="},
		{"summary markdown must be a string", `ci.summary(1, template="plan.md")`, "ci.summary: for parameter markdown: got int, want string"},
		{"comment body and template", `ci.comment("x", template="pr.md")`, "ci.comment: body and template are mutually exclusive"},
		{"comment body and data", `ci.comment("x", data={"n": 1})`, "ci.comment: body and data are mutually exclusive"},
		{"comment data must be a dict", `ci.comment(template="pr.md", data="x")`, "ci.comment: for parameter data: got string, want dict"},
		{"comment nothing to write", `ci.comment(key="k")`, "ci.comment: body is required unless template= is given"},
		{"comment empty body and template", `ci.comment("", template="pr.md")`, "ci.comment: body and template are mutually exclusive"},
		{"comment data without template", `ci.comment(data={"n": 1}, key="k")`, "ci.comment: data requires template="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// A mock with no expectations fails the test if validation lets a render or write through.
			_, _, err := runCI(t, newCIMock(t), tc.source)
			require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
			assert.ErrorContains(t, err, tc.message)
		})
	}
}

func TestCITemplateRenderErrorsPropagate(t *testing.T) {
	t.Parallel()
	renderErr := fmt.Errorf("%w: boom", errUtils.ErrTemplateEvaluation)
	for _, tc := range []struct {
		name, source, message string
		wantErr               error
		expect                func(m *MockReporter)
	}{
		{"summary render failure", `ci.summary(template="plan.md")`, "ci.summary:", errUtils.ErrTemplateEvaluation, func(m *MockReporter) {
			m.EXPECT().RenderSummary("plan.md", nil).Return("", renderErr)
		}},
		{"summary explicit missing template", `ci.summary(template="absent.md")`, "ci.summary:", errUtils.ErrCITemplateNotFound, func(m *MockReporter) {
			m.EXPECT().RenderSummary("absent.md", nil).Return("", errUtils.ErrCITemplateNotFound)
		}},
		{"comment render failure", `ci.comment(template="pr.md", key="k")`, "ci.comment:", errUtils.ErrTemplateEvaluation, func(m *MockReporter) {
			m.EXPECT().RenderComment("pr.md", nil).Return("", renderErr)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// No Summary or Comment expectation: a failed render must not write anything.
			m := newCIMock(t)
			tc.expect(m)
			_, _, err := runCI(t, m, tc.source)
			require.ErrorIs(t, err, tc.wantErr)
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.NotErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
			assert.ErrorContains(t, err, tc.message)
		})
	}
}

func TestCICommentRenderedEmptyBodyIsRejected(t *testing.T) {
	t.Parallel()
	m := newCIMock(t)
	m.EXPECT().RenderComment("pr.md", nil).Return("", nil)
	_, _, err := runCI(t, m, `ci.comment(template="pr.md")`)
	require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
	assert.ErrorContains(t, err, "body must not be empty")
}
