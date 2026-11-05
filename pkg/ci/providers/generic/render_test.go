package generic

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

var renderInitOnce sync.Once

// newBoundProvider returns a provider rendering into the returned buffer with an initialised formatter.
func newBoundProvider(t *testing.T) (*Provider, *bytes.Buffer) {
	t.Helper()
	// CI runners advertise color support; the assertions compare plain text.
	t.Setenv("NO_COLOR", "1")
	renderInitOnce.Do(func() {
		ioCtx, err := atmosio.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
	var buf bytes.Buffer
	p, ok := NewProvider().BindOutput(&buf).(*Provider)
	require.True(t, ok)
	return p, &buf
}

func TestBindOutput_SharesCounters(t *testing.T) {
	base := NewProvider()
	var a, b bytes.Buffer
	pa := base.BindOutput(&a)
	pb := base.BindOutput(&b)

	ctx := context.Background()
	opts := &provider.CreateCheckRunOptions{Name: "n", Status: provider.CheckRunStatePending}
	var ids []int64
	for _, p := range []provider.Provider{pa, pb, base, pa} {
		cr, err := p.CreateCheckRun(ctx, opts)
		require.NoError(t, err)
		ids = append(ids, cr.ID)
	}
	assert.Equal(t, []int64{1, 2, 3, 4}, ids)

	comment := &provider.PostCommentOptions{Body: "x"}
	c1, err := pa.PostComment(ctx, comment)
	require.NoError(t, err)
	c2, err := pb.PostComment(ctx, comment)
	require.NoError(t, err)
	assert.Equal(t, int64(1), c1.ID)
	assert.Equal(t, int64(2), c2.ID)
}

func TestPostComment(t *testing.T) {
	ctx := context.Background()

	t.Run("renders preview with PR number", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		c, err := p.PostComment(ctx, &provider.PostCommentOptions{
			Owner: "o", Repo: "r", PRNumber: 7, Body: "hello preview body", Behavior: provider.CommentBehaviorCreate,
		})
		require.NoError(t, err)
		assert.True(t, c.Created)
		assert.Empty(t, c.URL)
		assert.Equal(t, "hello preview body", c.Body)
		assert.Contains(t, buf.String(), "PR comment preview (create, PR #7)")
		assert.Contains(t, buf.String(), "preview")
	})

	t.Run("render is raw when unbound and markdown when bound", func(t *testing.T) {
		bound, _ := newBoundProvider(t)
		const body = "## Raw **md**"

		var rawBuf bytes.Buffer
		NewProvider().render(ui.New(&rawBuf), body)
		assert.Equal(t, body+"\n", rawBuf.String())

		var mdBuf bytes.Buffer
		bound.render(ui.New(&mdBuf), body)
		assert.Contains(t, mdBuf.String(), "Raw")
		assert.NotEqual(t, rawBuf.String(), mdBuf.String())
	})

	t.Run("no PR path renders and defaults to upsert", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		c, err := p.PostComment(ctx, &provider.PostCommentOptions{Body: "local body"})
		require.NoError(t, err)
		assert.True(t, c.Created)
		assert.Contains(t, buf.String(), "PR comment preview (upsert)")
		assert.NotContains(t, buf.String(), "PR #")
		assert.Contains(t, buf.String(), "local")
	})

	t.Run("masks registered secrets", func(t *testing.T) {
		atmosio.Reset()
		t.Cleanup(atmosio.Reset)
		const secret = "generic-comment-secret-ABCD1234"
		atmosio.RegisterSecret(secret)

		p, buf := newBoundProvider(t)
		c, err := p.PostComment(ctx, &provider.PostCommentOptions{Body: "token " + secret})
		require.NoError(t, err)
		assert.NotContains(t, c.Body, secret)
		assert.NotContains(t, buf.String(), secret)
	})

	t.Run("marker invariant", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		_, err := p.PostComment(ctx, &provider.PostCommentOptions{Body: "no marker", Marker: "<!-- m -->"})
		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		assert.Empty(t, buf.String())

		_, err = p.PostComment(ctx, &provider.PostCommentOptions{Body: "has <!-- m -->", Marker: "<!-- m -->"})
		require.NoError(t, err)
	})

	t.Run("invalid options", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		tests := []struct {
			name string
			opts *provider.PostCommentOptions
		}{
			{"nil options", nil},
			{"empty body", &provider.PostCommentOptions{}},
			{"unknown behavior", &provider.PostCommentOptions{Body: "x", Behavior: "bogus"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				c, err := p.PostComment(ctx, tt.opts)
				require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
				assert.Nil(t, c)
			})
		}
	})
}

func TestAnnotate(t *testing.T) {
	p, buf := newBoundProvider(t)
	require.NoError(t, p.Annotate([]provider.Annotation{
		{Path: "main.tf", StartLine: 12, Level: provider.AnnotationError, Title: "T1", Message: "bad thing"},
		{Path: "vars.tf", StartLine: 3, Level: provider.AnnotationWarning, Title: "T2", Message: "meh"},
		{Path: "a.tf", Level: provider.AnnotationNotice, Message: "fyi"},
		{Level: provider.AnnotationError, Title: "T4", Message: "no path"},
	}))
	out := buf.String()
	assert.Contains(t, out, "main.tf:12: error: bad thing (T1)")
	assert.Contains(t, out, "vars.tf:3: warning: meh (T2)")
	assert.Contains(t, out, "a.tf: notice: fyi")
	assert.Contains(t, out, "error: no path (T4)")
}

func TestFormatAnnotation(t *testing.T) {
	tests := []struct {
		name string
		in   provider.Annotation
		want string
	}{
		{"full", provider.Annotation{Path: "a", StartLine: 1, Level: provider.AnnotationWarning, Title: "t", Message: "m"}, "a:1: warning: m (t)"},
		{"no title omits the parentheses", provider.Annotation{Path: "a", StartLine: 1, Level: provider.AnnotationWarning, Message: "m"}, "a:1: warning: m"},
		{"no line", provider.Annotation{Path: "a", Level: provider.AnnotationError, Title: "t", Message: "m"}, "a: error: m (t)"},
		{"no path", provider.Annotation{StartLine: 5, Level: provider.AnnotationNotice, Title: "t", Message: "m"}, "notice: m (t)"},
		{"message only defaults to the warning level", provider.Annotation{Message: "m"}, "warning: m"},
		{"unknown level defaults to the warning level", provider.Annotation{Level: "bogus", Message: "m"}, "warning: m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatAnnotation(&tt.in))
		})
	}
}

func TestReportSARIF(t *testing.T) {
	ctx := context.Background()

	t.Run("names the category and byte count", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		require.NoError(t, p.ReportSARIF(ctx, provider.SARIFReport{Body: []byte("12345"), Category: "trivy"}))
		assert.Contains(t, buf.String(), `SARIF report "trivy" (5 bytes) not uploaded: no CI provider detected`)
	})

	t.Run("names the file path when known", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		require.NoError(t, p.ReportSARIF(ctx, provider.SARIFReport{Body: []byte("12345"), Category: "trivy", Path: "reports/trivy.sarif"}))
		assert.Contains(t, buf.String(), `SARIF report "trivy" from reports/trivy.sarif (5 bytes) not uploaded: no CI provider detected`)
	})

	t.Run("names the switch that is off instead of claiming no provider", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		require.NoError(t, p.ReportSARIF(ctx, provider.SARIFReport{
			Body: []byte("12345"), Category: "trivy", Path: "reports/trivy.sarif", SkipReason: "ci.results.enabled is off",
		}))
		assert.Contains(t, buf.String(), `not uploaded: ci.results.enabled is off`)
		assert.NotContains(t, buf.String(), "no CI provider detected")
	})
}

func TestLogGroup(t *testing.T) {
	p, buf := newBoundProvider(t)
	require.NoError(t, p.StartLogGroup("Plan vpc"))
	assert.Contains(t, buf.String(), "── Plan vpc")
	before := buf.Len()
	require.NoError(t, p.EndLogGroup())
	assert.Equal(t, before, buf.Len())
}

func TestContext_PullRequestAndRun(t *testing.T) {
	t.Run("populated from env", func(t *testing.T) {
		t.Setenv("ATMOS_CI_PR", "42")
		t.Setenv("ATMOS_CI_BASE_REF", "main")
		t.Setenv("ATMOS_CI_BRANCH", "feature/x")
		t.Setenv("ATMOS_CI_EVENT", "push")
		t.Setenv("ATMOS_CI_RUN_ID", "99")
		t.Setenv("ATMOS_CI_RUN_URL", "https://ci.example.com/runs/99")

		ctx, err := NewProvider().Context()
		require.NoError(t, err)
		require.NotNil(t, ctx.PullRequest)
		assert.Equal(t, 42, ctx.PullRequest.Number)
		assert.Equal(t, "feature/x", ctx.PullRequest.HeadRef)
		assert.Equal(t, "main", ctx.PullRequest.BaseRef)
		assert.Equal(t, "push", ctx.EventName)
		assert.Equal(t, "99", ctx.RunID)
		assert.Equal(t, "https://ci.example.com/runs/99", ctx.RunURL)
	})

	forkTests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"1", true},
		{"TRUE", true},
		{"false", false},
		{"0", false},
		{"yes", false},
		{"", false},
	}
	for _, tt := range forkTests {
		t.Run("ATMOS_CI_PR_FORK="+tt.value, func(t *testing.T) {
			t.Setenv("ATMOS_CI_PR", "42")
			t.Setenv("ATMOS_CI_PR_FORK", tt.value)
			ctx, err := NewProvider().Context()
			require.NoError(t, err)
			require.NotNil(t, ctx.PullRequest)
			assert.Equal(t, tt.want, ctx.PullRequest.Fork)
		})
	}

	for _, v := range []string{"abc", "0", "-3", ""} {
		t.Run("invalid PR "+v, func(t *testing.T) {
			t.Setenv("ATMOS_CI_PR", v)
			ctx, err := NewProvider().Context()
			require.NoError(t, err)
			assert.Nil(t, ctx.PullRequest)
		})
	}
}

func TestEnvExporter(t *testing.T) {
	t.Run("files", func(t *testing.T) {
		dir := t.TempDir()
		envFile := filepath.Join(dir, "env")
		pathFile := filepath.Join(dir, "path")
		t.Setenv("ATMOS_CI_ENV", envFile)
		t.Setenv("ATMOS_CI_PATH", pathFile)

		p, buf := newBoundProvider(t)
		require.NoError(t, p.WriteEnv("FOO", "bar"))
		require.NoError(t, p.WriteEnv("MULTI", "a\nb"))
		require.NoError(t, p.AddPath("/opt/tool/bin"))

		got, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Equal(t, "FOO=bar\nMULTI<<EOF\na\nb\nEOF\n", string(got))
		got, err = os.ReadFile(pathFile)
		require.NoError(t, err)
		assert.Equal(t, "/opt/tool/bin\n", string(got))
		assert.Empty(t, buf.String())
	})

	t.Run("unwritable file returns sentinel", func(t *testing.T) {
		t.Setenv("ATMOS_CI_ENV", filepath.Join(t.TempDir(), "missing-dir", "env"))
		p, _ := newBoundProvider(t)
		require.ErrorIs(t, p.WriteEnv("A", "b"), errUtils.ErrCIEnvWriteFailed)
	})

	t.Run("rendered when unset", func(t *testing.T) {
		t.Setenv("ATMOS_CI_ENV", "")
		t.Setenv("ATMOS_CI_PATH", "")
		p, buf := newBoundProvider(t)
		require.NoError(t, p.WriteEnv("FOO", "it's here"))
		require.NoError(t, p.AddPath("/opt/my tool"))
		assert.Contains(t, buf.String(), `export FOO='it'"'"'s here'`)
		assert.Contains(t, buf.String(), `export PATH='/opt/my tool':"$PATH"`)
	})
}

func TestOutputWriter_Rendering(t *testing.T) {
	t.Run("WriteOutput without file renders to buffer", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		require.NoError(t, p.OutputWriter().WriteOutput("key", "value"))
		assert.Equal(t, "key=value\n", buf.String())
	})

	t.Run("WriteOutput multiline without a file renders the heredoc form", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		require.NoError(t, p.OutputWriter().WriteOutput("report", "line1\nline2"))
		assert.Equal(t, "report<<EOF\nline1\nline2\nEOF\n", buf.String())
	})

	t.Run("WriteOutput heredoc avoids delimiter collision", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "out")
		w := &OutputWriter{outputFile: file}
		require.NoError(t, w.WriteOutput("K", "x\nEOF\ny"))
		got, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, "K<<EOF_\nx\nEOF\ny\nEOF_\n", string(got))
	})

	t.Run("WriteOutput unwritable file returns sentinel", func(t *testing.T) {
		w := &OutputWriter{outputFile: filepath.Join(t.TempDir(), "nope", "out")}
		require.ErrorIs(t, w.WriteOutput("k", "v"), errUtils.ErrCIOutputWriteFailed)
	})

	t.Run("WriteSummary without file renders markdown", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		require.NoError(t, p.OutputWriter().WriteSummary("# Heading\n\nsummary body"))
		assert.Contains(t, buf.String(), "Heading")
		assert.Contains(t, buf.String(), "summary")
	})

	t.Run("WriteSummary unbound emits raw markdown", func(t *testing.T) {
		var buf bytes.Buffer
		w := &OutputWriter{out: ui.New(&buf)}
		require.NoError(t, w.WriteSummary("## Plan Failed\n\n<summary>x</summary>"))
		assert.Equal(t, "## Plan Failed\n\n<summary>x</summary>\n", buf.String())
	})

	t.Run("WriteSummary unwritable file returns sentinel", func(t *testing.T) {
		w := &OutputWriter{summaryFile: filepath.Join(t.TempDir(), "nope", "sum")}
		require.ErrorIs(t, w.WriteSummary("x"), errUtils.ErrCISummaryWriteFailed)
	})
}

func TestCheckRun_RendersTheDetailsURL(t *testing.T) {
	ctx := context.Background()

	t.Run("create names the URL when set", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		_, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{Name: "chk", Status: provider.CheckRunStatePending, DetailsURL: "https://ci.example/run/1"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "URL: https://ci.example/run/1")
	})

	t.Run("update names the URL when set", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		_, err := p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{Name: "chk", Status: provider.CheckRunStateSuccess, DetailsURL: "https://ci.example/run/1"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "URL: https://ci.example/run/1")
	})

	t.Run("no URL line without one", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		_, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{Name: "chk", Status: provider.CheckRunStatePending})
		require.NoError(t, err)
		assert.NotContains(t, buf.String(), "URL:")
	})
}

func TestCheckRun_RendersToBoundWriter(t *testing.T) {
	p, buf := newBoundProvider(t)
	_, err := p.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{Name: "chk", Status: provider.CheckRunStatePending, Title: "ttl"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Check run created: chk")
	assert.Contains(t, buf.String(), "Title: ttl")
}

func TestPostComment_UpdateRequiresAnEarlierComment(t *testing.T) {
	ctx := context.Background()
	const marker = "<!-- atmos:ci:plan -->"
	opts := func(behavior provider.CommentBehavior, pr int) *provider.PostCommentOptions {
		return &provider.PostCommentOptions{PRNumber: pr, Marker: marker, Body: marker + "\nbody", Behavior: behavior}
	}

	t.Run("update without a prior comment fails like GitHub", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		c, err := p.PostComment(ctx, opts(provider.CommentBehaviorUpdate, 7))
		require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
		assert.Nil(t, c)
		assert.Empty(t, buf.String(), "a failed update renders no preview")
	})

	t.Run("update after a create reuses the comment", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		first, err := p.PostComment(ctx, opts(provider.CommentBehaviorCreate, 7))
		require.NoError(t, err)
		assert.True(t, first.Created)

		second, err := p.PostComment(ctx, opts(provider.CommentBehaviorUpdate, 7))
		require.NoError(t, err)
		assert.False(t, second.Created)
		assert.Equal(t, first.ID, second.ID)
	})

	t.Run("a comment on another pull request is not a match", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		_, err := p.PostComment(ctx, opts(provider.CommentBehaviorCreate, 7))
		require.NoError(t, err)
		_, err = p.PostComment(ctx, opts(provider.CommentBehaviorUpdate, 8))
		require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
	})

	t.Run("upsert creates then updates", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		first, err := p.PostComment(ctx, opts(provider.CommentBehaviorUpsert, 7))
		require.NoError(t, err)
		assert.True(t, first.Created)
		second, err := p.PostComment(ctx, opts(provider.CommentBehaviorUpsert, 7))
		require.NoError(t, err)
		assert.False(t, second.Created)
		assert.Equal(t, first.ID, second.ID)
	})

	t.Run("markers are shared by providers bound from the same base", func(t *testing.T) {
		base := NewProvider()
		var a, b bytes.Buffer
		_, err := base.BindOutput(&a).PostComment(ctx, opts(provider.CommentBehaviorCreate, 7))
		require.NoError(t, err)
		_, err = base.BindOutput(&b).PostComment(ctx, opts(provider.CommentBehaviorUpdate, 7))
		require.NoError(t, err)
	})
}

func TestPostCommitComment(t *testing.T) {
	ctx := context.Background()
	const marker = "<!-- atmos:ci:deploy -->"
	opts := func(behavior provider.CommentBehavior, sha string) *provider.PostCommitCommentOptions {
		return &provider.PostCommitCommentOptions{SHA: sha, Marker: marker, Body: marker + "\nbody", Behavior: behavior}
	}

	t.Run("renders a commit comment preview", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		c, err := p.PostCommitComment(ctx, opts(provider.CommentBehaviorCreate, "0123456789abcdef"))
		require.NoError(t, err)
		assert.True(t, c.Created)
		assert.Contains(t, buf.String(), "commit comment preview (create, commit 0123456)")
		assert.Contains(t, buf.String(), "body")
	})

	t.Run("without a SHA the preview omits it", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		_, err := p.PostCommitComment(ctx, opts(provider.CommentBehaviorUpsert, ""))
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "commit comment preview (upsert)")
	})

	t.Run("update requires an earlier commit comment on the same commit", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		_, err := p.PostCommitComment(ctx, opts(provider.CommentBehaviorUpdate, "aaaaaaaa"))
		require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)

		_, err = p.PostCommitComment(ctx, opts(provider.CommentBehaviorCreate, "aaaaaaaa"))
		require.NoError(t, err)
		c, err := p.PostCommitComment(ctx, opts(provider.CommentBehaviorUpdate, "aaaaaaaa"))
		require.NoError(t, err)
		assert.False(t, c.Created)

		_, err = p.PostCommitComment(ctx, opts(provider.CommentBehaviorUpdate, "bbbbbbbb"))
		require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
	})

	t.Run("a pull request comment is not a commit comment match", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		_, err := p.PostComment(ctx, &provider.PostCommentOptions{PRNumber: 1, Marker: marker, Body: marker, Behavior: provider.CommentBehaviorCreate})
		require.NoError(t, err)
		_, err = p.PostCommitComment(ctx, opts(provider.CommentBehaviorUpdate, "aaaaaaaa"))
		require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
	})

	t.Run("invalid options", func(t *testing.T) {
		p, buf := newBoundProvider(t)
		for _, o := range []*provider.PostCommitCommentOptions{nil, {}, {Body: "x", Marker: "<!-- m -->"}} {
			c, err := p.PostCommitComment(ctx, o)
			require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
			assert.Nil(t, c)
		}
		assert.Empty(t, buf.String())
	})
}

func TestUpdateCheckRun_KeepsTheCheckHandle(t *testing.T) {
	ctx := context.Background()

	t.Run("reuses the ID the caller holds and carries the details URL", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		created, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{Name: "n", Status: provider.CheckRunStatePending, DetailsURL: "https://ci.example/run/1"})
		require.NoError(t, err)
		assert.Equal(t, "https://ci.example/run/1", created.DetailsURL)

		updated, err := p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{ID: created.ID, Name: "n", Status: provider.CheckRunStateSuccess, DetailsURL: "https://ci.example/run/1"})
		require.NoError(t, err)
		assert.Equal(t, created.ID, updated.ID)
		assert.Equal(t, "https://ci.example/run/1", updated.DetailsURL)
	})

	t.Run("without an ID it allocates a new one", func(t *testing.T) {
		p, _ := newBoundProvider(t)
		created, err := p.CreateCheckRun(ctx, &provider.CreateCheckRunOptions{Name: "n", Status: provider.CheckRunStatePending})
		require.NoError(t, err)
		updated, err := p.UpdateCheckRun(ctx, &provider.UpdateCheckRunOptions{Name: "n", Status: provider.CheckRunStateSuccess})
		require.NoError(t, err)
		assert.NotEqual(t, created.ID, updated.ID)
	})
}
