package ci_test

import (
	"bytes"
	"context"
	stdio "io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/data"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// captureStreams is a minimal Streams implementation that captures the data (stdout)
// and UI (stderr) channels so tests can assert on what each channel received.
type captureStreams struct {
	stdout stdio.Writer
	stderr stdio.Writer
}

func (s *captureStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *captureStreams) Output() stdio.Writer    { return s.stdout }
func (s *captureStreams) Error() stdio.Writer     { return s.stderr }
func (s *captureStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *captureStreams) RawError() stdio.Writer  { return s.stderr }

// initIO points the global data writer and UI formatter at captured streams and returns the
// buffer that receives the data channel (stdout). Defaults are restored when the test ends.
func initIO(t *testing.T, uiStderr ...*bytes.Buffer) *bytes.Buffer {
	t.Helper()

	// CI runners advertise color support; the assertions compare plain text.
	t.Setenv("NO_COLOR", "1")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if len(uiStderr) > 0 {
		stderr = uiStderr[0]
	}
	ioCtx, err := atmosio.NewContext(atmosio.WithStreams(&captureStreams{stdout: stdout, stderr: stderr}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	t.Cleanup(func() {
		defaultCtx, err := atmosio.NewContext()
		require.NoError(t, err)
		data.InitWriter(defaultCtx)
		ui.InitFormatter(defaultCtx)
	})
	return stdout
}

// captureOSStderr redirects os.Stderr, which the global I/O context writes workflow commands to, into
// a temporary file. The returned function reads everything written so far.
func captureOSStderr(t *testing.T) func() string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)
	original := os.Stderr
	os.Stderr = f
	t.Cleanup(func() {
		os.Stderr = original
		_ = f.Close()
	})

	return func() string {
		t.Helper()
		data, err := os.ReadFile(f.Name())
		require.NoError(t, err)
		return string(data)
	}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// isolateLocalEnv blanks every variable that could redirect the generic provider's output
// or make a provider detect itself.
func isolateLocalEnv(t *testing.T) {
	t.Helper()

	for _, k := range []string{
		"CI", "ATMOS_CI", "GITHUB_ACTIONS", "ATMOS_CI_OUTPUT", "ATMOS_CI_SUMMARY", "ATMOS_CI_ENV", "ATMOS_CI_PATH",
		"ATMOS_CI_PR", "ATMOS_CI_BASE_REF", "ATMOS_CI_REPOSITORY", "ATMOS_CI_RUN_URL", "ATMOS_CI_RUN_ID",
	} {
		t.Setenv(k, "")
	}
}

// newLocalReporter returns a reporter on a registry holding only the generic provider, so
// no CI provider is detected, along with the buffer its local renderings go to.
func newLocalReporter(t *testing.T, cfg *schema.AtmosConfiguration) (ci.Reporter, *bytes.Buffer) {
	t.Helper()

	isolateLocalEnv(t)
	initIO(t)
	t.Cleanup(ci.SwapRegistryForTest())
	ci.Register(generic.NewProvider())

	var buf bytes.Buffer
	return ci.NewReporter(cfg).WithOutput(&buf), &buf
}

// localCase is one Reporter call and what its local rendering must contain.
type localCase struct {
	name string
	call func(t *testing.T, r ci.Reporter) (ci.Receipt, error)
	// want lists substrings the bound writer must contain.
	want []string
	// check runs extra assertions on the receipt.
	check func(t *testing.T, rc ci.Receipt)
	// providerUnchecked skips the receipt's Provider assertion. It is covered by a dedicated test.
	providerUnchecked bool
}

func localCases() []localCase {
	const secret = "local-parity-secret-Zx9Q7"
	return []localCase{
		{
			name: "Summary",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) {
				return r.Summary("# Heading\n\nadded resources")
			},
			want: []string{"Heading", "added resources"},
		},
		{
			name: "Comment with key",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) {
				return r.Comment(context.Background(), ci.CommentRequest{Body: "plan looks fine", Key: "plan"})
			},
			// With no pull request known, target=auto comments on the commit.
			want: []string{"commit comment preview (upsert", "plan looks fine"},
			check: func(t *testing.T, rc ci.Receipt) {
				require.NotNil(t, rc.Comment)
				assert.True(t, rc.Comment.Created)
				assert.Empty(t, rc.Comment.URL)
				assert.Contains(t, rc.Comment.Body, "<!-- atmos:ci:plan -->")
				assert.Contains(t, rc.Comment.Body, "plan looks fine")
				assert.Equal(t, ci.CommentTargetCommit, rc.Comment.Target)
			},
		},
		{
			name: "Comment on a pull request",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) {
				return r.Comment(context.Background(), ci.CommentRequest{Body: "plan looks fine", Key: "plan", PR: 42})
			},
			want: []string{"PR comment preview (upsert, PR #42)", "plan looks fine"},
			check: func(t *testing.T, rc ci.Receipt) {
				require.NotNil(t, rc.Comment)
				assert.Equal(t, ci.CommentTargetPR, rc.Comment.Target)
			},
		},
		{
			name: "Annotate",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) {
				return r.Annotate(ci.Annotation{Path: "main.tf", StartLine: 12, Level: ci.AnnotationError, Message: "bucket is public"})
			},
			want: []string{"main.tf:12: error: bucket is public"},
		},
		{
			name: "Output",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) { return r.Output("plan_id", "abc123") },
			want: []string{"plan_id=abc123"},
		},
		{
			name: "Env",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) { return r.Env("TF_IN_AUTOMATION", "true") },
			want: []string{"export TF_IN_AUTOMATION="},
		},
		{
			name: "Path",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) { return r.Path("/opt/tools/bin") },
			want: []string{"export PATH=", "/opt/tools/bin"},
		},
		{
			name: "Group",
			call: func(t *testing.T, r ci.Reporter) (ci.Receipt, error) {
				end, rc, err := r.Group("Deploy vpc")
				require.NotNil(t, end)
				end()
				return rc, err
			},
			want: []string{"Deploy vpc"},
		},
		{
			name: "Check",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) {
				return r.Check(context.Background(), ci.CheckRequest{Name: "atmos/plan"})
			},
			want: []string{"Check run created: atmos/plan [pending]"},
			check: func(t *testing.T, rc ci.Receipt) {
				require.NotNil(t, rc.Check)
				assert.Equal(t, "atmos/plan", rc.Check.Name)
				assert.Equal(t, ci.CheckRunStatePending, rc.Check.Status)
			},
		},
		{
			name: "UpdateCheck",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) {
				return r.UpdateCheck(context.Background(), ci.CheckRequest{Name: "atmos/plan", State: ci.CheckRunStateSuccess})
			},
			want: []string{"Check run completed: atmos/plan [success]"},
			check: func(t *testing.T, rc ci.Receipt) {
				require.NotNil(t, rc.Check)
				assert.Equal(t, ci.CheckRunStateSuccess, rc.Check.Status)
				assert.Equal(t, "success", rc.Check.Conclusion)
			},
		},
		{
			name: "SARIF",
			call: func(_ *testing.T, r ci.Reporter) (ci.Receipt, error) {
				return r.SARIF(context.Background(), ci.SARIFReport{Body: []byte(`{"runs":[]}`), Category: "tfsec"})
			},
			want: []string{`SARIF report "tfsec"`, "not uploaded"},
		},
		{
			name:              "Mask",
			providerUnchecked: true,
			call: func(t *testing.T, r ci.Reporter) (ci.Receipt, error) {
				t.Cleanup(atmosio.Reset)
				rc, err := r.Mask(secret)
				assert.NotContains(t, atmosio.MaskString("x"+secret+"y"), secret, "the secret must be registered with the Atmos masker")
				return rc, err
			},
		},
	}
}

// TestReporter_LocalParity drives every Reporter call with no CI provider detected and asserts
// the generic provider renders it locally with no gate.
func TestReporter_LocalParity(t *testing.T) {
	configs := map[string]*schema.AtmosConfiguration{
		"nil config":         nil,
		"ci disabled":        {CI: schema.CIConfig{Enabled: false}},
		"everything enabled": {CI: schema.CIConfig{Enabled: true, Comments: schema.CICommentsConfig{Enabled: ptr(true)}, Checks: schema.CIChecksConfig{Enabled: ptr(true)}}},
	}
	for cfgName, cfg := range configs {
		for _, tc := range localCases() {
			t.Run(cfgName+"/"+tc.name, func(t *testing.T) {
				r, buf := newLocalReporter(t, cfg)

				rc, err := tc.call(t, r)

				require.NoError(t, err)
				assert.True(t, rc.Local)
				assert.Empty(t, rc.Gate, "flags are irrelevant when no provider is detected")
				if !tc.providerUnchecked {
					assert.Equal(t, generic.ProviderName, rc.Provider)
				}
				for _, want := range tc.want {
					assert.Contains(t, buf.String(), want)
				}
				if tc.check != nil {
					tc.check(t, rc)
				}
			})
		}
	}
}

// TestReporter_LocalContext covers the context the generic provider reports locally.
func TestReporter_LocalContext(t *testing.T) {
	t.Run("provider name", func(t *testing.T) {
		r, _ := newLocalReporter(t, nil)
		c, err := r.Context()
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.Equal(t, generic.ProviderName, c.Provider)
		assert.Nil(t, c.PullRequest, "no pull request without ATMOS_CI_PR")
	})

	t.Run("ATMOS_CI_PR yields the pull request number", func(t *testing.T) {
		r, _ := newLocalReporter(t, nil)
		// The context is resolved lazily, so variables set after construction are honored.
		t.Setenv("ATMOS_CI_PR", "42")
		t.Setenv("ATMOS_CI_REPOSITORY", "owner/repo")
		c, err := r.Context()
		require.NoError(t, err)
		require.NotNil(t, c.PullRequest)
		assert.Equal(t, 42, c.PullRequest.Number)
		assert.Equal(t, "owner", c.RepoOwner)
		assert.Equal(t, "repo", c.RepoName)
	})
}

// TestReporter_LocalMaskReceiptNamesProvider documents that a locally handled Mask names the
// generic provider in its receipt like every other local call. Today Mask returns a receipt with
// an empty Provider because it registers the secret with the Atmos masker without routing
// through a provider, so this test fails until the receipt names one.
func TestReporter_LocalMaskReceiptNamesProvider(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	r, _ := newLocalReporter(t, nil)

	rc, err := r.Mask("local-mask-receipt-secret-Bn47")

	require.NoError(t, err)
	assert.True(t, rc.Local)
	assert.Empty(t, rc.Gate)
	assert.Equal(t, generic.ProviderName, rc.Provider)
}
