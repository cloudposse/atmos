package errors

import (
	goerrors "errors"
	"fmt"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deepCauseWithHint builds a hint- and explanation-bearing error the way providers do.
func deepCauseWithHint() error {
	return Build(errors.New("device flow unavailable")).
		WithExplanation("interactive terminal required").
		WithHint("run aws sso login first").
		Err()
}

// TestFormat_HintSurvivesMultiCauseWrap is the reproduction for hints being dropped when a
// layer wraps a hint-bearing cause with more than one %w (a multi-cause Unwrap() []error).
func TestFormat_HintSurvivesMultiCauseWrap(t *testing.T) {
	wrapped := fmt.Errorf("%w: identity %q: %w", ErrAuthenticationFailed, "dev", deepCauseWithHint())

	require.ErrorIs(t, wrapped, ErrAuthenticationFailed)

	out := stripANSI(Format(wrapped, DefaultFormatterConfig()))
	assert.Contains(t, out, "run aws sso login first")
	assert.Contains(t, out, "interactive terminal required")
}

func TestAllHints_DescendsIntoMultiCauseErrors(t *testing.T) {
	first := Build(errors.New("first")).WithHint("hint one").WithHint("shared").Err()
	second := Build(errors.New("second")).WithHint("shared").WithHint("hint two").Err()

	tests := []struct {
		name string
		err  error
		want []string
	}{
		{name: "nil", err: nil, want: nil},
		{name: "plain error", err: errors.New("plain"), want: nil},
		{name: "single cause chain", err: fmt.Errorf("ctx: %w", first), want: errors.GetAllHints(first)},
		{name: "errors.Join", err: goerrors.Join(first, second), want: []string{"hint one", "shared", "hint two"}},
		{name: "double %w", err: fmt.Errorf("%w: %w", first, second), want: []string{"hint one", "shared", "hint two"}},
		{
			name: "nested multi-cause inside a single-cause chain",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("%w: %w", ErrAuthenticationFailed, goerrors.Join(first, second))),
			want: []string{"hint one", "shared", "hint two"},
		},
		{
			name: "outer hint comes first",
			err:  errors.WithHint(goerrors.Join(first, second), "outer hint"),
			want: []string{"outer hint", "hint one", "shared", "hint two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AllHints(tt.err))
		})
	}
}

func TestAllDetails_DescendsIntoMultiCauseErrors(t *testing.T) {
	first := Build(errors.New("first")).WithExplanation("detail one").Err()
	second := Build(errors.New("second")).WithExplanation("detail two").Err()

	got := AllDetails(fmt.Errorf("%w: %w", first, second))
	require.Len(t, got, 2)
	assert.Equal(t, "detail one", got[0])
	assert.Equal(t, "detail two", got[1])

	// The same detail reached through two branches is reported once.
	assert.Len(t, AllDetails(goerrors.Join(first, first)), 1)
	assert.Empty(t, AllDetails(nil))
}

func TestWithCause_PreservesHintsAcrossMultiCauseCause(t *testing.T) {
	cause := fmt.Errorf("%w: %w", ErrAuthenticationFailed, deepCauseWithHint())

	err := Build(ErrInvalidAuthConfig).WithCause(cause).Err()

	assert.Contains(t, errors.GetAllHints(err), "run aws sso login first")
}
