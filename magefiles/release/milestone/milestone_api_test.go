//go:build mage

package milestone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func milestoneHTTP(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	githubtest.Server(t, handler)
	c, err := NewClient(context.Background(), githubtest.Repo)
	require.NoError(t, err)
	return c
}

func stableRelease(tag string) map[string]any {
	return map[string]any{"tag_name": tag, "published_at": "2026-10-01T00:00:00Z"}
}

func TestMilestonePagination(t *testing.T) {
	for _, mode := range []string{"two pages", "error", "limit"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/repos/cloudposse/atmos/milestones", r.URL.Path)
				require.Equal(t, "all", r.URL.Query().Get("state"))
				require.Equal(t, "100", r.URL.Query().Get("per_page"))
				calls++
				require.Equal(t, strconv.Itoa(calls), r.URL.Query().Get("page"))
				if mode == "error" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				size := 100
				if mode == "two pages" && calls == 2 {
					size = 1
				}
				items := make([]Milestone, size)
				for i := range items {
					items[i] = Milestone{(calls-1)*100 + i + 1, fmt.Sprintf("v1.%d.0", (calls-1)*100+i+1), "open"}
				}
				githubtest.JSON(t, w, items)
			})
			got, err := c.ListMilestones()
			switch mode {
			case "error":
				require.ErrorContains(t, err, "503")
				require.Nil(t, got)
			case "limit":
				require.ErrorIs(t, err, ErrMilestonePolicy)
				require.ErrorContains(t, err, "pagination limit")
				require.Nil(t, got)
				require.Equal(t, 100, calls)
			default:
				require.NoError(t, err)
				require.Len(t, got, 101)
				require.Equal(t, Milestone{1, "v1.1.0", "open"}, got[0])
				require.Equal(t, Milestone{101, "v1.101.0", "open"}, got[100])
			}
		})
	}
}

func TestLatestStableIgnoresDraftsPrereleasesAndPublicationOrder(t *testing.T) {
	releases := []map[string]any{stableRelease("v1.9.0"), stableRelease("v1.10.0"), stableRelease("v1.8.0"), stableRelease("nightly"), stableRelease("v2.0.0-rc.1")}
	draft, prerelease := stableRelease("v9.0.0"), stableRelease("v8.0.0")
	draft["draft"], prerelease["prerelease"] = true, true
	releases = append(releases, draft, prerelease, map[string]any{"tag_name": "v7.0.0"})
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) { githubtest.JSON(t, w, releases) })
	got, err := c.LatestStable()
	require.NoError(t, err)
	require.Equal(t, "v1.10.0", got)
	releases = releases[3:]
	_, err = c.LatestStable()
	require.ErrorIs(t, err, ErrMilestonePolicy)
	require.ErrorContains(t, err, "no published stable")
}

func TestRepositoryLookupHasNoTrailingSlash(t *testing.T) {
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/cloudposse/atmos" {
			http.NotFound(w, r)
			return
		}
		githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
	})
	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}
	require.NoError(t, c.Get("", &repository))
	require.Equal(t, "main", repository.DefaultBranch)
}

func TestEnsureMilestoneRetriesAndConflicts(t *testing.T) {
	for _, tc := range []struct {
		name               string
		initial, fresh     []Milestone
		postCode, readCode int
		wantErr            string
		wantPosts          int
	}{
		{name: "create", wantPosts: 1},
		{name: "reuse normalized version", initial: []Milestone{{1, "1.234.0", "open"}}},
		{name: "closed cannot be reused", initial: []Milestone{{1, "v1.234.0", "closed"}}, wantErr: "already exists and is closed"},
		{name: "duplicate", initial: []Milestone{{1, "v1.234.0", "open"}, {2, "1.234.0", "closed"}}, wantErr: "duplicate"},
		{name: "lost response recovered", postCode: 503, fresh: []Milestone{{1, "1.234.0", "open"}}, wantPosts: 1},
		{name: "lost response remains absent", postCode: 503, wantPosts: 1, wantErr: "503"},
		{name: "closed after response lost", postCode: 503, fresh: []Milestone{{1, "v1.234.0", "closed"}}, wantPosts: 1, wantErr: "503"},
		{name: "ambiguous recovery", postCode: 503, fresh: []Milestone{{1, "1.234.0", "open"}, {2, "v1.234.0", "open"}}, wantPosts: 1, wantErr: "503"},
		{name: "recovery read fails", postCode: 503, readCode: 502, wantPosts: 1, wantErr: "502"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, posts := 0, 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/repos/cloudposse/atmos/milestones", r.URL.Path)
				if r.Method == http.MethodGet {
					reads++
					if reads > 1 && tc.readCode != 0 {
						http.Error(w, "read failed", tc.readCode)
						return
					}
					data := tc.initial
					if reads > 1 {
						data = tc.fresh
					}
					githubtest.JSON(t, w, data)
					return
				}
				require.Equal(t, http.MethodPost, r.Method)
				posts++
				var body map[string]string
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, map[string]string{"title": "v1.234.0", "state": "open"}, body)
				if tc.postCode != 0 {
					http.Error(w, "write failed", tc.postCode)
					return
				}
				githubtest.JSON(t, w, map[string]int{"number": 1})
			})
			err := c.ensureMilestone("v1.234.0")
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			require.Equal(t, tc.wantPosts, posts)
		})
	}
}

func TestEnsureMinorMilestones(t *testing.T) {
	for _, mode := range []string{"create and reopen", "baseline changed", "recheck failed", "read failed", "initial baseline failed", "invalid window", "patch failed", "create failed"} {
		t.Run(mode, func(t *testing.T) {
			releases, posts, patches := 0, []string{}, []string{}
			existing := []Milestone{{1, "v1.234.0", "closed"}, {2, "v1.235.0", "open"}, {3, "v1.233.2", "open"}, {4, "v2.0.0", "open"}}
			if mode == "invalid window" {
				existing = append(existing, Milestone{9, "1.235.0", "open"})
			}
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/releases"):
					releases++
					if mode == "initial baseline failed" || (mode == "recheck failed" && releases > 1) {
						http.Error(w, "release unavailable", http.StatusServiceUnavailable)
						return
					}
					tag := "v1.233.1"
					if mode == "baseline changed" && releases > 1 {
						tag = "v1.234.0"
					}
					githubtest.JSON(t, w, []any{stableRelease(tag)})
				case r.Method == http.MethodGet:
					if mode == "read failed" {
						http.Error(w, "milestones unavailable", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, existing)
				case r.Method == http.MethodPatch:
					patches = append(patches, r.URL.Path)
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, map[string]string{"state": "open"}, body)
					if mode == "patch failed" {
						http.Error(w, "patch failed", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, map[string]any{})
				case r.Method == http.MethodPost:
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					posts = append(posts, body["title"])
					if mode == "create failed" {
						http.Error(w, "create failed", http.StatusServiceUnavailable)
						return
					}
					githubtest.JSON(t, w, map[string]any{})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
			})
			err := c.ensureMinorMilestones()
			if mode == "create and reopen" {
				require.NoError(t, err)
				require.Equal(t, []string{"v1.236.0"}, posts)
				require.Equal(t, []string{"/repos/cloudposse/atmos/milestones/1"}, patches)
			} else {
				require.Error(t, err)
				if mode == "baseline changed" {
					require.ErrorContains(t, err, "stable release changed")
					require.Empty(t, patches)
				}
			}
		})
	}
}

func TestMilestoneEntrypointsAndValidation(t *testing.T) {
	for _, invoke := range []func() error{
		func() error { return (Milestones{}).Create("invalid", "v1.2.3") },
		func() error { return (Milestones{}).Create(githubtest.Repo, "bad") },
		func() error { return (Milestones{}).Ensure("invalid") },
		func() error { return (Milestones{}).Check("invalid") },
	} {
		require.ErrorIs(t, invoke(), ErrMilestonePolicy)
	}
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases") {
			githubtest.JSON(t, w, []any{stableRelease("v1.233.1")})
			return
		}
		githubtest.JSON(t, w, []Milestone{{1, "v1.234.0", "open"}, {2, "v1.235.0", "open"}, {3, "v1.236.0", "open"}})
	})
	require.NoError(t, (Milestones{}).Create(githubtest.Repo, "v1.234.0"))
	require.NoError(t, (Milestones{}).Ensure(githubtest.Repo))
	require.ErrorIs(t, c.ensureMilestone("invalid"), ErrMilestonePolicy)
	require.Error(t, c.Write(http.MethodPost, "milestones", make(chan int)))
}

func TestEnsureMilestoneReadFailureDoesNotCreate(t *testing.T) {
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		http.Error(w, "milestones unavailable", http.StatusServiceUnavailable)
	})
	require.ErrorContains(t, c.ensureMilestone("v1.234.0"), "503")
}

func TestMilestonePolicyRejectsMissingTrainAndInvalidTarget(t *testing.T) {
	_, err := RequiredBump("v1.233.0", "v1.234.0-rc.1")
	require.ErrorIs(t, err, ErrMilestonePolicy)
	m := Milestone{1, "v1.234.0", "open"}
	pr := policyPR("patch", &m)
	require.ErrorContains(t, CheckMilestonePR(&pr, nil, "v1.233.0"), "no open versioned milestone")
	changes, err := MinorMilestoneWindow("v1.233.1", []Milestone{{1, "Backlog", "open"}})
	require.NoError(t, err)
	require.Equal(t, []MilestoneChange{{0, "v1.234.0"}, {0, "v1.235.0"}, {0, "v1.236.0"}}, changes)
}

func TestMilestoneActionDispatch(t *testing.T) {
	for _, operation := range []string{"check", "ensure", "create", "unknown"} {
		t.Run(operation, func(t *testing.T) {
			t.Setenv("GITHUB_EVENT_NAME", "workflow_dispatch")
			requests, writes := 0, 0
			githubtest.Server(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.NotEqual(t, "unknown", operation, "unsupported operations must fail before contacting GitHub")
				switch r.URL.Path {
				case "/repos/cloudposse/atmos":
					require.Equal(t, "check", operation)
					githubtest.JSON(t, w, map[string]string{"default_branch": "main"})
				case "/repos/cloudposse/atmos/pulls":
					require.Equal(t, "check", operation)
					require.Equal(t, "main", r.URL.Query().Get("base"))
					githubtest.JSON(t, w, []ReleasePR{})
				case "/graphql":
					require.Equal(t, "check", operation)
					githubtest.JSON(t, w, queueResponse(nil, false, ""))
				case "/repos/cloudposse/atmos/releases":
					require.NotEqual(t, "create", operation)
					githubtest.JSON(t, w, []any{stableRelease("v1.233.0")})
				case "/repos/cloudposse/atmos/milestones":
					if r.Method == http.MethodPost {
						writes++
						require.Equal(t, "create", operation)
						var body map[string]string
						require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
						require.Equal(t, map[string]string{"title": "v1.233.1", "state": "open"}, body)
						githubtest.JSON(t, w, map[string]int{"number": 4})
						return
					}
					require.Equal(t, http.MethodGet, r.Method)
					githubtest.JSON(t, w, []Milestone{{1, "v1.234.0", "open"}, {2, "v1.235.0", "open"}, {3, "v1.236.0", "open"}})
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
				}
			})
			err := (Milestones{}).Run(githubtest.Repo, operation, "v1.233.1")
			if operation == "unknown" {
				require.ErrorIs(t, err, ErrMilestonePolicy)
				require.Zero(t, requests)
			} else {
				require.NoError(t, err)
				require.Positive(t, requests)
			}
			if operation == "create" {
				require.Equal(t, 1, writes, "explicit create operation passes the requested patch version without semver inference")
			} else {
				require.Zero(t, writes)
			}
		})
	}
}
