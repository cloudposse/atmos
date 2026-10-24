//go:build mage

package qualitygate

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// releaseContextAPI models transport cancellation during an evidence request.
type releaseContextAPI struct {
	githubapi.API
	before func(string)
	calls  int
}

func (api *releaseContextAPI) DoWithContext(ctx context.Context, method, path string, body io.Reader, response any) error {
	api.calls++
	if api.before != nil {
		api.before(path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return api.API.DoWithContext(ctx, method, path, body, response)
}

func TestQualityReleaseEvidenceSurvivesIndividualPRCancellation(t *testing.T) {
	for _, endpoint := range []string{"milestones?", "/compare/"} {
		t.Run(endpoint, func(t *testing.T) {
			f := &releaseEvidenceFixture{}
			q := f.client(t)
			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			q.ctx = runCtx
			require.NoError(t, q.reconcileAll(nil))
			prCtx, cancelPR := context.WithCancel(runCtx)
			defer cancelPR()
			transport := &releaseContextAPI{API: q.api, before: func(path string) {
				if strings.Contains(path, endpoint) {
					cancelPR()
				}
			}}
			q.api = transport
			scoped := *q
			scoped.ctx = prCtx
			pr := releaseEvidencePR()
			gate, err := scoped.releasePolicy(&pr)
			require.NoError(t, err, "shared evidence must use the live run context")
			require.True(t, gate.Passed)
			require.ErrorIs(t, prCtx.Err(), context.Canceled, "cancellation must occur during evidence loading")
			before := transport.calls
			gate, err = q.releasePolicy(&pr)
			require.NoError(t, err)
			require.True(t, gate.Passed, "later PRs must reuse healthy release evidence")
			require.Equal(t, before, transport.calls, "the cache must not reload evidence")
			// The run context applies only to shared evidence. An expired PR
			// cannot perform its own final identity check or publication calls.
			require.ErrorIs(t, scoped.get("pulls/42", &struct{}{}), context.Canceled)
		})
	}
}

func TestQualityReleaseEvidenceRunCancellationFailsClosed(t *testing.T) {
	for _, endpoint := range []string{"milestones?", "/compare/"} {
		t.Run(endpoint, func(t *testing.T) {
			f := &releaseEvidenceFixture{}
			q := f.client(t)
			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			q.ctx = runCtx
			// Standalone reporters lazily initialize the cache before rendering.
			q.initReleaseEvidence()
			transport := &releaseContextAPI{API: q.api, before: func(path string) {
				if strings.Contains(path, endpoint) {
					cancelRun()
				}
			}}
			q.api = transport
			pr := releaseEvidencePR()
			gate, err := q.releasePolicy(&pr)
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, gate.Passed)
			before := transport.calls
			gate, err = q.releasePolicy(&pr)
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, gate.Passed)
			require.Equal(t, before, transport.calls, "run cancellation must remain a cached failure")
			pr.Labels[0].Name = "no-release"
			gate, err = q.releasePolicy(&pr)
			require.NoError(t, err)
			require.True(t, gate.Passed, "no-release does not depend on repository release evidence")
		})
	}
}
