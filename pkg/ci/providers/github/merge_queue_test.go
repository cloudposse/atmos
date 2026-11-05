package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const testQueueRef = "refs/heads/gh-readonly-queue/main/pr-42-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestProviderContextMergeQueue(t *testing.T) {
	for _, tc := range []struct {
		name, payloadRef, ref, refName string
		want                           int
	}{
		{"payload", testQueueRef, "refs/heads/main", "main", 42},
		{"payload wins", testQueueRef, strings.Replace(testQueueRef, "pr-42", "pr-99", 1), "", 42},
		{"environment", "", testQueueRef, "", 42},
		{"ref name", "", "", strings.TrimPrefix(testQueueRef, "refs/heads/"), 42},
		{"nested base branch", strings.Replace(testQueueRef, "/main/", "/release/pr-99-stable/", 1), "", "", 42},
		{"invalid payload does not use another PR", "refs/heads/main", testQueueRef, "", 0},
		{"missing", "", "", "", 0},
		{"ordinary PR ref", "", "refs/pull/42/merge", "", 0},
		{"ordinary branch", "", "refs/heads/feature/pr-42-deadbeef", "", 0},
		{"missing base branch", "", "refs/heads/gh-readonly-queue/pr-42-deadbeef", "", 0},
		{"zero", "", strings.Replace(testQueueRef, "pr-42", "pr-0", 1), "", 0},
		{"negative", "", strings.Replace(testQueueRef, "pr-42", "pr--1", 1), "", 0},
		{"overflow", "", strings.Replace(testQueueRef, "pr-42", "pr-99999999999999999999999", 1), "", 0},
		{"missing suffix", "", "refs/heads/gh-readonly-queue/main/pr-42-", "", 0},
		{"invalid suffix", "", "refs/heads/gh-readonly-queue/main/pr-42-not-a-sha", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_EVENT_NAME", eventMergeGroup)
			t.Setenv("GITHUB_EVENT_PATH", "")
			t.Setenv("GITHUB_REF", tc.ref)
			t.Setenv("GITHUB_REF_NAME", tc.refName)
			if tc.payloadRef != "" {
				t.Setenv("GITHUB_EVENT_PATH", writeEventPayload(t, map[string]any{
					"action": "checks_requested", "merge_group": map[string]any{"head_ref": tc.payloadRef},
				}))
			}
			p := NewProvider()
			run, err := p.Context()
			require.NoError(t, err)
			require.NotNil(t, run.MergeQueue)
			assert.Equal(t, tc.want, run.MergeQueue.PullRequestNumber)
			assert.Nil(t, run.PullRequest, "queue runs must retain their synthetic-commit context")
			assert.Nil(t, p.client, "context detection must not initialize the API client")
		})
	}
}

func TestProviderLabelsMergeQueue(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/api/v3/repos/owner/repo/issues/42/labels", r.URL.Path)
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `[{"name":"deploy"}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
		_, _ = fmt.Fprint(w, `[{"name":"preview"}]`)
	}))
	t.Cleanup(s.Close)
	t.Setenv("GITHUB_EVENT_NAME", eventMergeGroup)
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_REF", testQueueRef)
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	p := setupLabelsClient(t, s.URL+"/api/v3/")
	run, err := p.Context()
	require.NoError(t, err)
	got, err := p.Labels(context.Background(), run)
	require.NoError(t, err)
	assert.Equal(t, []string{"preview", "deploy"}, got)
	assert.EqualValues(t, 2, calls.Load(), "fetch only this PR's labels, including every page")

	run.MergeQueue.PullRequestNumber = 0
	_, err = p.Labels(context.Background(), run)
	require.ErrorIs(t, err, errUtils.ErrInvalidArguments)
	assert.EqualValues(t, 2, calls.Load(), "an unresolved queue PR must not issue API calls")
}

func TestProviderContextQueueRefOnPush(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_REF", testQueueRef)
	p := NewProvider()
	run, err := p.Context()
	require.NoError(t, err)
	assert.Nil(t, run.PullRequest)
	assert.Nil(t, run.MergeQueue, "only merge_group events resolve queue review identity")
}
