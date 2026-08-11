package ci

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
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
