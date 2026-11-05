package function

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
)

func TestCILabelsFunction(t *testing.T) {
	t.Cleanup(ci.SwapRegistryForTest())
	ci.ResetLabelsCache()
	t.Cleanup(ci.ResetLabelsCache)
	var fail atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"message":"forbidden"}`)
			return
		}
		_, _ = fmt.Fprint(w, `[{"name":"preview"},{"name":"needs review"},{"name":"deploy"}]`)
	}))
	t.Cleanup(s.Close)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_REF", "refs/pull/42/merge")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "labels-test-token")
	t.Setenv("ATMOS_CI_GITHUB_API_URL", s.URL)
	ci.Register(github.NewProvider())
	f := NewCILabelsFunction()
	assert.Equal(t, PostMerge, f.Phase())
	for _, tc := range []struct {
		args string
		want any
	}{
		{"", []any{"deploy", "needs review", "preview"}},
		{"[preview]", true},
		{"[preview, deploy]", true},
		{"[preview, missing]", false},
		{"[Preview]", false},
		{"[preview, preview]", true},
		{`["needs review"]`, true},
	} {
		t.Run(tc.args, func(t *testing.T) {
			got, err := f.Execute(context.Background(), tc.args, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	ci.ResetLabelsCache()
	fail.Store(true)
	_, err := f.Execute(context.Background(), "[]", nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
	_, err = f.Execute(context.Background(), "[preview]", nil)
	require.ErrorIs(t, err, errUtils.ErrCILabelsFetchFailed)
	ci.Reset()
	got, err := f.Execute(context.Background(), "", nil)
	require.NoError(t, err)
	assert.Equal(t, []any{}, got)
	got, err = f.Execute(context.Background(), "[preview]", nil)
	require.NoError(t, err)
	assert.Equal(t, false, got)
}
