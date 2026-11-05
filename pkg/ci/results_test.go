package ci

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
)

// reportingMockProvider is a detected provider that implements both the
// Annotator and SARIFReporter capabilities, recording what it was handed.
type reportingMockProvider struct {
	mockProvider
	annotations []provider.Annotation
	sarif       []provider.SARIFReport
	annotateErr error
	sarifErr    error
}

func (m *reportingMockProvider) Annotate(a []provider.Annotation) error {
	m.annotations = append(m.annotations, a...)
	return m.annotateErr
}

func (m *reportingMockProvider) ReportSARIF(_ context.Context, r provider.SARIFReport) error {
	m.sarif = append(m.sarif, r)
	return m.sarifErr
}

func TestAnnotate(t *testing.T) {
	t.Run("dispatches to a capable provider", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)

		require.NoError(t, Annotate([]Annotation{{Path: "a.tf", StartLine: 1, Message: "x"}}))
		require.Len(t, m.annotations, 1)
		assert.Equal(t, "a.tf", m.annotations[0].Path)
	})

	t.Run("no-op when no provider detected", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		assert.NoError(t, Annotate([]Annotation{{Path: "a.tf", Message: "x"}}))
	})

	t.Run("no-op when provider lacks the capability", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		Register(&mockProvider{name: "plain", detected: true}) // no Annotate method
		assert.NoError(t, Annotate([]Annotation{{Path: "a.tf", Message: "x"}}))
	})

	t.Run("no-op on empty input", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)
		require.NoError(t, Annotate(nil))
		assert.Empty(t, m.annotations)
	})

	t.Run("wraps provider failures with annotation sentinel", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		providerErr := errors.New("provider annotation failed")
		m := &reportingMockProvider{
			mockProvider: mockProvider{name: "cap", detected: true},
			annotateErr:  providerErr,
		}
		Register(m)

		err := Annotate([]Annotation{{Path: "a.tf", StartLine: 1, Message: "x"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCIAnnotationFailed)
		assert.ErrorIs(t, err, providerErr)
	})
}

func TestReportSARIF(t *testing.T) {
	t.Run("dispatches to a capable provider", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)

		require.NoError(t, ReportSARIF(context.Background(), SARIFReport{Body: []byte(`{}`), Category: "c"}))
		require.Len(t, m.sarif, 1)
		assert.Equal(t, "c", m.sarif[0].Category)
	})

	t.Run("no-op when no provider detected", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		assert.NoError(t, ReportSARIF(context.Background(), SARIFReport{Body: []byte(`{}`)}))
	})

	t.Run("no-op when provider lacks the capability", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		Register(&mockProvider{name: "plain", detected: true})
		assert.NoError(t, ReportSARIF(context.Background(), SARIFReport{Body: []byte(`{}`)}))
	})

	t.Run("no-op on empty body", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)
		require.NoError(t, ReportSARIF(context.Background(), SARIFReport{Category: "c"}))
		assert.Empty(t, m.sarif)
	})

	t.Run("wraps provider failures with SARIF upload sentinel", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		providerErr := errors.New("provider SARIF upload failed")
		m := &reportingMockProvider{
			mockProvider: mockProvider{name: "cap", detected: true},
			sarifErr:     providerErr,
		}
		Register(m)

		err := ReportSARIF(context.Background(), SARIFReport{Body: []byte(`{}`), Category: "c"})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCISARIFUploadFailed)
		assert.ErrorIs(t, err, providerErr)
	})
}

func TestAnnotatePolicyDenial(t *testing.T) {
	denial := policy.EnforceDecision(policy.Decision{Status: policy.Deny, Findings: []policy.Finding{
		{RuleID: "function-disable-exec/guard", Status: policy.Deny, Effects: policy.Effects{
			Deny: true, Reason: "**`!exec` is disabled** because stack configuration must not run shell commands.",
		}},
		{RuleID: "no-destroy/guard", Status: policy.Deny, Effects: policy.Effects{Deny: true}},
	}})

	t.Run("one error annotation per denying rule", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)

		require.NoError(t, AnnotatePolicyDenial(fmt.Errorf("wrapped: %w", denial), "terraform destroy"))
		require.Len(t, m.annotations, 2)

		first := m.annotations[0]
		assert.Equal(t, AnnotationError, first.Level)
		assert.Equal(t, "Policy function-disable-exec/guard denied terraform destroy", first.Title)
		assert.Equal(t, "!exec is disabled because stack configuration must not run shell commands.", first.Message,
			"Markdown reasons must render as plain text, without dangling emphasis markers")
		assert.Empty(t, first.Path, "command-level denials have no file location")
		assert.Zero(t, first.StartLine)

		last := m.annotations[1]
		assert.Equal(t, "Policy no-destroy/guard denied terraform destroy", last.Title)
		assert.Equal(t, "Denied by no-destroy/guard.", last.Message, "a reason-less rule must still be named")
	})

	t.Run("title without a subject", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)

		require.NoError(t, AnnotatePolicyDenial(denial, ""))
		require.Len(t, m.annotations, 2)
		assert.Equal(t, "Policy no-destroy/guard denied execution", m.annotations[1].Title)
	})

	t.Run("no-op for errors that are not denials", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)

		require.NoError(t, AnnotatePolicyDenial(nil, "terraform plan"))
		require.NoError(t, AnnotatePolicyDenial(errUtils.ErrPolicyEvaluation, "terraform plan"))
		require.NoError(t, AnnotatePolicyDenial(errors.New("boom"), "terraform plan"))
		assert.Empty(t, m.annotations)
	})

	t.Run("no-op outside CI", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		assert.NoError(t, AnnotatePolicyDenial(denial, "terraform destroy"))
	})

	t.Run("annotator failure is reported", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		Register(&reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}, annotateErr: errors.New("write failed")})

		require.ErrorIs(t, AnnotatePolicyDenial(denial, "terraform destroy"), errUtils.ErrCIAnnotationFailed)
	})
}

func TestAnnotateFailure(t *testing.T) {
	enabled := true
	disabled := false
	capture := func(t *testing.T) *reportingMockProvider {
		t.Helper()
		restore := SwapRegistryForTest()
		t.Cleanup(restore)
		m := &reportingMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)
		return m
	}

	t.Run("generic failure uses title, message, explanation, and location", func(t *testing.T) {
		m := capture(t)
		err := errUtils.Build(errUtils.ErrInvalidStack).
			WithTitle("Stack Error").
			WithExplanation("The `vars` section must be a **map**.").
			WithLocation("stacks/deploy/ue2-dev.yaml", 42).
			Err()

		require.NoError(t, AnnotateFailure(nil, fmt.Errorf("describe: %w", err)))
		require.Len(t, m.annotations, 1)
		got := m.annotations[0]
		assert.Equal(t, AnnotationError, got.Level)
		assert.Equal(t, "Stack Error", got.Title)
		assert.Equal(t, "stacks/deploy/ue2-dev.yaml", got.Path)
		assert.Equal(t, 42, got.StartLine)
		assert.True(t, strings.HasPrefix(got.Message, "describe: "), got.Message)
		assert.Contains(t, got.Message, "The vars section must be a map.")
	})

	t.Run("untitled failure gets the default title and no location", func(t *testing.T) {
		m := capture(t)
		require.NoError(t, AnnotateFailure(nil, errors.New("boom")))
		require.Len(t, m.annotations, 1)
		assert.Equal(t, "Atmos command failed", m.annotations[0].Title)
		assert.Equal(t, "boom", m.annotations[0].Message)
		assert.Empty(t, m.annotations[0].Path)
	})

	t.Run("policy denial annotates each rule at the recorded location", func(t *testing.T) {
		m := capture(t)
		denial := policy.EnforceDecision(policy.Decision{Status: policy.Deny, Findings: []policy.Finding{
			{RuleID: "protect-eip/guard", Status: policy.Deny, Effects: policy.Effects{Deny: true, Reason: "EIP cannot be destroyed."}},
			{RuleID: "no-destroy/guard", Status: policy.Deny, Effects: policy.Effects{Deny: true}},
		}})
		located := errUtils.Build(denial).WithLocation("stacks/deploy/ue2-dev.yaml", 7).Err()

		require.NoError(t, AnnotateFailure(nil, located))
		require.Len(t, m.annotations, 2)
		assert.Equal(t, "Policy protect-eip/guard denied execution", m.annotations[0].Title)
		assert.Equal(t, "EIP cannot be destroyed.", m.annotations[0].Message)
		assert.Equal(t, "stacks/deploy/ue2-dev.yaml", m.annotations[0].Path)
		assert.Equal(t, 7, m.annotations[0].StartLine)
		assert.Equal(t, "Denied by no-destroy/guard.", m.annotations[1].Message)
		assert.Equal(t, 7, m.annotations[1].StartLine)
	})

	t.Run("skips errors that are not command failures", func(t *testing.T) {
		m := capture(t)
		nested := cockroach.Mark(errors.New("atmos terraform destroy exited with code 77"), errUtils.ErrNestedAtmosFailed)
		for name, err := range map[string]error{
			"nil":       nil,
			"nested":    fmt.Errorf("script: %w", nested),
			"silent":    errUtils.ExitCodeError{Code: 3, Silent: true},
			"success":   errUtils.ExitCodeError{Code: 0},
			"interrupt": errUtils.WithExitCode(errors.New("interrupted"), errUtils.ExitCodeSIGINT),
		} {
			require.NoError(t, AnnotateFailure(nil, err), name)
		}
		assert.Empty(t, m.annotations)
	})

	t.Run("loaded configuration gates annotations", func(t *testing.T) {
		m := capture(t)
		off := &schema.AtmosConfiguration{Initialized: true}
		off.CI.Enabled = true
		off.CI.Annotations.Enabled = &disabled
		require.NoError(t, AnnotateFailure(off, errors.New("boom")))

		ciOff := &schema.AtmosConfiguration{Initialized: true}
		require.NoError(t, AnnotateFailure(ciOff, errors.New("boom")))
		assert.Empty(t, m.annotations, "loaded config with annotations or CI disabled must not annotate")

		on := &schema.AtmosConfiguration{Initialized: true}
		on.CI.Enabled = true
		on.CI.Annotations.Enabled = &enabled
		require.NoError(t, AnnotateFailure(on, errors.New("boom")))

		notLoaded := &schema.AtmosConfiguration{}
		require.NoError(t, AnnotateFailure(notLoaded, errors.New("early")))
		require.Len(t, m.annotations, 2, "enabled config and not-yet-loaded config both annotate")
		assert.Equal(t, "early", m.annotations[1].Message)
	})
}
