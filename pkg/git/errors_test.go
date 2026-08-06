package git

import (
	"context"
	"errors"
	"io"
	"testing"

	cockroacherrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSwappableProvider is a minimal Provider that also implements
// StderrSwapper, so tests can assert CaptureStderr routes writes through the
// swapped writer.
type stubSwappableProvider struct {
	writeOnSwap string
}

func (s *stubSwappableProvider) SwapStderr(w io.Writer) func() {
	if s.writeOnSwap != "" {
		_, _ = w.Write([]byte(s.writeOnSwap))
	}
	return func() {}
}

func (s *stubSwappableProvider) Init(context.Context, *InitOptions) error   { return nil }
func (s *stubSwappableProvider) Clone(context.Context, *CloneOptions) error { return nil }
func (s *stubSwappableProvider) Pull(context.Context, *PullOptions) error   { return nil }
func (s *stubSwappableProvider) Push(context.Context, *PushOptions) error   { return nil }
func (s *stubSwappableProvider) Status(context.Context, *StatusOptions) (*StatusResult, error) {
	return &StatusResult{}, nil
}

func (s *stubSwappableProvider) Diff(context.Context, *DiffOptions) (*DiffResult, error) {
	return &DiffResult{}, nil
}

func (s *stubSwappableProvider) Commit(context.Context, *CommitOptions) (*CommitResult, error) {
	return &CommitResult{}, nil
}

// stubNonSwappableProvider implements Provider but not StderrSwapper, mirroring
// test doubles elsewhere in the codebase that don't need stderr capture.
type stubNonSwappableProvider struct{ stubSwappableProvider }

func TestCaptureStderr_SwappableProvider(t *testing.T) {
	provider := &stubSwappableProvider{writeOnSwap: "fatal: something went wrong\n"}

	stderr, err := CaptureStderr(provider, func() error {
		return errors.New("boom")
	})

	require.Error(t, err)
	assert.Equal(t, "fatal: something went wrong", stderr, "captured stderr must be trimmed")
}

func TestCaptureStderr_NonSwappableProviderRunsUnmodified(t *testing.T) {
	var nonSwappable Provider = &struct{ stubNonSwappableProvider }{}

	ran := false
	stderr, err := CaptureStderr(nonSwappable, func() error {
		ran = true
		return nil
	})

	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.True(t, ran, "operation must still run when the provider doesn't support stderr capture")
}

func TestWrapOperationError_NilErrorReturnsNil(t *testing.T) {
	assert.NoError(t, WrapOperationError("do something", "/workdir", "", nil, "a hint"))
}

func TestWrapOperationError_EmbedsWorkdirStderrAndHint(t *testing.T) {
	base := errors.New("git command exited with non-zero status")

	err := WrapOperationError("clone Git repository", "/tmp/workdir", "fatal: remote branch not found", base, "try again")
	require.Error(t, err)
	assert.ErrorIs(t, err, base, "the underlying sentinel/error must remain in the chain for errors.Is")

	details := cockroacherrors.GetAllDetails(err)
	joined := ""
	for _, d := range details {
		joined += d
	}
	assert.Contains(t, joined, "/tmp/workdir")
	assert.Contains(t, joined, "fatal: remote branch not found")
	assert.Contains(t, joined, base.Error())

	hints := cockroacherrors.GetAllHints(err)
	joinedHints := ""
	for _, h := range hints {
		joinedHints += h
	}
	assert.Contains(t, joinedHints, "try again")
}

func TestWrapOperationError_NoWorkdirOmitsQuotedEmptyPath(t *testing.T) {
	base := errors.New("failed")
	err := WrapOperationError("do something", "", "", base, "")
	require.Error(t, err)
	details := cockroacherrors.GetAllDetails(err)
	joined := ""
	for _, d := range details {
		joined += d
	}
	assert.NotContains(t, joined, `in ""`)
}
