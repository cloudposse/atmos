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
	assert.Contains(t, out, "main.tf:12: T1: bad thing")
	assert.Contains(t, out, "vars.tf:3: T2: meh")
	assert.Contains(t, out, "a.tf: fyi")
	assert.Contains(t, out, "T4: no path")
}

func TestFormatAnnotation(t *testing.T) {
	tests := []struct {
		name string
		in   provider.Annotation
		want string
	}{
		{"full", provider.Annotation{Path: "a", StartLine: 1, Title: "t", Message: "m"}, "a:1: t: m"},
		{"no line", provider.Annotation{Path: "a", Title: "t", Message: "m"}, "a: t: m"},
		{"no path", provider.Annotation{StartLine: 5, Title: "t", Message: "m"}, "t: m"},
		{"message only", provider.Annotation{Message: "m"}, "m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatAnnotation(&tt.in))
		})
	}
}

func TestReportSARIF(t *testing.T) {
	p, buf := newBoundProvider(t)
	require.NoError(t, p.ReportSARIF(context.Background(), provider.SARIFReport{Body: []byte("12345"), Category: "trivy"}))
	assert.Contains(t, buf.String(), `SARIF report "trivy" (5 bytes) not uploaded: no CI provider detected`)
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

func TestCheckRun_RendersToBoundWriter(t *testing.T) {
	p, buf := newBoundProvider(t)
	_, err := p.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{Name: "chk", Status: provider.CheckRunStatePending, Title: "ttl"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Check run created: chk")
	assert.Contains(t, buf.String(), "Title: ttl")
}
