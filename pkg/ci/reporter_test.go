package ci

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	cockroachdb "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields and constants these tests depend on.
var (
	_ = schema.CIConfig{AllowUnsafeForkExecution: true}
	_ = schema.CIGroupsConfig{Mode: GroupModeOff}
	_ = Reporter(&reporter{})
)

// recordingWriter is an OutputWriter that records writes.
type recordingWriter struct {
	outputs   [][2]string
	summaries []string
	err       error
}

func (w *recordingWriter) WriteOutput(key, value string) error {
	w.outputs = append(w.outputs, [2]string{key, value})
	return w.err
}

func (w *recordingWriter) WriteSummary(content string) error {
	w.summaries = append(w.summaries, content)
	return w.err
}

// capableFake is a Provider that implements every optional reporting capability and records calls.
type capableFake struct {
	*MockProvider
	writer      *recordingWriter
	err         error
	annotations [][]Annotation
	sarif       []SARIFReport
	groups      []string
	ends        int
	envs        [][2]string
	paths       []string
	masked      []string
	commits     []PostCommitCommentOptions
	commitErr   error
	boundTo     io.Writer
}

func (f *capableFake) PostCommitComment(_ context.Context, o *PostCommitCommentOptions) (*Comment, error) {
	f.commits = append(f.commits, *o)
	if f.commitErr != nil {
		return nil, f.commitErr
	}
	return &Comment{ID: int64(len(f.commits)), Body: o.Body, Created: true}, nil
}

func (f *capableFake) Annotate(a []Annotation) error {
	f.annotations = append(f.annotations, a)
	return f.err
}

func (f *capableFake) ReportSARIF(_ context.Context, r SARIFReport) error {
	f.sarif = append(f.sarif, r)
	return f.err
}

func (f *capableFake) StartLogGroup(title string) error {
	f.groups = append(f.groups, title)
	return f.err
}

func (f *capableFake) EndLogGroup() error {
	f.ends++
	return f.err
}

func (f *capableFake) WriteEnv(k, v string) error {
	f.envs = append(f.envs, [2]string{k, v})
	return f.err
}

func (f *capableFake) AddPath(dir string) error {
	f.paths = append(f.paths, dir)
	return f.err
}

func (f *capableFake) MaskValue(v string) error {
	f.masked = append(f.masked, v)
	return f.err
}

func (f *capableFake) BindOutput(w io.Writer) provider.Provider {
	f.boundTo = w
	return f
}

// kind selects which fake a harness registers.
type kind int

const (
	kindNone    kind = iota // Nothing registered.
	kindPlain               // MockProvider with no optional capabilities.
	kindCapable             // capableFake.
)

// harness wires a detected provider and a generic fallback into a swapped registry.
type harness struct {
	detected, local             *capableFake
	detectedPlain, localPlain   *MockProvider
	detectedWriter, localWriter *recordingWriter
}

func newFake(t *testing.T, ctrl *gomock.Controller, name string, detect bool) (*MockProvider, *recordingWriter) {
	t.Helper()
	m := NewMockProvider(ctrl)
	w := &recordingWriter{}
	m.EXPECT().Name().Return(name).AnyTimes()
	m.EXPECT().Detect().Return(detect).AnyTimes()
	m.EXPECT().OutputWriter().Return(w).AnyTimes()
	return m, w
}

func newHarness(t *testing.T, detected, local kind) *harness {
	t.Helper()
	// A forced CI mode would make the generic provider the detected one, and a parent Atmos
	// process (the CI job that runs these tests inside a log group) would make every group nested.
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")
	t.Setenv(logGroupSentinelEnvVar, "")
	restore := SwapRegistryForTest()
	t.Cleanup(restore)
	ctrl := gomock.NewController(t)
	h := &harness{}

	register := func(k kind, name string, detect bool) (*capableFake, *MockProvider, *recordingWriter) {
		if k == kindNone {
			return nil, nil, nil
		}
		m, w := newFake(t, ctrl, name, detect)
		if k == kindPlain {
			Register(m)
			return nil, m, w
		}
		f := &capableFake{MockProvider: m, writer: w}
		Register(f)
		return f, m, w
	}
	h.detected, h.detectedPlain, h.detectedWriter = register(detected, "detected", true)
	h.local, h.localPlain, h.localWriter = register(local, "generic", false)
	return h
}

// ciCtx sets the Context returned by the detected provider.
func (h *harness) ciCtx(c *Context) {
	if h.detectedPlain != nil {
		h.detectedPlain.EXPECT().Context().Return(c, nil).AnyTimes()
	}
	if h.localPlain != nil {
		h.localPlain.EXPECT().Context().Return(&Context{}, nil).AnyTimes()
	}
}

func ciConfig(mutate func(*schema.CIConfig)) *schema.AtmosConfiguration {
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Enabled = true
	if mutate != nil {
		mutate(&cfg.CI)
	}
	return cfg
}

func TestReporterTarget(t *testing.T) {
	features := []struct {
		feature Feature
		enabled func(*schema.AtmosConfiguration) bool
		on      func(*schema.CIConfig)
		off     func(*schema.CIConfig)
	}{
		{FeatureSummary, SummaryEnabled, func(c *schema.CIConfig) { c.Summary.Enabled = boolPtr(true) }, func(c *schema.CIConfig) { c.Summary.Enabled = boolPtr(false) }},
		{FeatureOutput, OutputEnabled, func(c *schema.CIConfig) { c.Output.Enabled = boolPtr(true) }, func(c *schema.CIConfig) { c.Output.Enabled = boolPtr(false) }},
		{FeatureAnnotations, AnnotationsEnabled, func(c *schema.CIConfig) { c.Annotations.Enabled = boolPtr(true) }, func(c *schema.CIConfig) { c.Annotations.Enabled = boolPtr(false) }},
		{FeatureResults, ResultsEnabled, func(c *schema.CIConfig) { c.Results.Enabled = boolPtr(true) }, func(c *schema.CIConfig) { c.Results.Enabled = boolPtr(false) }},
		{FeatureChecks, ChecksEnabled, func(c *schema.CIConfig) { c.Checks.Enabled = boolPtr(true) }, func(c *schema.CIConfig) { c.Checks.Enabled = boolPtr(false) }},
		{FeatureComments, CommentsEnabled, func(c *schema.CIConfig) { c.Comments.Enabled = boolPtr(true) }, func(c *schema.CIConfig) { c.Comments.Enabled = boolPtr(false) }},
		{FeatureGroups, func(c *schema.AtmosConfiguration) bool { return resolveGroupMode(c) != GroupModeOff }, func(c *schema.CIConfig) { c.Groups.Mode = GroupModeAuto }, func(c *schema.CIConfig) { c.Groups.Mode = GroupModeOff }},
	}

	for _, f := range features {
		cases := []struct {
			name         string
			cfg          *schema.AtmosConfiguration
			detect       bool
			wantProvider string
			wantLocal    bool
			wantGate     Feature
		}{
			// With the master switch off, every per-feature switch reads off too; the master switch is the one named.
			{"nil config", nil, true, "generic", true, FeatureEnabled},
			{"ci disabled", &schema.AtmosConfiguration{}, true, "generic", true, FeatureEnabled},
			{"feature off", ciConfig(f.off), true, "generic", true, f.feature},
			{"feature on", ciConfig(f.on), true, "detected", false, ""},
			{"no detection feature on", ciConfig(f.on), false, "generic", true, ""},
			{"no detection nil config", nil, false, "generic", true, ""},
		}
		for _, tc := range cases {
			t.Run(string(f.feature)+"/"+tc.name, func(t *testing.T) {
				detected := kindNone
				if tc.detect {
					detected = kindPlain
				}
				newHarness(t, detected, kindPlain)

				r := NewReporter(tc.cfg).(*reporter)
				p, rc := r.target(f.feature, f.enabled)

				require.NotNil(t, p)
				assert.Equal(t, tc.wantProvider, p.Name())
				assert.Equal(t, tc.wantProvider, rc.Provider)
				assert.Equal(t, tc.wantLocal, rc.Local)
				assert.Equal(t, tc.wantGate, rc.Gate)
			})
		}
	}
}

func TestReporterTarget_NothingRegistered(t *testing.T) {
	newHarness(t, kindNone, kindNone)

	r := NewReporter(ciConfig(nil)).(*reporter)
	p, rc := r.target(FeatureSummary, SummaryEnabled)

	assert.Nil(t, p)
	assert.Equal(t, Receipt{Local: true}, rc)
}

func TestReporter_NothingRegisteredIsNoOp(t *testing.T) {
	newHarness(t, kindNone, kindNone)
	r := NewReporter(ciConfig(nil))

	rc, err := r.Summary("x")
	require.NoError(t, err)
	assert.Equal(t, Receipt{Local: true}, rc)

	rc, err = r.Comment(context.Background(), CommentRequest{Body: "x"})
	require.NoError(t, err)
	assert.Equal(t, Receipt{Local: true}, rc)

	end, rc, err := r.Group("g")
	require.NoError(t, err)
	assert.Equal(t, Receipt{Local: true}, rc)
	end()

	_, err = r.Context()
	require.ErrorIs(t, err, errUtils.ErrCIProviderNotDetected)
	_, err = r.Base()
	require.ErrorIs(t, err, errUtils.ErrCIProviderNotDetected)
}

func TestReporter_SummaryAndOutputRouting(t *testing.T) {
	t.Run("enabled goes to the detected provider", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		r := NewReporter(ciConfig(nil))

		rc, err := r.Summary("# hi")
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		assert.Equal(t, []string{"# hi"}, h.detectedWriter.summaries)
		assert.Empty(t, h.localWriter.summaries)

		rc, err = r.Output("k", "v")
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		assert.Equal(t, [][2]string{{"k", "v"}}, h.detectedWriter.outputs)
	})

	t.Run("gated goes to the local provider", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		r := NewReporter(ciConfig(func(c *schema.CIConfig) {
			c.Summary.Enabled = boolPtr(false)
			c.Output.Enabled = boolPtr(false)
		}))

		rc, err := r.Summary("# hi")
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureSummary}, rc)
		assert.Equal(t, []string{"# hi"}, h.localWriter.summaries)
		assert.Empty(t, h.detectedWriter.summaries)

		rc, err = r.Output("k", "v")
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureOutput}, rc)
		assert.Equal(t, [][2]string{{"k", "v"}}, h.localWriter.outputs)
	})

	t.Run("undetected goes to the local provider without a gate", func(t *testing.T) {
		h := newHarness(t, kindNone, kindPlain)
		r := NewReporter(ciConfig(nil))

		rc, err := r.Summary("# hi")
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true}, rc)
		assert.Equal(t, []string{"# hi"}, h.localWriter.summaries)
	})

	t.Run("write errors carry the sentinel", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.detectedWriter.err = errors.New("disk full")
		r := NewReporter(ciConfig(nil))

		_, err := r.Summary("x")
		require.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
		assert.ErrorContains(t, err, "disk full")

		_, err = r.Output("k", "v")
		require.ErrorIs(t, err, errUtils.ErrCIOutputWriteFailed)
	})
}

func TestReporter_CapabilityRouting(t *testing.T) {
	annotation := Annotation{Path: "main.tf", StartLine: 3, Level: AnnotationWarning, Message: "m"}
	report := SARIFReport{Body: []byte("{}"), Category: "c", Path: "reports/scan.sarif"}
	enableAll := func(c *schema.CIConfig) { c.Results.Enabled = boolPtr(true) }
	disableAll := func(c *schema.CIConfig) {
		c.Output.Enabled = boolPtr(false)
		c.Annotations.Enabled = boolPtr(false)
		c.Groups.Mode = GroupModeOff
		c.Results.Enabled = boolPtr(false)
	}

	// exercise runs every capability-backed method and returns the receipts in a stable order.
	exercise := func(t *testing.T, r Reporter) []Receipt {
		t.Helper()
		var receipts []Receipt
		rc, err := r.Env("A", "1")
		require.NoError(t, err)
		receipts = append(receipts, rc)
		rc, err = r.Path("/bin")
		require.NoError(t, err)
		receipts = append(receipts, rc)
		rc, err = r.Annotate(annotation)
		require.NoError(t, err)
		receipts = append(receipts, rc)
		rc, err = r.SARIF(context.Background(), report)
		require.NoError(t, err)
		receipts = append(receipts, rc)
		end, rc, err := r.Group("title")
		require.NoError(t, err)
		end()
		receipts = append(receipts, rc)
		return receipts
	}

	t.Run("enabled routes to the detected provider", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(&Context{})
		receipts := exercise(t, NewReporter(ciConfig(enableAll)))

		for _, rc := range receipts {
			assert.Equal(t, Receipt{Provider: "detected"}, rc)
		}
		assert.Equal(t, [][2]string{{"A", "1"}}, h.detected.envs)
		assert.Equal(t, []string{"/bin"}, h.detected.paths)
		assert.Equal(t, [][]Annotation{{annotation}}, h.detected.annotations)
		assert.Equal(t, []SARIFReport{report}, h.detected.sarif)
		assert.Equal(t, []string{"title"}, h.detected.groups)
		assert.Equal(t, 1, h.detected.ends)
		assert.Empty(t, h.local.envs)
		assert.Empty(t, h.local.groups)
	})

	t.Run("gated routes to the local provider and records the gate", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(&Context{})
		receipts := exercise(t, NewReporter(ciConfig(disableAll)))

		gates := []Feature{FeatureOutput, FeatureOutput, FeatureAnnotations, FeatureResults, FeatureGroups}
		for i, rc := range receipts {
			assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: gates[i]}, rc)
		}
		assert.Equal(t, [][2]string{{"A", "1"}}, h.local.envs)
		assert.Equal(t, []string{"/bin"}, h.local.paths)
		assert.Equal(t, [][]Annotation{{annotation}}, h.local.annotations)
		gatedReport := report
		gatedReport.SkipReason = "ci.results.enabled is off"
		assert.Equal(t, []SARIFReport{gatedReport}, h.local.sarif, "the local rendering says which switch is off")
		assert.Equal(t, []string{"title"}, h.local.groups)
		assert.Equal(t, 1, h.local.ends)
		assert.Empty(t, h.detected.envs)
		assert.Empty(t, h.detected.annotations)
	})

	t.Run("undetected routes to the local provider without a gate", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		receipts := exercise(t, NewReporter(ciConfig(enableAll)))

		for _, rc := range receipts {
			assert.Equal(t, Receipt{Provider: "generic", Local: true}, rc)
		}
		assert.Equal(t, [][]Annotation{{annotation}}, h.local.annotations)
	})

	t.Run("detected provider without the capability falls through to local", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindCapable)
		h.ciCtx(&Context{})
		receipts := exercise(t, NewReporter(ciConfig(enableAll)))

		for _, rc := range receipts {
			assert.Equal(t, Receipt{Provider: "generic", Local: true}, rc)
		}
		assert.Equal(t, [][2]string{{"A", "1"}}, h.local.envs)
		assert.Equal(t, [][]Annotation{{annotation}}, h.local.annotations)
		assert.Equal(t, []SARIFReport{report}, h.local.sarif)
	})

	t.Run("neither provider implements the capability", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(&Context{})
		receipts := exercise(t, NewReporter(ciConfig(enableAll)))

		for _, rc := range receipts {
			assert.Equal(t, Receipt{Provider: "detected"}, rc)
		}
	})

	t.Run("gated with no local capability is a silent no-op", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindPlain)
		h.ciCtx(&Context{})
		receipts := exercise(t, NewReporter(ciConfig(disableAll)))

		gates := []Feature{FeatureOutput, FeatureOutput, FeatureAnnotations, FeatureResults, FeatureGroups}
		for i, rc := range receipts {
			assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: gates[i]}, rc)
		}
		assert.Empty(t, h.detected.annotations)
		assert.Empty(t, h.detected.envs)
		assert.Empty(t, h.detected.groups)
	})
}

func TestReporter_CapabilityErrors(t *testing.T) {
	boom := errors.New("boom")
	newReporter := func(t *testing.T) Reporter {
		h := newHarness(t, kindCapable, kindNone)
		h.ciCtx(&Context{})
		h.detected.err = boom
		return NewReporter(ciConfig(func(c *schema.CIConfig) { c.Results.Enabled = boolPtr(true) }))
	}

	t.Run("env", func(t *testing.T) {
		_, err := newReporter(t).Env("k", "v")
		require.ErrorIs(t, err, errUtils.ErrCIEnvWriteFailed)
		require.ErrorIs(t, err, boom)
	})
	t.Run("path", func(t *testing.T) {
		_, err := newReporter(t).Path("/bin")
		require.ErrorIs(t, err, errUtils.ErrCIEnvWriteFailed)
	})
	t.Run("annotate", func(t *testing.T) {
		_, err := newReporter(t).Annotate(Annotation{})
		require.ErrorIs(t, err, errUtils.ErrCIAnnotationFailed)
	})
	t.Run("sarif", func(t *testing.T) {
		_, err := newReporter(t).SARIF(context.Background(), SARIFReport{})
		require.ErrorIs(t, err, errUtils.ErrCISARIFUploadFailed)
	})
	t.Run("mask", func(t *testing.T) {
		_, err := newReporter(t).Mask("some-secret-value")
		require.ErrorIs(t, err, errUtils.ErrCIMaskFailed)
	})
	t.Run("group start", func(t *testing.T) {
		_, _, err := newReporter(t).Group("g")
		require.Error(t, err)
		require.ErrorIs(t, err, boom)
	})
	t.Run("group end errors are not returned", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		r := NewReporter(ciConfig(nil))
		end, _, err := r.Group("g")
		require.NoError(t, err)
		h.detected.err = boom
		assert.NotPanics(t, end)
		assert.Equal(t, 1, h.detected.ends)
	})
}

func TestReporter_Mask(t *testing.T) {
	t.Run("registers the secret and masks in the detected provider", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		secret := "reporter-mask-secret-detected-1"
		r := NewReporter(ciConfig(nil))

		rc, err := r.Mask(secret)

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		assert.Equal(t, []string{secret}, h.detected.masked)
		assert.NotContains(t, atmosio.MaskString("x"+secret+"y"), secret)
	})

	t.Run("registers the secret when ci is disabled but skips the provider", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		secret := "reporter-mask-secret-disabled-2"
		r := NewReporter(nil)

		rc, err := r.Mask(secret)

		require.NoError(t, err)
		assert.Equal(t, Receipt{Local: true}, rc)
		assert.Empty(t, h.detected.masked)
		assert.NotContains(t, atmosio.MaskString("x"+secret+"y"), secret)
	})

	t.Run("registers the secret when nothing is detected", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		secret := "reporter-mask-secret-undetected-3"
		r := NewReporter(ciConfig(nil))

		rc, err := r.Mask(secret)

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true}, rc)
		assert.Empty(t, h.local.masked)
		assert.NotContains(t, atmosio.MaskString("x"+secret+"y"), secret)
	})

	t.Run("detected provider without a masker still registers the secret", func(t *testing.T) {
		newHarness(t, kindPlain, kindNone)
		secret := "reporter-mask-secret-plain-4"

		rc, err := NewReporter(ciConfig(nil)).Mask(secret)

		require.NoError(t, err)
		assert.Equal(t, Receipt{Local: true}, rc)
		assert.NotContains(t, atmosio.MaskString("x"+secret+"y"), secret)
	})

	t.Run("empty value is a no-op", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)

		rc, err := NewReporter(ciConfig(nil)).Mask("")

		require.NoError(t, err)
		assert.Equal(t, Receipt{Local: true}, rc)
		assert.Empty(t, h.detected.masked)
	})
}

func TestReporter_Comment(t *testing.T) {
	ctx := context.Background()
	commentsOn := func(c *schema.CIConfig) { c.Comments.Enabled = boolPtr(true) }
	runCtx := &Context{
		RepoOwner:   "acme",
		RepoName:    "infra",
		SHA:         "abc123",
		RunURL:      "https://ci.example/run/1",
		PullRequest: &PRInfo{Number: 42},
	}

	t.Run("marker prepended and upsert by default", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		want := &Comment{ID: 7, URL: "u"}
		h.detectedPlain.EXPECT().PostComment(ctx, &PostCommentOptions{
			Owner:    "acme",
			Repo:     "infra",
			PRNumber: 42,
			Marker:   "<!-- atmos:ci:plan -->",
			Body:     "<!-- atmos:ci:plan -->\nhello",
			Behavior: CommentBehaviorUpsert,
		}).Return(want, nil)

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "hello", Key: "plan"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected", Comment: want}, rc)
	})

	t.Run("explicit behavior is honored", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, o *PostCommentOptions) (*Comment, error) {
				assert.Equal(t, CommentBehaviorUpdate, o.Behavior)
				return &Comment{}, nil
			},
		)

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Key: "k", Behavior: CommentBehaviorUpdate})
		require.NoError(t, err)
	})

	t.Run("invalid behavior is rejected before posting", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Key: "k", Behavior: "bogus"})
		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
	})

	t.Run("empty key creates without a marker", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		h.detectedPlain.EXPECT().PostComment(ctx, &PostCommentOptions{
			Owner:    "acme",
			Repo:     "infra",
			PRNumber: 42,
			Body:     "plain",
			Behavior: CommentBehaviorCreate,
		}).Return(&Comment{}, nil)

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "plain"})
		require.NoError(t, err)
	})

	t.Run("update without a key is an argument error", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "plain", Behavior: CommentBehaviorUpdate})

		require.ErrorIs(t, err, errUtils.ErrCICommentKeyRequired)
		assert.Nil(t, rc.Comment)
	})

	t.Run("update without a key is an argument error even when the write is gated", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)

		_, err := NewReporter(ciConfig(nil)).Comment(ctx, CommentRequest{Body: "plain", Behavior: CommentBehaviorUpdate})

		require.ErrorIs(t, err, errUtils.ErrCICommentKeyRequired)
	})

	t.Run("invalid target is rejected before posting", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Target: "issue"})

		require.ErrorIs(t, err, errUtils.ErrCICommentTargetInvalid)
	})

	t.Run("request PR overrides the context PR", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, o *PostCommentOptions) (*Comment, error) {
				assert.Equal(t, 99, o.PRNumber)
				return &Comment{}, nil
			},
		)

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", PR: 99})
		require.NoError(t, err)
	})

	t.Run("target pr without a PR is an error with a host-neutral hint", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra"})

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Target: CommentTargetPR})

		require.ErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
		assert.Nil(t, rc.Comment)
		hints := strings.Join(cockroachdb.GetAllHints(err), "\n")
		assert.Contains(t, hints, "pull request number")
		assert.NotContains(t, hints, "pr=123", "pr=123 is Starlark syntax and has no place in the host-neutral reporter")
	})

	t.Run("auto without a PR comments on the commit", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra", SHA: "abc123"})

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Key: "deploy"})

		require.NoError(t, err)
		require.Len(t, h.detected.commits, 1)
		assert.Equal(t, PostCommitCommentOptions{
			Owner: "acme", Repo: "infra", SHA: "abc123",
			Marker: "<!-- atmos:ci:deploy -->", Body: "<!-- atmos:ci:deploy -->\nb", Behavior: CommentBehaviorUpsert,
		}, h.detected.commits[0])
		require.NotNil(t, rc.Comment)
		assert.Equal(t, CommentTargetCommit, rc.Comment.Target)
		assert.False(t, rc.Local)
	})

	t.Run("auto with a PR comments on the pull request", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		h.ciCtx(runCtx)
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{ID: 1}, nil)

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Target: CommentTargetAuto})

		require.NoError(t, err)
		assert.Empty(t, h.detected.commits)
		require.NotNil(t, rc.Comment)
		assert.Equal(t, CommentTargetPR, rc.Comment.Target)
	})

	t.Run("target commit comments on the commit even when a PR is known", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		h.ciCtx(runCtx)

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Target: CommentTargetCommit})

		require.NoError(t, err)
		require.Len(t, h.detected.commits, 1)
		assert.Equal(t, "abc123", h.detected.commits[0].SHA)
		assert.Equal(t, CommentTargetCommit, rc.Comment.Target)
	})

	t.Run("a commit comment without a SHA is an error", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra"})

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b"})

		require.ErrorIs(t, err, errUtils.ErrCICommitUnknown)
		assert.Empty(t, h.detected.commits)
	})

	t.Run("a provider without commit comments falls back to local rendering", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindCapable)
		h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra", SHA: "abc123"})

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.True(t, rc.Local)
		assert.Len(t, h.local.commits, 1)
	})

	t.Run("a provider without commit comments and no local fallback is not supported", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra", SHA: "abc123"})

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b"})

		require.ErrorIs(t, err, errUtils.ErrCIOperationNotSupported)
	})

	t.Run("commit comment errors carry the sentinel", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindNone)
		h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra", SHA: "abc123"})
		boom := errors.New("api down")
		h.detected.commitErr = boom

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b"})

		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		require.ErrorIs(t, err, boom)
	})

	t.Run("local rendering accepts a missing PR", func(t *testing.T) {
		h := newHarness(t, kindNone, kindPlain)
		h.localPlain.EXPECT().Context().Return(&Context{}, nil).AnyTimes()
		want := &Comment{Body: "rendered"}
		h.localPlain.EXPECT().PostComment(ctx, &PostCommentOptions{
			Body:     "b",
			Behavior: CommentBehaviorCreate,
		}).Return(want, nil)

		rc, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Target: CommentTargetPR})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Comment: want}, rc)
	})

	t.Run("local rendering tolerates a missing context", func(t *testing.T) {
		h := newHarness(t, kindNone, kindPlain)
		h.localPlain.EXPECT().Context().Return(nil, errUtils.ErrCIProviderNotDetected).AnyTimes()
		h.localPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b", Target: CommentTargetPR})
		require.NoError(t, err)
	})

	t.Run("gated by configuration goes local", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		h.localPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		rc, err := NewReporter(ciConfig(nil)).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureComments, Comment: &Comment{Target: CommentTargetPR}}, rc)
	})

	t.Run("provider errors carry the sentinel", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		boom := errors.New("api down")
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(nil, boom)

		_, err := NewReporter(ciConfig(commentsOn)).Comment(ctx, CommentRequest{Body: "b"})

		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		require.ErrorIs(t, err, boom)
	})
}

func TestReporter_ForkGate(t *testing.T) {
	ctx := context.Background()
	// elevated is a fork pull request on an elevated event: the only case the gate holds.
	elevated := &Context{ElevatedEvent: true, RepoOwner: "acme", RepoName: "infra", SHA: "s", PullRequest: &PRInfo{Number: 1, Fork: true}}
	elevatedSameRepo := &Context{ElevatedEvent: true, RepoOwner: "acme", RepoName: "infra", SHA: "s", PullRequest: &PRInfo{Number: 1}}
	elevatedNoPR := &Context{ElevatedEvent: true, RepoOwner: "acme", RepoName: "infra", SHA: "s"}
	cfg := func(allow bool) *schema.AtmosConfiguration {
		c := ciConfig(func(c *schema.CIConfig) {
			c.Comments.Enabled = boolPtr(true)
			c.Checks.Enabled = boolPtr(true)
		})
		c.CI.AllowUnsafeForkExecution = allow
		return c
	}

	t.Run("comment for a same-repository pull request on an elevated event posts", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(elevatedSameRepo)
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		rc, err := NewReporter(cfg(false)).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.Equal(t, "detected", rc.Provider)
		assert.False(t, rc.Local)
		assert.Empty(t, rc.Gate)
	})

	t.Run("comment on an elevated event without a pull request in context posts", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(elevatedNoPR)
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		rc, err := NewReporter(cfg(false)).Comment(ctx, CommentRequest{Body: "b", PR: 7})

		require.NoError(t, err)
		assert.Equal(t, "detected", rc.Provider)
		assert.Empty(t, rc.Gate)
	})

	t.Run("check for a same-repository pull request on an elevated event posts", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(elevatedSameRepo)
		h.detectedPlain.EXPECT().CreateCheckRun(ctx, gomock.Any()).Return(&CheckRun{ID: 4}, nil)

		rc, err := NewReporter(cfg(false)).Check(ctx, CheckRequest{Name: "n"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected", Check: &CheckRun{ID: 4}}, rc)
	})

	t.Run("comment for a fork pull request on elevated event without opt-in goes local", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(elevated)
		h.localPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		rc, err := NewReporter(cfg(false)).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.Equal(t, "generic", rc.Provider)
		assert.True(t, rc.Local)
		assert.Equal(t, FeatureForkGate, rc.Gate)
	})

	t.Run("comment for a fork pull request on elevated event with opt-in goes to the detected provider", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(elevated)
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		rc, err := NewReporter(cfg(true)).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.Equal(t, "detected", rc.Provider)
		assert.False(t, rc.Local)
		assert.Empty(t, rc.Gate)
	})

	t.Run("non-elevated event is not gated", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra", PullRequest: &PRInfo{Number: 1}})
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		rc, err := NewReporter(cfg(false)).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.Equal(t, "detected", rc.Provider)
	})

	t.Run("check for a fork pull request on elevated event without opt-in goes local", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(elevated)
		h.localPlain.EXPECT().CreateCheckRun(ctx, gomock.Any()).Return(&CheckRun{ID: 5}, nil)

		rc, err := NewReporter(cfg(false)).Check(ctx, CheckRequest{Name: "n"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureForkGate, Check: &CheckRun{ID: 5}}, rc)
	})

	t.Run("update check on elevated event with opt-in goes to the detected provider", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(elevated)
		h.detectedPlain.EXPECT().UpdateCheckRun(ctx, gomock.Any()).Return(&CheckRun{ID: 6}, nil)

		rc, err := NewReporter(cfg(true)).UpdateCheck(ctx, CheckRequest{Name: "n", State: CheckRunStateSuccess})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected", Check: &CheckRun{ID: 6}}, rc)
	})
}

// TestReporter_ForkGateScope pins which writes the posting gate holds for a fork pull request on an
// elevated event. Env, path, and SARIF publish beyond the job, so they are held with comments and checks.
// Summary, output, annotations, groups, and masks stay inside the job and are not.
func TestReporter_ForkGateScope(t *testing.T) {
	ctx := context.Background()
	fork := &Context{ElevatedEvent: true, RepoOwner: "acme", RepoName: "infra", SHA: "s", PullRequest: &PRInfo{Number: 1, Fork: true}}
	annotation := Annotation{Path: "main.tf", StartLine: 1, Level: AnnotationError, Message: "m"}
	report := SARIFReport{Body: []byte("{}"), Category: "c"}
	cfg := func(allow bool) *schema.AtmosConfiguration {
		c := ciConfig(func(c *schema.CIConfig) {
			c.Results.Enabled = boolPtr(true)
			c.Comments.Enabled = boolPtr(true)
		})
		c.CI.AllowUnsafeForkExecution = allow
		return c
	}

	t.Run("env path and sarif are held", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(fork)
		r := NewReporter(cfg(false))

		rcEnv, err := r.Env("A", "1")
		require.NoError(t, err)
		rcPath, err := r.Path("/bin")
		require.NoError(t, err)
		rcSARIF, err := r.SARIF(ctx, report)
		require.NoError(t, err)

		for _, rc := range []Receipt{rcEnv, rcPath, rcSARIF} {
			assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureForkGate}, rc)
		}
		assert.Empty(t, h.detected.envs)
		assert.Empty(t, h.detected.paths)
		assert.Empty(t, h.detected.sarif)
		assert.Equal(t, [][2]string{{"A", "1"}}, h.local.envs)
		require.Len(t, h.local.sarif, 1)
		assert.Contains(t, h.local.sarif[0].SkipReason, "fork pull request")
	})

	t.Run("summary output annotate group and mask are not held", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(fork)
		r := NewReporter(cfg(false))

		rc, err := r.Annotate(annotation)
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		end, rc, err := r.Group("g")
		require.NoError(t, err)
		end()
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		rc, err = r.Summary("s")
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		rc, err = r.Output("k", "v")
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		secret := "fork-scope-mask-secret-77"
		rc, err = r.Mask(secret)
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)

		assert.Len(t, h.detected.annotations, 1)
		assert.Equal(t, []string{secret}, h.detected.masked)
		assert.Len(t, h.detectedWriter.summaries, 1)
	})

	t.Run("unsafe fork execution releases env path and sarif", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(fork)
		r := NewReporter(cfg(true))

		_, err := r.Env("A", "1")
		require.NoError(t, err)
		_, err = r.Path("/bin")
		require.NoError(t, err)
		_, err = r.SARIF(ctx, report)
		require.NoError(t, err)

		assert.Len(t, h.detected.envs, 1)
		assert.Len(t, h.detected.paths, 1)
		assert.Len(t, h.detected.sarif, 1)
	})

	t.Run("a context failure holds the write", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		h.detectedPlain.EXPECT().Context().Return(nil, errors.New("no context")).AnyTimes()
		r := NewReporter(cfg(false))

		rc, err := r.Env("A", "1")

		require.NoError(t, err)
		assert.Equal(t, FeatureForkGate, rc.Gate)
		assert.Empty(t, h.detected.envs)
	})

	t.Run("a fork pull request on a plain pull_request event is not gated", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(&Context{ElevatedEvent: false, RepoOwner: "acme", RepoName: "infra", SHA: "s", PullRequest: &PRInfo{Number: 1, Fork: true}})
		h.detectedPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)

		rc, err := NewReporter(cfg(false)).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.Equal(t, "detected", rc.Provider)
		assert.False(t, rc.Local)
		assert.Empty(t, rc.Gate)
	})

	t.Run("a commit comment for a fork pull request on an elevated event is held", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(fork)

		rc, err := NewReporter(cfg(false)).Comment(ctx, CommentRequest{Body: "b", Target: CommentTargetCommit})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureForkGate, Comment: rc.Comment}, rc)
		assert.Empty(t, h.detected.commits)
		assert.Len(t, h.local.commits, 1)
	})
}

func TestReporter_Check(t *testing.T) {
	ctx := context.Background()
	checksOn := func(c *schema.CIConfig) { c.Checks.Enabled = boolPtr(true) }
	runCtx := &Context{RepoOwner: "acme", RepoName: "infra", SHA: "abc123", RunURL: "https://ci.example/run/1"}

	t.Run("create defaults to pending and populates identity from context", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		want := &CheckRun{ID: 1, Name: "atmos/plan"}
		h.detectedPlain.EXPECT().CreateCheckRun(ctx, &CreateCheckRunOptions{
			Owner:      "acme",
			Repo:       "infra",
			SHA:        "abc123",
			Name:       "atmos/plan",
			Status:     CheckRunStatePending,
			Title:      "queued",
			DetailsURL: "https://ci.example/run/1",
		}).Return(want, nil)

		rc, err := NewReporter(ciConfig(checksOn)).Check(ctx, CheckRequest{Name: "atmos/plan", Description: "queued"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected", Check: want}, rc)
	})

	t.Run("create honors explicit state and URL", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		h.detectedPlain.EXPECT().CreateCheckRun(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, o *CreateCheckRunOptions) (*CheckRun, error) {
				assert.Equal(t, CheckRunStateInProgress, o.Status)
				assert.Equal(t, "https://other", o.DetailsURL)
				return &CheckRun{}, nil
			},
		)

		_, err := NewReporter(ciConfig(checksOn)).Check(ctx, CheckRequest{Name: "n", State: CheckRunStateInProgress, URL: "https://other"})
		require.NoError(t, err)
	})

	t.Run("update to a terminal state sets conclusion and completion time", func(t *testing.T) {
		tests := []struct {
			state      CheckRunState
			conclusion string
		}{
			{CheckRunStateSuccess, "success"},
			{CheckRunStateFailure, "failure"},
			{CheckRunStateError, "failure"},
			{CheckRunStateCancelled, "cancelled"},
		}
		for _, tc := range tests {
			t.Run(string(tc.state), func(t *testing.T) {
				h := newHarness(t, kindPlain, kindPlain)
				h.ciCtx(runCtx)
				h.detectedPlain.EXPECT().UpdateCheckRun(ctx, gomock.Any()).DoAndReturn(
					func(_ context.Context, o *UpdateCheckRunOptions) (*CheckRun, error) {
						assert.Equal(t, "acme", o.Owner)
						assert.Equal(t, "infra", o.Repo)
						assert.Equal(t, "abc123", o.SHA)
						assert.Equal(t, "n", o.Name)
						assert.Equal(t, tc.state, o.Status)
						assert.Equal(t, tc.conclusion, o.Conclusion)
						assert.Equal(t, "done", o.Title)
						assert.Equal(t, "https://ci.example/run/1", o.DetailsURL)
						require.NotNil(t, o.CompletedAt)
						assert.False(t, o.CompletedAt.IsZero())
						return &CheckRun{Status: o.Status}, nil
					},
				)

				rc, err := NewReporter(ciConfig(checksOn)).UpdateCheck(ctx, CheckRequest{Name: "n", State: tc.state, Description: "done"})

				require.NoError(t, err)
				assert.Equal(t, tc.state, rc.Check.Status)
			})
		}
	})

	t.Run("update to a non-terminal state has no conclusion", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		h.detectedPlain.EXPECT().UpdateCheckRun(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, o *UpdateCheckRunOptions) (*CheckRun, error) {
				assert.Equal(t, CheckRunStateInProgress, o.Status)
				assert.Empty(t, o.Conclusion)
				assert.Nil(t, o.CompletedAt)
				return &CheckRun{}, nil
			},
		)

		_, err := NewReporter(ciConfig(checksOn)).UpdateCheck(ctx, CheckRequest{Name: "n", State: CheckRunStateInProgress})
		require.NoError(t, err)
	})

	t.Run("disabled checks render locally with the gate recorded", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		h.localPlain.EXPECT().CreateCheckRun(ctx, gomock.Any()).Return(&CheckRun{ID: 3}, nil)

		rc, err := NewReporter(ciConfig(nil)).Check(ctx, CheckRequest{Name: "n"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureChecks, Check: &CheckRun{ID: 3, DetailsURL: runCtx.RunURL}}, rc)
	})

	t.Run("provider errors carry the sentinels", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		h.ciCtx(runCtx)
		boom := errors.New("api down")
		h.detectedPlain.EXPECT().CreateCheckRun(ctx, gomock.Any()).Return(nil, boom)
		h.detectedPlain.EXPECT().UpdateCheckRun(ctx, gomock.Any()).Return(nil, boom)
		r := NewReporter(ciConfig(checksOn))

		_, err := r.Check(ctx, CheckRequest{Name: "n"})
		require.ErrorIs(t, err, errUtils.ErrCICheckRunCreateFailed)
		require.ErrorIs(t, err, boom)

		_, err = r.UpdateCheck(ctx, CheckRequest{Name: "n", State: CheckRunStateSuccess})
		require.ErrorIs(t, err, errUtils.ErrCICheckRunUpdateFailed)
	})

	t.Run("detected context failure is returned", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		boom := errors.New("no context")
		h.detectedPlain.EXPECT().Context().Return(nil, boom).AnyTimes()

		_, err := NewReporter(ciConfig(checksOn)).Check(ctx, CheckRequest{Name: "n"})
		require.ErrorIs(t, err, boom)
	})
}

func TestReporter_WithOutput(t *testing.T) {
	t.Run("copies share the context cache", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindNone)
		want := &Context{RunID: "1"}
		h.detectedPlain.EXPECT().Context().Return(want, nil).Times(1)
		base := NewReporter(ciConfig(nil))
		bound := base.WithOutput(&bytes.Buffer{})

		first, err := base.Context()
		require.NoError(t, err)
		second, err := bound.Context()
		require.NoError(t, err)

		assert.Same(t, want, first)
		assert.Same(t, want, second)
	})

	t.Run("local renderings go to the bound writer", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		buf := &bytes.Buffer{}
		r := NewReporter(ciConfig(nil)).WithOutput(buf)

		_, err := r.Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.Same(t, buf, h.local.boundTo)
		assert.Len(t, h.local.annotations, 1)
	})

	t.Run("the original reporter stays unbound", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		base := NewReporter(ciConfig(nil))
		_ = base.WithOutput(&bytes.Buffer{})

		_, err := base.Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.Nil(t, h.local.boundTo)
	})

	t.Run("detected provider is not bound", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		r := NewReporter(ciConfig(nil)).WithOutput(&bytes.Buffer{})

		_, err := r.Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.Nil(t, h.detected.boundTo)
		assert.Len(t, h.detected.annotations, 1)
	})
}

func TestReporter_Context(t *testing.T) {
	t.Run("detected provider supplies the context", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		want := &Context{RunID: "d"}
		h.detectedPlain.EXPECT().Context().Return(want, nil)

		got, err := NewReporter(ciConfig(nil)).Context()

		require.NoError(t, err)
		assert.Same(t, want, got)
	})

	t.Run("fallback supplies the context when nothing is detected", func(t *testing.T) {
		h := newHarness(t, kindNone, kindPlain)
		want := &Context{RunID: "g"}
		h.localPlain.EXPECT().Context().Return(want, nil)

		got, err := NewReporter(ciConfig(nil)).Context()

		require.NoError(t, err)
		assert.Same(t, want, got)
	})

	t.Run("provider errors are cached and returned", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindNone)
		boom := errors.New("boom")
		h.detectedPlain.EXPECT().Context().Return(nil, boom).Times(1)
		r := NewReporter(ciConfig(nil))

		_, err := r.Context()
		require.ErrorIs(t, err, boom)
		_, err = r.Context()
		require.ErrorIs(t, err, boom)
	})
}

func TestReporter_Base(t *testing.T) {
	t.Run("detected provider resolves the base", func(t *testing.T) {
		h := newHarness(t, kindPlain, kindPlain)
		want := &BaseResolution{Ref: "main"}
		h.detectedPlain.EXPECT().ResolveBase().Return(want, nil)

		got, err := NewReporter(ciConfig(nil)).Base()

		require.NoError(t, err)
		assert.Same(t, want, got)
	})

	t.Run("fallback resolves the base when nothing is detected", func(t *testing.T) {
		h := newHarness(t, kindNone, kindPlain)
		boom := errors.New("no base")
		h.localPlain.EXPECT().ResolveBase().Return(nil, boom)

		_, err := NewReporter(nil).Base()
		require.ErrorIs(t, err, boom)
	})
}

func TestWrapErr(t *testing.T) {
	boom := errors.New("boom")

	assert.NoError(t, wrapErr(errUtils.ErrCIMaskFailed, nil))

	wrapped := wrapErr(errUtils.ErrCIMaskFailed, boom)
	require.ErrorIs(t, wrapped, errUtils.ErrCIMaskFailed)
	require.ErrorIs(t, wrapped, boom)

	already := wrapErr(errUtils.ErrCIMaskFailed, wrapped)
	assert.Equal(t, wrapped.Error(), already.Error())
}

func TestReporter_UpdateCheckPassesTheHandle(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, kindPlain, kindPlain)
	h.ciCtx(&Context{RepoOwner: "acme", RepoName: "infra", SHA: "abc123", RunURL: "https://ci.example/run/1"})
	r := NewReporter(ciConfig(func(c *schema.CIConfig) { c.Checks.Enabled = boolPtr(true) }))

	t.Run("sends the ID and fills the details URL", func(t *testing.T) {
		h.detectedPlain.EXPECT().UpdateCheckRun(ctx, gomock.Any()).DoAndReturn(
			func(_ context.Context, o *UpdateCheckRunOptions) (*CheckRun, error) {
				assert.EqualValues(t, 77, o.ID)
				return &CheckRun{ID: o.ID}, nil
			},
		)

		rc, err := r.UpdateCheck(ctx, CheckRequest{Name: "n", ID: 77, State: CheckRunStateSuccess})

		require.NoError(t, err)
		assert.EqualValues(t, 77, rc.Check.ID)
		assert.Equal(t, "https://ci.example/run/1", rc.Check.DetailsURL, "the receipt names where the check links to")
	})

	t.Run("a provider that reports the URL keeps it", func(t *testing.T) {
		h.detectedPlain.EXPECT().CreateCheckRun(ctx, gomock.Any()).Return(&CheckRun{ID: 1, DetailsURL: "https://provider.example/1"}, nil)

		rc, err := r.Check(ctx, CheckRequest{Name: "n"})

		require.NoError(t, err)
		assert.Equal(t, "https://provider.example/1", rc.Check.DetailsURL)
	})
}

// resetLogGroupDepth restores the process-wide log group slot after a test that holds it open.
func resetLogGroupDepth(t *testing.T) {
	t.Helper()
	t.Setenv(logGroupSentinelEnvVar, "")
	atomic.StoreInt32(&logGroupDepth, 0)
	t.Cleanup(func() { atomic.StoreInt32(&logGroupDepth, 0) })
}

func TestReporter_GroupNesting(t *testing.T) {
	t.Run("a nested group renders a plain heading instead of a nested marker", func(t *testing.T) {
		resetLogGroupDepth(t)
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(&Context{})
		r := NewReporter(ciConfig(nil))

		endOuter, rcOuter, err := r.Group("outer")
		require.NoError(t, err)
		endInner, rcInner, err := r.Group("inner")
		require.NoError(t, err)

		assert.Equal(t, Receipt{Provider: "detected"}, rcOuter)
		assert.Equal(t, Receipt{Provider: "generic", Local: true}, rcInner)
		assert.Equal(t, []string{"outer"}, h.detected.groups, "only the outermost group emits provider markers")
		assert.Equal(t, []string{"inner"}, h.local.groups, "the inner title is still shown")

		endInner()
		assert.Zero(t, h.detected.ends, "closing the plain heading must not close the outer group")
		endOuter()
		assert.Equal(t, 1, h.detected.ends)
	})

	t.Run("closing releases the slot and double close is harmless", func(t *testing.T) {
		resetLogGroupDepth(t)
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(&Context{})
		r := NewReporter(ciConfig(nil))

		end, _, err := r.Group("first")
		require.NoError(t, err)
		end()
		end()
		_, rc, err := r.Group("second")
		require.NoError(t, err)

		assert.Equal(t, []string{"first", "second"}, h.detected.groups)
		assert.False(t, rc.Local)
		assert.Equal(t, 1, h.detected.ends)
	})

	t.Run("a group inside a process that a parent group already covers is plain", func(t *testing.T) {
		resetLogGroupDepth(t)
		h := newHarness(t, kindCapable, kindCapable)
		h.ciCtx(&Context{})
		// Set after newHarness, which blanks the sentinel so the ambient CI job cannot leak one in.
		t.Setenv(logGroupSentinelEnvVar, "1")

		end, rc, err := NewReporter(ciConfig(nil)).Group("child")
		require.NoError(t, err)
		end()

		assert.True(t, rc.Local)
		assert.Empty(t, h.detected.groups)
		assert.Equal(t, []string{"child"}, h.local.groups)
	})

	t.Run("a failed start releases the slot", func(t *testing.T) {
		resetLogGroupDepth(t)
		h := newHarness(t, kindCapable, kindNone)
		h.ciCtx(&Context{})
		h.detected.err = errors.New("boom")
		r := NewReporter(ciConfig(nil))

		_, _, err := r.Group("g")
		require.Error(t, err)
		h.detected.err = nil
		_, rc, err := r.Group("again")

		require.NoError(t, err)
		assert.False(t, rc.Local)
	})
}

func TestNewReporter_ForcedCIMode(t *testing.T) {
	ctx := context.Background()

	t.Run("ATMOS_CI makes the generic provider the detected one", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		t.Setenv("ATMOS_CI", "true")
		h.localPlain.EXPECT().Context().Return(&Context{Provider: "generic"}, nil).AnyTimes()
		r := NewReporter(ciConfig(nil))

		rc, err := r.Annotate(Annotation{Message: "m"})
		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic"}, rc, "a forced CI mode is not the pure-local mode: Local is false")

		c, err := r.Context()
		require.NoError(t, err)
		assert.Equal(t, "generic", c.Provider)
		assert.Len(t, h.local.annotations, 1)
	})

	t.Run("CI makes the generic provider the detected one", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		t.Setenv("CI", "true")

		rc, err := NewReporter(ciConfig(nil)).Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.False(t, rc.Local)
		assert.Len(t, h.local.annotations, 1)
	})

	for _, falsy := range []string{"false", "0", "no", ""} {
		t.Run("ATMOS_CI="+falsy+" stays local", func(t *testing.T) {
			newHarness(t, kindNone, kindCapable)
			t.Setenv("ATMOS_CI", falsy)

			rc, err := NewReporter(ciConfig(nil)).Annotate(Annotation{Message: "m"})

			require.NoError(t, err)
			assert.True(t, rc.Local)
		})
	}

	t.Run("gates apply in forced CI mode", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		t.Setenv("ATMOS_CI", "true")
		h.ciCtx(&Context{})
		disabled := ciConfig(func(c *schema.CIConfig) { c.Annotations.Enabled = boolPtr(false) })

		rc, err := NewReporter(disabled).Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "generic", Local: true, Gate: FeatureAnnotations}, rc)
	})

	t.Run("the master switch is named when it is off", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		t.Setenv("ATMOS_CI", "true")
		h.ciCtx(&Context{})

		rc, err := NewReporter(&schema.AtmosConfiguration{}).Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.Equal(t, FeatureEnabled, rc.Gate)
	})

	t.Run("a detected provider wins over forced mode", func(t *testing.T) {
		h := newHarness(t, kindCapable, kindCapable)
		t.Setenv("ATMOS_CI", "true")

		rc, err := NewReporter(ciConfig(nil)).Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.Equal(t, Receipt{Provider: "detected"}, rc)
		assert.Len(t, h.detected.annotations, 1)
	})

	t.Run("forced mode binds renderings to WithOutput", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		t.Setenv("ATMOS_CI", "true")
		buf := &bytes.Buffer{}

		_, err := NewReporter(ciConfig(nil)).WithOutput(buf).Annotate(Annotation{Message: "m"})

		require.NoError(t, err)
		assert.Same(t, buf, h.local.boundTo)
	})

	t.Run("forced mode posts comments through the generic provider", func(t *testing.T) {
		h := newHarness(t, kindNone, kindCapable)
		t.Setenv("ATMOS_CI", "true")
		h.localPlain.EXPECT().Context().Return(&Context{RepoOwner: "acme", RepoName: "infra", SHA: "abc123"}, nil).AnyTimes()
		cfg := ciConfig(func(c *schema.CIConfig) { c.Comments.Enabled = boolPtr(true) })

		rc, err := NewReporter(cfg).Comment(ctx, CommentRequest{Body: "b"})

		require.NoError(t, err)
		assert.False(t, rc.Local)
		assert.Len(t, h.local.commits, 1)
	})
}

func TestNewReporterForProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("uses the given provider and the preseeded context", func(t *testing.T) {
		h := newHarness(t, kindNone, kindPlain)
		given := NewMockProvider(gomock.NewController(t))
		given.EXPECT().Name().Return("given").AnyTimes()
		want := &Context{RepoOwner: "acme", RepoName: "infra", PullRequest: &PRInfo{Number: 3}}
		given.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{ID: 5}, nil)
		_ = h

		rc, err := NewReporterForProvider(ciConfig(func(c *schema.CIConfig) { c.Comments.Enabled = boolPtr(true) }), given, want).
			Comment(ctx, CommentRequest{Body: "b", Key: "plan:vpc:dev"})

		require.NoError(t, err)
		assert.Equal(t, "given", rc.Provider)
		assert.False(t, rc.Local)
		assert.EqualValues(t, 5, rc.Comment.ID)
	})

	t.Run("holds a fork pull request on an elevated event", func(t *testing.T) {
		h := newHarness(t, kindNone, kindPlain)
		given := NewMockProvider(gomock.NewController(t))
		given.EXPECT().Name().Return("given").AnyTimes()
		h.localPlain.EXPECT().PostComment(ctx, gomock.Any()).Return(&Comment{}, nil)
		forkCtx := &Context{ElevatedEvent: true, RepoOwner: "acme", RepoName: "infra", PullRequest: &PRInfo{Number: 3, Fork: true}}

		rc, err := NewReporterForProvider(ciConfig(func(c *schema.CIConfig) { c.Comments.Enabled = boolPtr(true) }), given, forkCtx).
			Comment(ctx, CommentRequest{Body: "b", Target: CommentTargetPR})

		require.NoError(t, err)
		assert.Equal(t, FeatureForkGate, rc.Gate)
	})

	t.Run("reads the context from the provider when none is given", func(t *testing.T) {
		newHarness(t, kindNone, kindNone)
		given := NewMockProvider(gomock.NewController(t))
		given.EXPECT().Name().Return("given").AnyTimes()
		want := &Context{RunID: "9"}
		given.EXPECT().Context().Return(want, nil)

		got, err := NewReporterForProvider(nil, given, nil).Context()

		require.NoError(t, err)
		assert.Same(t, want, got)
	})
}

func TestReceipt_Gated(t *testing.T) {
	assert.False(t, Receipt{}.Gated())
	assert.False(t, Receipt{Local: true}.Gated())
	assert.True(t, Receipt{Local: true, Gate: FeatureChecks}.Gated())
}
