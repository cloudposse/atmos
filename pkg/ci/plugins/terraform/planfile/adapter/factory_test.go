package adapter

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
)

// errStoreUnavailable stands in for a backend that cannot initialize in the
// current environment (e.g. GitHub Artifacts without a runtime token).
var errStoreUnavailable = errors.New("backend unavailable")

// stubBackend is a minimal artifact.Store used to observe which candidate won.
type stubBackend struct {
	artifact.Store
	name string
}

func (s *stubBackend) Name() string { return s.name }

// stubBackends installs a backend factory that succeeds only for the listed types
// and records the options each attempt was made with.
func stubBackends(t *testing.T, available ...string) *[]planfile.StoreOptions {
	t.Helper()

	attempts := &[]planfile.StoreOptions{}
	original := newBackend
	newBackend = func(opts artifact.StoreOptions) (artifact.Store, error) {
		*attempts = append(*attempts, opts)
		for _, storeType := range available {
			if opts.Type == storeType {
				return &stubBackend{name: opts.Type}, nil
			}
		}
		return nil, errStoreUnavailable
	}
	t.Cleanup(func() { newBackend = original })

	return attempts
}

func candidate(name, storeType string, source planfile.StoreSource) planfile.StoreCandidate {
	return planfile.StoreCandidate{
		Name:    name,
		Source:  source,
		Options: planfile.StoreOptions{Type: storeType, Options: map[string]any{}},
	}
}

func TestNewStoreFromCandidates(t *testing.T) {
	t.Run("uses the first candidate that initializes", func(t *testing.T) {
		attempts := stubBackends(t, planfile.LocalStoreType)

		candidates := []planfile.StoreCandidate{
			candidate("github", planfile.GitHubStoreType, planfile.StoreSourcePriority),
			candidate("s3", planfile.S3StoreType, planfile.StoreSourcePriority),
			candidate("local", planfile.LocalStoreType, planfile.StoreSourcePriority),
		}

		store, selected, err := NewStoreFromCandidates(candidates, nil)
		require.NoError(t, err)
		assert.Equal(t, planfile.LocalStoreType, store.Name())
		assert.Equal(t, "local", selected.Name)
		assert.Equal(t, planfile.StoreSourcePriority, selected.Source)

		// Every earlier candidate was attempted, in order.
		require.Len(t, *attempts, 3)
		assert.Equal(t, planfile.GitHubStoreType, (*attempts)[0].Type)
		assert.Equal(t, planfile.LocalStoreType, (*attempts)[2].Type)
	})

	t.Run("stops at the first success without trying later candidates", func(t *testing.T) {
		attempts := stubBackends(t, planfile.S3StoreType, planfile.LocalStoreType)

		candidates := []planfile.StoreCandidate{
			candidate("s3", planfile.S3StoreType, planfile.StoreSourcePriority),
			candidate("local", planfile.LocalStoreType, planfile.StoreSourcePriority),
		}

		store, selected, err := NewStoreFromCandidates(candidates, nil)
		require.NoError(t, err)
		assert.Equal(t, planfile.S3StoreType, store.Name())
		assert.Equal(t, "s3", selected.Name)
		require.Len(t, *attempts, 1)
		assert.Equal(t, planfile.S3StoreType, (*attempts)[0].Type)
	})

	t.Run("joins the failures when no candidate initializes", func(t *testing.T) {
		stubBackends(t)

		candidates := []planfile.StoreCandidate{
			candidate("github", planfile.GitHubStoreType, planfile.StoreSourcePriority),
			candidate("s3", planfile.S3StoreType, planfile.StoreSourcePriority),
		}

		store, selected, err := NewStoreFromCandidates(candidates, nil)
		require.Error(t, err)
		assert.Nil(t, store)
		assert.Empty(t, selected.Name)
		require.ErrorIs(t, err, errUtils.ErrPlanfileStoreUnavailable)
		require.ErrorIs(t, err, errStoreUnavailable)
		// Both attempts are described so the user can see why each was rejected.
		assert.Contains(t, err.Error(), "github (github/artifacts)")
		assert.Contains(t, err.Error(), "s3 (aws/s3)")
	})

	t.Run("no candidates is an error", func(t *testing.T) {
		stubBackends(t, planfile.LocalStoreType)

		store, _, err := NewStoreFromCandidates(nil, nil)
		require.ErrorIs(t, err, errUtils.ErrPlanfileStoreUnavailable)
		assert.Nil(t, store)
	})

	t.Run("decorator is applied to the constructed options", func(t *testing.T) {
		attempts := stubBackends(t, planfile.S3StoreType)

		candidates := []planfile.StoreCandidate{candidate("s3", planfile.S3StoreType, planfile.StoreSourceDefault)}

		_, selected, err := NewStoreFromCandidates(candidates, func(opts *planfile.StoreOptions) {
			opts.Identity = "core-gbl-root-admin"
		})
		require.NoError(t, err)
		require.Len(t, *attempts, 1)
		assert.Equal(t, "core-gbl-root-admin", (*attempts)[0].Identity)
		// The returned candidate reflects the decorated options.
		assert.Equal(t, "core-gbl-root-admin", selected.Options.Identity)
	})

	t.Run("decorator does not mutate the caller's candidates", func(t *testing.T) {
		stubBackends(t, planfile.S3StoreType)

		candidates := []planfile.StoreCandidate{candidate("s3", planfile.S3StoreType, planfile.StoreSourceDefault)}

		_, _, err := NewStoreFromCandidates(candidates, func(opts *planfile.StoreOptions) {
			opts.Identity = "core-gbl-root-admin"
		})
		require.NoError(t, err)
		assert.Empty(t, candidates[0].Options.Identity)
	})
}
