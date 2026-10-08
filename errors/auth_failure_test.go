package errors

import (
	goerrors "errors"
	"fmt"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapAuthenticationFailed(t *testing.T) {
	cause := goerrors.New("exit status 1")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "plain cause",
			err:  WrapAuthenticationFailed(cause, "identity %q", "cp"),
			want: `authentication failed for identity "cp": exit status 1`,
		},
		{
			name: "cause that repeats the sentinel is not stuttered",
			err:  WrapAuthenticationFailed(fmt.Errorf("%w: provider=sso: %w", ErrAuthenticationFailed, cause), "identity %q", "dev"),
			want: `authentication failed for identity "dev": provider=sso: exit status 1`,
		},
		{
			name: "bare sentinel cause leaves only the scope",
			err:  WrapAuthenticationFailed(ErrAuthenticationFailed, "identity %q", "dev"),
			want: `authentication failed for identity "dev"`,
		},
		{
			name: "scopes accumulate outermost first",
			err:  WrapAuthenticationFailed(WrapAuthenticationFailed(cause, "provider %q", "sso"), "identity %q", "dev"),
			want: `authentication failed for identity "dev" via provider "sso": exit status 1`,
		},
		{
			name: "duplicate scopes are collapsed",
			err:  WrapAuthenticationFailed(WrapAuthenticationFailed(cause, "identity %q", "dev"), "identity %q", "dev"),
			want: `authentication failed for identity "dev": exit status 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)
			assert.Equal(t, tt.want, tt.err.Error())
			assert.ErrorIs(t, tt.err, ErrAuthenticationFailed)
		})
	}
}

func TestWrapAuthenticationFailed_Nil(t *testing.T) {
	assert.NoError(t, WrapAuthenticationFailed(nil, "identity %q", "dev"))
}

func TestWrapAuthenticationFailed_KeepsCauseMatchable(t *testing.T) {
	cause := goerrors.New("root cause")
	err := WrapAuthenticationFailed(fmt.Errorf("context: %w", cause), "identity %q", "dev")

	assert.ErrorIs(t, err, cause)
	assert.NotErrorIs(t, err, ErrInvalidAuthConfig, "must not match unrelated sentinels")
}

func TestWrapAuthenticationFailed_KeepsHintsAndDetailsReachable(t *testing.T) {
	cause := Build(ErrAuthenticationFailed).
		WithExplanation("interactive terminal required").
		WithHint("run aws sso login first").
		Err()

	err := WrapAuthenticationFailed(WrapAuthenticationFailed(cause, "provider %q", "sso"), "identity %q", "dev")

	assert.Equal(t, `authentication failed for identity "dev" via provider "sso"`, err.Error())
	assert.Equal(t, []string{"run aws sso login first"}, errors.GetAllHints(err), "reachable without multi-cause traversal")
	assert.Equal(t, []string{"interactive terminal required"}, AllDetails(err))
}

func TestMarkAs(t *testing.T) {
	base := goerrors.New("already descriptive message")

	marked := MarkAs(base, ErrIdentityAuthFailed)

	assert.Equal(t, base.Error(), marked.Error(), "message is unchanged")
	assert.ErrorIs(t, marked, ErrIdentityAuthFailed)
	assert.ErrorIs(t, marked, base)
	assert.NotErrorIs(t, marked, ErrAuthenticationFailed)
	assert.NoError(t, MarkAs(nil, ErrIdentityAuthFailed))
}

func TestEnsureAuthenticationFailed(t *testing.T) {
	plain := goerrors.New("provider not found")

	t.Run("adds the sentinel once to a plain error", func(t *testing.T) {
		err := EnsureAuthenticationFailed(plain)
		assert.ErrorIs(t, err, ErrAuthenticationFailed)
		assert.ErrorIs(t, err, plain)
		assert.Equal(t, "authentication failed: provider not found", err.Error())
	})

	t.Run("returns an existing authentication failure unchanged", func(t *testing.T) {
		existing := WrapAuthenticationFailed(plain, "identity %q", "dev")
		assert.Same(t, existing, EnsureAuthenticationFailed(existing))
	})

	t.Run("nil stays nil", func(t *testing.T) {
		assert.NoError(t, EnsureAuthenticationFailed(nil))
	})
}

func TestWrapAuthenticationFailed_EmptyScopeOmitsFor(t *testing.T) {
	err := WrapAuthenticationFailed(goerrors.New("boom"), "")
	assert.Equal(t, "authentication failed: boom", err.Error())
}

func TestWrapIdentityAuthFailed(t *testing.T) {
	plain := goerrors.New("keyring locked")

	t.Run("prefixes a plain error with the identity", func(t *testing.T) {
		err := WrapIdentityAuthFailed("dev", plain)
		assert.ErrorIs(t, err, ErrIdentityAuthFailed)
		assert.ErrorIs(t, err, plain)
		assert.Equal(t, "failed to authenticate identity 'dev': keyring locked", err.Error())
	})

	t.Run("keeps an existing authentication failure message", func(t *testing.T) {
		failure := WrapAuthenticationFailed(plain, "identity %q", "dev")
		err := WrapIdentityAuthFailed("dev", failure)
		assert.ErrorIs(t, err, ErrIdentityAuthFailed)
		assert.ErrorIs(t, err, ErrAuthenticationFailed)
		assert.Equal(t, failure.Error(), err.Error())
	})

	t.Run("nil stays nil", func(t *testing.T) {
		assert.NoError(t, WrapIdentityAuthFailed("dev", nil))
	})
}

// TestWrapAuthenticationFailed_SeparatorsAndSentinels covers compositions where an inner layer joined
// the sentinel with errors.Join (newline separated) or a multi-sentinel message, which previously
// produced "authentication failed interactive ..." (no separator) and a repeated sentinel clause.
func TestWrapAuthenticationFailed_SeparatorsAndSentinels(t *testing.T) {
	promptUnavailable := Build(ErrAuthPromptUnavailable).
		WithExplanation("Identity \"ft-user\" requires an MFA token").
		WithHint("Run `atmos auth login --identity=ft-user` in a terminal").
		Err()
	joined := goerrors.Join(ErrAuthenticationFailed, promptUnavailable)
	cause := goerrors.New("exit status 1")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "joined sentinel then prompt-unavailable cause",
			err:  WrapAuthenticationFailed(joined, "identity %q", "ft-user"),
			want: `authentication failed for identity "ft-user": interactive authentication prompt unavailable: requires a TTY`,
		},
		{
			name: "chained identity scopes keep one sentinel and a separator",
			err: WrapAuthenticationFailed(
				WrapAuthenticationFailed(joined, "identity %q", "ft-user"), "identity %q", "ft-role",
			),
			want: `authentication failed for identity "ft-role" via identity "ft-user": interactive authentication prompt unavailable: requires a TTY`,
		},
		{
			name: "ensure on a joined sentinel does not repeat it",
			err:  EnsureAuthenticationFailed(WrapAuthenticationFailed(joined, "")),
			want: `authentication failed: interactive authentication prompt unavailable: requires a TTY`,
		},
		{
			name: "sentinel in the middle of a joined message is dropped",
			err:  WrapAuthenticationFailed(goerrors.Join(cause, ErrAuthenticationFailed), "identity %q", "dev"),
			want: `authentication failed for identity "dev": exit status 1`,
		},
		{
			name: "MarkAs does not change the message nor add a separator",
			err:  WrapAuthenticationFailed(MarkAs(joined, ErrIdentityAuthFailed), "identity %q", "dev"),
			want: `authentication failed for identity "dev": interactive authentication prompt unavailable: requires a TTY`,
		},
		{
			name: "marked sentinel text with a colon separator",
			err: WrapAuthenticationFailed(
				MarkAs(fmt.Errorf("%w: %w", ErrAuthenticationFailed, ErrAuthPromptUnavailable), ErrIdentityAuthFailed),
				"identity %q", "dev",
			),
			want: `authentication failed for identity "dev": interactive authentication prompt unavailable: requires a TTY`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)
			assert.Equal(t, tt.want, tt.err.Error())
			assert.NotContains(t, tt.err.Error(), "\n")
			assert.ErrorIs(t, tt.err, ErrAuthenticationFailed)
		})
	}

	t.Run("sentinels stay matchable", func(t *testing.T) {
		err := WrapAuthenticationFailed(
			WrapAuthenticationFailed(joined, "identity %q", "ft-user"), "identity %q", "ft-role",
		)
		assert.ErrorIs(t, err, ErrAuthenticationFailed)
		assert.ErrorIs(t, err, ErrAuthPromptUnavailable)
		assert.ErrorIs(t, err, ErrTTYRequired)
	})
}
