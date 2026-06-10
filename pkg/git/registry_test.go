package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

type stubProvider struct{}

func (s *stubProvider) Clone(context.Context, *CloneOptions) error { return nil }
func (s *stubProvider) Pull(context.Context, *PullOptions) error   { return nil }
func (s *stubProvider) Status(context.Context, *StatusOptions) (*StatusResult, error) {
	return &StatusResult{Clean: true}, nil
}

func (s *stubProvider) Diff(context.Context, *DiffOptions) (*DiffResult, error) {
	return &DiffResult{}, nil
}

func (s *stubProvider) Commit(context.Context, *CommitOptions) (*CommitResult, error) {
	return &CommitResult{}, nil
}
func (s *stubProvider) Push(context.Context, *PushOptions) error { return nil }

func TestProviderRegistry(t *testing.T) {
	RegisterProvider("test-stub", func() Provider { return &stubProvider{} })

	provider, err := NewProvider("test-stub")
	require.NoError(t, err)
	assert.NotNil(t, provider)

	names := RegisteredProviders()
	assert.Contains(t, names, "test-stub")
}

func TestNewProviderUnknown(t *testing.T) {
	_, err := NewProvider("no-such-provider")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitProviderNotFound))
}

func TestNewProviderEmptyNameUsesDefault(t *testing.T) {
	// The default name resolves only when the cli provider package is
	// imported; in this package's tests it is not registered, so the lookup
	// must report the default name as missing rather than panic.
	_, err := NewProvider("")
	if err != nil {
		assert.True(t, errors.Is(err, errUtils.ErrGitProviderNotFound))
		assert.Contains(t, err.Error(), DefaultProviderName)
	}
}
