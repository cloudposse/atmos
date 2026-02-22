package generic

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCheckRun(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	t.Run("returns check run with correct fields", func(t *testing.T) {
		opts := &provider.CreateCheckRunOptions{
			Name:    "atmos/plan: plat-ue2-dev/vpc",
			Status:  provider.CheckRunStatePending,
			Title:   "Planning vpc",
			Summary: "Running terraform plan",
		}

		checkRun, err := p.CreateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.NotZero(t, checkRun.ID)
		assert.Equal(t, opts.Name, checkRun.Name)
		assert.Equal(t, opts.Status, checkRun.Status)
		assert.Equal(t, opts.Title, checkRun.Title)
		assert.Equal(t, opts.Summary, checkRun.Summary)
		assert.False(t, checkRun.StartedAt.IsZero())
	})

	t.Run("incrementing IDs", func(t *testing.T) {
		p := NewProvider()
		opts := &provider.CreateCheckRunOptions{
			Name:   "check-1",
			Status: provider.CheckRunStatePending,
		}

		first, err := p.CreateCheckRun(ctx, opts)
		require.NoError(t, err)

		opts.Name = "check-2"
		second, err := p.CreateCheckRun(ctx, opts)
		require.NoError(t, err)

		assert.NotEqual(t, first.ID, second.ID)
		assert.Equal(t, first.ID+1, second.ID)
	})
}

func TestUpdateCheckRun(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	t.Run("success status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 1,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateSuccess,
			Conclusion: "success",
			Title:      "Plan succeeded",
			Summary:    "No changes needed",
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, int64(1), checkRun.ID)
		assert.Equal(t, opts.Name, checkRun.Name)
		assert.Equal(t, opts.Status, checkRun.Status)
		assert.Equal(t, opts.Conclusion, checkRun.Conclusion)
		assert.Equal(t, opts.Title, checkRun.Title)
		assert.Equal(t, opts.Summary, checkRun.Summary)
	})

	t.Run("failure status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 2,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateFailure,
			Conclusion: "failure",
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateFailure, checkRun.Status)
	})

	t.Run("error status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 3,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateError,
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateError, checkRun.Status)
	})

	t.Run("cancelled status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 4,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateCancelled,
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateCancelled, checkRun.Status)
	})

	t.Run("in progress status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 5,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateInProgress,
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateInProgress, checkRun.Status)
	})
}
