//go:build mage

package release

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func releaseEnvironment() map[string]string {
	return map[string]string{"ATMOS_GUARDED_RELEASES_ENABLED": "true", "GITHUB_ACTOR_ID": "52489", "GITHUB_TRIGGERING_ACTOR": "osterman", "GITHUB_RUN_ID": "77", "GITHUB_RUN_ATTEMPT": "1", "GITHUB_EVENT_NAME": "workflow_dispatch", "RELEASE_MODE": "cut", "RELEASE_MILESTONE": "1", "RELEASE_VERSION": "v1.234.0", "RELEASE_COMMIT": githubtest.SHA}
}

func environmentReader(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func tagObject(kind, sha string) map[string]any {
	return map[string]any{"object": map[string]string{"type": kind, "sha": sha}}
}

func TestReleaseAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name              string
		changes           map[string]string
		userID            int64
		userType          string
		apiFails, allowed bool
	}{
		{name: "Erik", allowed: true},
		{name: "Andriy", changes: map[string]string{"GITHUB_ACTOR_ID": "7356997"}, userID: 7356997, allowed: true},
		{name: "cutover disabled", changes: map[string]string{"ATMOS_GUARDED_RELEASES_ENABLED": "false"}},
		{name: "actor ID substring", changes: map[string]string{"GITHUB_ACTOR_ID": "152489"}},
		{name: "ID leading zeros", changes: map[string]string{"GITHUB_ACTOR_ID": "052489"}},
		{name: "ID positive sign", changes: map[string]string{"GITHUB_ACTOR_ID": "+52489"}},
		{name: "invalid ID", changes: map[string]string{"GITHUB_ACTOR_ID": "osterman"}},
		{name: "missing triggering actor", changes: map[string]string{"GITHUB_TRIGGERING_ACTOR": ""}},
		{name: "unauthorized rerunner", userID: 152489},
		{name: "bot rerunner", userType: "Bot"},
		{name: "user API fails", apiFails: true},
		{name: "initial scheduled nightly", changes: map[string]string{"RELEASE_MODE": "nightly", "GITHUB_EVENT_NAME": "schedule", "GITHUB_ACTOR_ID": "untrusted", "GITHUB_TRIGGERING_ACTOR": ""}, allowed: true},
		{name: "scheduled unauthorized rerun", changes: map[string]string{"RELEASE_MODE": "nightly", "GITHUB_EVENT_NAME": "schedule", "GITHUB_ACTOR_ID": "untrusted", "GITHUB_RUN_ATTEMPT": "2"}, userID: 152489},
		{name: "scheduled authorized rerun", changes: map[string]string{"RELEASE_MODE": "nightly", "GITHUB_EVENT_NAME": "schedule", "GITHUB_ACTOR_ID": "untrusted", "GITHUB_RUN_ATTEMPT": "2"}, userID: 7356997, allowed: true},
		{name: "scheduled missing attempt not exempt", changes: map[string]string{"RELEASE_MODE": "nightly", "GITHUB_EVENT_NAME": "schedule", "GITHUB_RUN_ATTEMPT": ""}, userID: 152489},
		{name: "scheduled cut never exempt", changes: map[string]string{"GITHUB_EVENT_NAME": "schedule", "GITHUB_ACTOR_ID": "152489"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := releaseEnvironment()
			for k, v := range tc.changes {
				env[k] = v
			}
			calls := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/users/osterman", r.URL.Path)
				require.Equal(t, http.MethodGet, r.Method)
				if tc.apiFails {
					http.Error(w, "user unavailable", http.StatusServiceUnavailable)
					return
				}
				id := tc.userID
				if id == 0 {
					id = 52489
				}
				kind := tc.userType
				if kind == "" {
					kind = "User"
				}
				githubtest.JSON(t, w, map[string]any{"id": id, "type": kind})
			})
			err := c.authorize(environmentReader(env))
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			if tc.name == "initial scheduled nightly" {
				require.Zero(t, calls)
			}
		})
	}
}

func TestPrepareReleaseModesAndTagFirst(t *testing.T) {
	for _, mode := range []string{"cut", "candidate", "nightly", "feature"} {
		t.Run(mode, func(t *testing.T) {
			env := releaseEnvironment()
			env["RELEASE_MODE"] = mode
			env["RELEASE_PR"] = "42"
			routes := planRoutes()
			pr := routes["pulls/42"].(releasePull)
			pr.Head.SHA = githubtest.Base
			routes["pulls/42"] = pr
			source := githubtest.SHA
			tag := "v1.234.0"
			switch mode {
			case "candidate":
				tag += "-rc.77"
			case "nightly":
				tag += "-nightly.77"
			case "feature":
				tag += "-feature.42.77"
				source = githubtest.Base
			}
			created := false
			writes := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/users/osterman" {
					githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
					return
				}
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if path == "git/ref/tags/"+tag {
					if !created {
						http.Error(w, "not found", http.StatusNotFound)
						return
					}
					githubtest.JSON(t, w, tagObject("commit", source))
					return
				}
				if r.Method != http.MethodGet {
					writes++
					require.Equal(t, "git/refs", path, "Prepare must only reserve the tag, never create a release or assets")
					var body map[string]string
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, map[string]string{"ref": "refs/tags/" + tag, "sha": source}, body)
					created = true
					githubtest.JSON(t, w, map[string]any{})
					return
				}
				value, ok := routes[path]
				require.True(t, ok, "unexpected %s", r.URL)
				githubtest.JSON(t, w, value)
			})
			p, err := c.prepare(environmentReader(env))
			require.NoError(t, err)
			require.Equal(t, tag, p.Tag)
			require.Equal(t, source, p.Commit)
			require.Equal(t, mode, p.Mode)
			require.Equal(t, "77", p.RunID)
			require.Equal(t, 1, writes)
			retry, err := c.prepare(environmentReader(env))
			require.NoError(t, err)
			require.Equal(t, p, retry)
			require.Equal(t, 1, writes, "retry must preserve exact tag identity")
		})
	}
}

func TestPrepareRejectsChangedOrMissingInputsBeforeTagging(t *testing.T) {
	for _, tc := range []struct{ key, value, want string }{
		{"ATMOS_GUARDED_RELEASES_ENABLED", "false", "cutover"},
		{"RELEASE_MODE", "invalid", "unsupported"},
		{"RELEASE_MILESTONE", "2", "selected milestone changed"},
		{"RELEASE_VERSION", "v1.235.0", "milestone version changed"},
		{"RELEASE_MILESTONE", "", "explicit milestone"},
		{"RELEASE_VERSION", "", "explicit milestone"},
		{"RELEASE_COMMIT", "", "explicit milestone"},
		{"GITHUB_RUN_ID", "0", "invalid run identity"},
		{"GITHUB_RUN_ID", "077", "invalid run identity"},
		{"GITHUB_RUN_ID", "+77", "invalid run identity"},
		{"GITHUB_RUN_ID", "", "invalid run identity"},
		{"GITHUB_RUN_ID", "invalid", "invalid run identity"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			env := releaseEnvironment()
			env[tc.key] = tc.value
			routes := planRoutes()
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				if r.URL.Path == "/users/osterman" {
					githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
					return
				}
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				value, ok := routes[path]
				require.True(t, ok, "no tag operations should occur: %s", r.URL)
				githubtest.JSON(t, w, value)
			})
			p, err := c.prepare(environmentReader(env))
			require.Nil(t, p)
			require.ErrorIs(t, err, ErrMilestonePolicy)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestFeaturePlanFailures(t *testing.T) {
	for _, mode := range []string{"invalid number", "leading zero", "negative number", "PR API", "closed PR", "invalid SHA", "milestones API", "no milestone", "baseline API", "wrong label"} {
		t.Run(mode, func(t *testing.T) {
			number := "42"
			routes := planRoutes()
			pr := routes["pulls/42"].(releasePull)
			pr.Head.SHA = githubtest.Base
			switch mode {
			case "invalid number":
				number = "bad"
			case "leading zero":
				number = "042"
			case "negative number":
				number = "-1"
			case "closed PR":
				pr.State = "closed"
			case "invalid SHA":
				pr.Head.SHA = "bad"
			case "no milestone":
				routes["milestones"] = []Milestone{}
			case "wrong label":
				pr.Labels[0].Name = "major"
			}
			routes["pulls/42"] = pr
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if mode == "PR API" && path == "pulls/42" || mode == "milestones API" && path == "milestones" || mode == "baseline API" && path == "releases" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				value, ok := routes[path]
				require.True(t, ok)
				githubtest.JSON(t, w, value)
			})
			p, err := c.featurePlan(number)
			require.Nil(t, p)
			require.Error(t, err)
		})
	}
}

func TestEnsureTagResolutionAndConflicts(t *testing.T) {
	for _, mode := range []string{"lightweight", "annotated", "deep annotation", "invalid type", "invalid commit", "read error", "annotation error", "conflict", "create failure", "creation reread failure"} {
		t.Run(mode, func(t *testing.T) {
			reads, writes := 0, 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if r.Method == http.MethodPost {
					writes++
					if mode == "create failure" {
						http.Error(w, "create denied", http.StatusForbidden)
						return
					}
					githubtest.JSON(t, w, map[string]any{})
					return
				}
				reads++
				if strings.HasPrefix(path, "git/tags/") {
					switch mode {
					case "annotation error":
						http.Error(w, "annotation unavailable", http.StatusServiceUnavailable)
					case "deep annotation":
						githubtest.JSON(t, w, tagObject("tag", githubtest.Base))
					default:
						githubtest.JSON(t, w, tagObject("commit", githubtest.SHA))
					}
					return
				}
				if mode == "create failure" || mode == "creation reread failure" {
					if reads == 1 {
						http.Error(w, "not found", http.StatusNotFound)
					} else {
						http.Error(w, "reread unavailable", http.StatusServiceUnavailable)
					}
					return
				}
				if mode == "read error" {
					http.Error(w, "ref unavailable", http.StatusServiceUnavailable)
					return
				}
				kind, sha := "commit", githubtest.SHA
				switch mode {
				case "annotated", "deep annotation", "annotation error":
					kind = "tag"
					sha = githubtest.Base
				case "invalid type":
					kind = "tree"
				case "invalid commit":
					sha = "short"
				case "conflict":
					sha = githubtest.Base
				}
				githubtest.JSON(t, w, tagObject(kind, sha))
			})
			err := c.ensureTag("v1.234.0", githubtest.SHA)
			if mode == "lightweight" || mode == "annotated" {
				require.NoError(t, err)
				require.Zero(t, writes)
			} else {
				require.Error(t, err)
			}
			if mode == "deep annotation" {
				require.Equal(t, 9, reads)
			}
			if mode == "conflict" {
				require.ErrorIs(t, err, ErrMilestonePolicy)
				require.Zero(t, writes, "existing tag must never move")
			}
		})
	}
}

func preparedFixture() *preparedRelease {
	return &preparedRelease{Plan: Plan{Repository: githubtest.Repo, Milestone: Milestone{Number: 1, Title: "v1.234.0", State: "open"}, Version: "v1.234.0", Commit: githubtest.SHA, PreviousTag: "v1.233.0", Branch: "main", Mode: "cut", Entries: []releasenotes.PREntry{{Category: "## 🐛 Bug Fixes", Number: 42, Summary: "Fix credentials @contributor (#42)", Body: "Fixed credential injection."}}}, Tag: "v1.234.0", RunID: "77"}
}

func TestDraftReleaseCreationAndRetries(t *testing.T) {
	for _, mode := range []string{"cut", "candidate"} {
		t.Run(mode, func(t *testing.T) {
			p := preparedFixture()
			p.Mode = mode
			if mode == "candidate" {
				p.Tag += "-rc.77"
			}
			writes := 0
			created := false
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if strings.HasPrefix(path, "git/ref/tags/") {
					githubtest.JSON(t, w, tagObject("commit", p.Commit))
					return
				}
				require.Equal(t, "releases", path)
				release := map[string]any{"id": 99, "tag_name": p.Tag, "target_commitish": p.Commit, "draft": true}
				if r.Method == http.MethodGet {
					releases := []map[string]any{{"id": 98, "tag_name": "unrelated", "draft": true}}
					if created {
						releases = append(releases, release)
					}
					githubtest.JSON(t, w, releases)
					return
				}
				writes++
				require.Equal(t, http.MethodPost, r.Method)
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, p.Tag, body["tag_name"])
				require.Equal(t, p.Commit, body["target_commitish"])
				require.Equal(t, true, body["draft"])
				require.Equal(t, mode != "cut", body["prerelease"])
				require.Equal(t, "false", body["make_latest"])
				require.Contains(t, body["body"], "Fix credentials @contributor (#42)")
				require.Contains(t, body["body"], "## 🐛 Bug Fixes")
				created = true
				githubtest.JSON(t, w, release)
			})
			id, err := c.draft(p)
			require.NoError(t, err)
			require.EqualValues(t, 99, id)
			again, err := c.draft(p)
			require.NoError(t, err)
			require.Equal(t, id, again)
			require.Equal(t, 1, writes)
		})
	}
}

func TestDraftReleaseFailures(t *testing.T) {
	for _, mode := range []string{"repository mismatch", "invalid source", "tag API", "tag moved", "releases API", "published release", "wrong draft commit", "create API", "created wrong ID", "created wrong tag", "created wrong commit", "created published"} {
		t.Run(mode, func(t *testing.T) {
			p := preparedFixture()
			if mode == "repository mismatch" {
				p.Repository = "other/repo"
			}
			if mode == "invalid source" {
				p.Commit = "short"
			}
			writes := 0
			c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if strings.HasPrefix(path, "git/ref/tags/") {
					if mode == "tag API" {
						http.Error(w, "ref unavailable", http.StatusServiceUnavailable)
						return
					}
					sha := githubtest.SHA
					if mode == "tag moved" {
						sha = githubtest.Base
					}
					githubtest.JSON(t, w, tagObject("commit", sha))
					return
				}
				require.Equal(t, "releases", path)
				response := map[string]any{"id": 99, "tag_name": p.Tag, "target_commitish": p.Commit, "draft": true}
				if r.Method == http.MethodGet {
					switch mode {
					case "releases API":
						http.Error(w, "releases unavailable", http.StatusServiceUnavailable)
					case "published release":
						response["draft"] = false
						githubtest.JSON(t, w, []any{response})
					case "wrong draft commit":
						response["target_commitish"] = githubtest.Base
						githubtest.JSON(t, w, []any{response})
					default:
						githubtest.JSON(t, w, []any{})
					}
					return
				}
				writes++
				switch mode {
				case "create API":
					http.Error(w, "create unavailable", http.StatusServiceUnavailable)
					return
				case "created wrong ID":
					response["id"] = 0
				case "created wrong tag":
					response["tag_name"] = "v1.235.0"
				case "created wrong commit":
					response["target_commitish"] = githubtest.Base
				case "created published":
					response["draft"] = false
				}
				githubtest.JSON(t, w, response)
			})
			id, err := c.draft(p)
			require.Zero(t, id)
			require.Error(t, err)
			if !strings.HasPrefix(mode, "create") {
				require.Zero(t, writes)
			}
		})
	}
}

func TestReleaseOutputs(t *testing.T) {
	t.Setenv("GITHUB_OUTPUT", "")
	require.ErrorIs(t, releaseOutputs(map[string]string{"tag": "v1.234.0"}), ErrMilestonePolicy)
	output := filepath.Join(t.TempDir(), "output")
	t.Setenv("GITHUB_OUTPUT", output)
	require.Error(t, releaseOutputs(map[string]string{"tag": "v1.234.0"}))
	require.NoError(t, os.WriteFile(output, []byte("existing=value\n"), 0o600))
	require.NoError(t, releaseOutputs(map[string]string{"tag": "v1.234.0"}))
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "existing=value\ntag=v1.234.0\n", string(data))
	require.ErrorIs(t, releaseOutputs(map[string]string{"tag": "value\ninjected=value"}), ErrMilestonePolicy)
	data, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "existing=value\ntag=v1.234.0\n", string(data))
}

func TestReleaseLifecycleEntrypoints(t *testing.T) {
	t.Run("prepare saves exact plan and outputs", func(t *testing.T) {
		env := releaseEnvironment()
		for key, value := range env {
			t.Setenv(key, value)
		}
		path := filepath.Join(t.TempDir(), "plan.json")
		output := filepath.Join(t.TempDir(), "output")
		require.NoError(t, os.WriteFile(output, nil, 0o600))
		t.Setenv("RELEASE_PLAN_FILE", path)
		t.Setenv("GITHUB_OUTPUT", output)
		routes := planRoutes()
		milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			if r.URL.Path == "/users/osterman" {
				githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
				return
			}
			key := strings.TrimPrefix(r.URL.Path, planPrefix)
			if strings.HasPrefix(key, "git/ref/tags/") {
				githubtest.JSON(t, w, tagObject("commit", githubtest.SHA))
				return
			}
			value, ok := routes[key]
			require.True(t, ok)
			githubtest.JSON(t, w, value)
		})
		require.NoError(t, (Release{}).Prepare(githubtest.Repo))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var p preparedRelease
		require.NoError(t, json.Unmarshal(data, &p))
		require.Equal(t, "v1.234.0", p.Tag)
		require.Equal(t, githubtest.SHA, p.Commit)
		got, err := os.ReadFile(output)
		require.NoError(t, err)
		for _, line := range []string{"tag=v1.234.0\n", "commit=" + githubtest.SHA + "\n", "milestone=1\n", "mode=cut\n"} {
			require.Contains(t, string(got), line)
		}
		t.Setenv("RELEASE_PLAN_FILE", filepath.Join(t.TempDir(), "missing", "plan"))
		require.Error(t, (Release{}).Prepare(githubtest.Repo))
		t.Setenv("ATMOS_GUARDED_RELEASES_ENABLED", "false")
		require.ErrorIs(t, (Release{}).Prepare(githubtest.Repo), ErrMilestonePolicy)
	})
	t.Run("draft outputs identity", func(t *testing.T) {
		p := preparedFixture()
		path := filepath.Join(t.TempDir(), "plan.json")
		data, err := json.Marshal(p)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))
		output := filepath.Join(t.TempDir(), "output")
		require.NoError(t, os.WriteFile(output, nil, 0o600))
		t.Setenv("GITHUB_OUTPUT", output)
		milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/git/ref/tags/") {
				githubtest.JSON(t, w, tagObject("commit", p.Commit))
				return
			}
			githubtest.JSON(t, w, []any{map[string]any{"id": 99, "tag_name": p.Tag, "target_commitish": p.Commit, "draft": true}})
		})
		require.NoError(t, (Release{}).Draft(githubtest.Repo, path))
		data, err = os.ReadFile(output)
		require.NoError(t, err)
		require.Equal(t, "release-id=99\n", string(data))
		require.Error(t, (Release{}).Draft(githubtest.Repo, path+"-absent"))
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
		require.Error(t, (Release{}).Draft(githubtest.Repo, path))
		p.Repository = "other/repo"
		data, err = json.Marshal(p)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))
		require.ErrorIs(t, (Release{}).Draft(githubtest.Repo, path), ErrMilestonePolicy)
	})
	require.ErrorIs(t, (Release{}).Prepare("invalid"), ErrMilestonePolicy)
	require.ErrorIs(t, (Release{}).Draft("invalid", "anything"), ErrMilestonePolicy)
}

func savePrepared(t *testing.T, p *preparedRelease) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "prepared.json")
	data, err := json.Marshal(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, data, 0o600))
	return file
}

func TestPreparedIdentityValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*preparedRelease)
	}{
		{"invalid repository", func(p *preparedRelease) { p.Repository = "invalid" }},
		{"invalid SHA", func(p *preparedRelease) { p.Commit = "short" }},
		{"invalid milestone", func(p *preparedRelease) { p.Milestone.Number = 0 }},
		{"missing branch", func(p *preparedRelease) { p.Branch = "" }},
		{"unversioned milestone", func(p *preparedRelease) { p.Milestone.Title = "Backlog" }},
		{"version differs", func(p *preparedRelease) { p.Version = "v1.235.0" }},
		{"invalid run ID", func(p *preparedRelease) { p.RunID = "077" }},
		{"invalid mode", func(p *preparedRelease) { p.Mode = "invalid" }},
		{"tag differs", func(p *preparedRelease) { p.Tag = "v1.235.0" }},
		{"feature missing PR", func(p *preparedRelease) { p.Mode = "feature"; p.Entries = nil }},
		{"feature invalid PR", func(p *preparedRelease) { p.Mode = "feature"; p.Entries[0].Number = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := preparedFixture()
			tc.mutate(p)
			require.ErrorIs(t, validatePrepared(p), ErrMilestonePolicy)
		})
	}
	for _, mode := range []string{"cut", "candidate", "nightly", "feature"} {
		t.Run(mode, func(t *testing.T) {
			p := preparedFixture()
			p.Mode = mode
			switch mode {
			case "candidate":
				p.Tag += "-rc.77"
			case "nightly":
				p.Tag += "-nightly.77"
			case "feature":
				p.Tag += "-feature.42.77"
			}
			require.NoError(t, validatePrepared(p))
		})
	}
}

func TestValidateReleasePlanRechecksCurrentState(t *testing.T) {
	for _, mode := range []string{"cut", "candidate", "nightly", "feature"} {
		t.Run(mode, func(t *testing.T) {
			env := releaseEnvironment()
			env["RELEASE_MODE"] = mode
			for k, v := range env {
				t.Setenv(k, v)
			}
			p := preparedFixture()
			p.Mode = mode
			switch mode {
			case "candidate":
				p.Tag += "-rc.77"
			case "nightly":
				p.Tag += "-nightly.77"
			case "feature":
				p.Tag += "-feature.42.77"
			}
			routes := planRoutes()
			pr := routes["pulls/42"].(releasePull)
			pr.Head.SHA = githubtest.SHA
			routes["pulls/42"] = pr
			reads := []string{}
			milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				reads = append(reads, r.URL.Path)
				if r.URL.Path == "/users/osterman" {
					githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
					return
				}
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if strings.HasPrefix(path, "git/ref/tags/") {
					githubtest.JSON(t, w, tagObject("commit", p.Commit))
					return
				}
				value, ok := routes[path]
				require.True(t, ok, "unexpected %s", r.URL)
				githubtest.JSON(t, w, value)
			})
			require.NoError(t, (Release{}).ValidatePlan(githubtest.Repo, savePrepared(t, p)))
			require.Contains(t, reads, "/users/osterman")
			require.Contains(t, reads, planPrefix+"git/ref/tags/"+p.Tag)
			require.Contains(t, reads, planPrefix+"milestones")
		})
	}
}

func TestValidateReleasePlanRejectsTamperingAndChangedState(t *testing.T) {
	for _, mode := range []string{"invalid identity", "wrong repository", "wrong run", "wrong mode", "unauthorized actor", "live plan failure", "live version changed", "live source changed", "tag read fails", "tag moved", "missing file", "invalid JSON"} {
		t.Run(mode, func(t *testing.T) {
			env := releaseEnvironment()
			for k, v := range env {
				t.Setenv(k, v)
			}
			p := preparedFixture()
			routes := planRoutes()
			switch mode {
			case "invalid identity":
				p.Tag = "v9.0.0"
			case "wrong repository":
				p.Repository = "other/repo"
			case "wrong run":
				t.Setenv("GITHUB_RUN_ID", "78")
			case "wrong mode":
				t.Setenv("RELEASE_MODE", "candidate")
			case "unauthorized actor":
				t.Setenv("GITHUB_ACTOR_ID", "152489")
			case "live version changed":
				p.Version = "v1.235.0"
				p.Milestone.Title = "v1.235.0"
				p.Tag = "v1.235.0"
			case "live source changed":
				p.Mode = "feature"
				p.Tag += "-feature.42.77"
				t.Setenv("RELEASE_MODE", "feature")
				pr := routes["pulls/42"].(releasePull)
				pr.Head.SHA = githubtest.Base
				routes["pulls/42"] = pr
			}
			file := savePrepared(t, p)
			if mode == "missing file" {
				file += "-absent"
			}
			if mode == "invalid JSON" {
				require.NoError(t, os.WriteFile(file, []byte("{"), 0o600))
			}
			milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				if r.URL.Path == "/users/osterman" {
					githubtest.JSON(t, w, map[string]any{"id": 52489, "type": "User"})
					return
				}
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if strings.HasPrefix(path, "git/ref/tags/") {
					if mode == "tag read fails" {
						http.Error(w, "tag unavailable", http.StatusServiceUnavailable)
						return
					}
					sha := githubtest.SHA
					if mode == "tag moved" {
						sha = githubtest.Base
					}
					githubtest.JSON(t, w, tagObject("commit", sha))
					return
				}
				if mode == "live plan failure" && path == "milestones" {
					http.Error(w, "milestones unavailable", http.StatusServiceUnavailable)
					return
				}
				value, ok := routes[path]
				require.True(t, ok, "unexpected %s", r.URL)
				githubtest.JSON(t, w, value)
			})
			require.Error(t, (Release{}).ValidatePlan(githubtest.Repo, file))
		})
	}
	require.ErrorIs(t, (Release{}).ValidatePlan("invalid", "unused"), ErrMilestonePolicy)
}

func TestCompleteReleaseClosesOnlyPublishedStableMilestone(t *testing.T) {
	for _, mode := range []string{"close stable", "already closed", "candidate", "nightly", "feature"} {
		t.Run(mode, func(t *testing.T) {
			p := preparedFixture()
			switch mode {
			case "candidate":
				p.Mode = mode
				p.Tag += "-rc.77"
			case "nightly":
				p.Mode = mode
				p.Tag += "-nightly.77"
			case "feature":
				p.Mode = mode
				p.Tag += "-feature.42.77"
			}
			requests, writes := 0, 0
			milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				switch path {
				case "releases/tags/" + p.Tag:
					githubtest.JSON(t, w, map[string]any{"tag_name": p.Tag, "draft": false, "immutable": true, "prerelease": false})
				case "git/ref/tags/" + p.Tag:
					githubtest.JSON(t, w, tagObject("commit", p.Commit))
				case "milestones/1":
					if r.Method == http.MethodPatch {
						writes++
						var body map[string]string
						require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
						require.Equal(t, map[string]string{"state": "closed"}, body)
						githubtest.JSON(t, w, map[string]any{})
						return
					}
					m := p.Milestone
					if mode == "already closed" {
						m.State = "closed"
					}
					githubtest.JSON(t, w, m)
				default:
					t.Errorf("unexpected %s", r.URL)
				}
			})
			require.NoError(t, (Release{}).Complete(githubtest.Repo, savePrepared(t, p)))
			if mode == "close stable" {
				require.Equal(t, 1, writes)
			} else {
				require.Zero(t, writes)
			}
			if p.Mode != "cut" {
				require.Zero(t, requests, "prereleases must never close a milestone")
			}
		})
	}
}

func TestCompleteReleaseFailsWithoutProofOfPublication(t *testing.T) {
	for _, mode := range []string{"invalid plan", "wrong repository", "release API", "wrong tag", "still draft", "mutable release", "prerelease", "tag API", "tag moved", "milestone API", "milestone renamed", "close API", "missing file", "invalid JSON"} {
		t.Run(mode, func(t *testing.T) {
			p := preparedFixture()
			if mode == "invalid plan" {
				p.Commit = "bad"
			}
			if mode == "wrong repository" {
				p.Repository = "other/repo"
			}
			file := savePrepared(t, p)
			if mode == "missing file" {
				file += "-absent"
			}
			if mode == "invalid JSON" {
				require.NoError(t, os.WriteFile(file, []byte("{"), 0o600))
			}
			writes := 0
			milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, planPrefix)
				if r.Method != http.MethodGet {
					writes++
					require.Equal(t, "close API", mode)
					http.Error(w, "close unavailable", http.StatusServiceUnavailable)
					return
				}
				switch path {
				case "releases/tags/" + p.Tag:
					if mode == "release API" {
						http.Error(w, "release unavailable", http.StatusServiceUnavailable)
						return
					}
					body := map[string]any{"tag_name": p.Tag, "draft": false, "immutable": true, "prerelease": false}
					switch mode {
					case "wrong tag":
						body["tag_name"] = "v9.0.0"
					case "still draft":
						body["draft"] = true
					case "mutable release":
						body["immutable"] = false
					case "prerelease":
						body["prerelease"] = true
					}
					githubtest.JSON(t, w, body)
				case "git/ref/tags/" + p.Tag:
					if mode == "tag API" {
						http.Error(w, "tag unavailable", http.StatusServiceUnavailable)
						return
					}
					sha := githubtest.SHA
					if mode == "tag moved" {
						sha = githubtest.Base
					}
					githubtest.JSON(t, w, tagObject("commit", sha))
				case "milestones/1":
					if mode == "milestone API" {
						http.Error(w, "milestone unavailable", http.StatusServiceUnavailable)
						return
					}
					m := p.Milestone
					if mode == "milestone renamed" {
						m.Title = "v1.235.0"
					}
					githubtest.JSON(t, w, m)
				default:
					t.Errorf("unexpected %s", r.URL)
				}
			})
			require.Error(t, (Release{}).Complete(githubtest.Repo, file))
			if mode != "close API" {
				require.Zero(t, writes)
			}
		})
	}
	require.ErrorIs(t, (Release{}).Complete("invalid", "unused"), ErrMilestonePolicy)
}

func TestWorkflowOutputFilesCannotFollowEscapingSymlinks(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "unchanged")
	require.NoError(t, os.WriteFile(outside, []byte("preserve"), 0o600))
	for _, output := range []string{"plan", "github outputs"} {
		t.Run(output, func(t *testing.T) {
			link := filepath.Join(t.TempDir(), "link")
			require.NoError(t, os.Symlink(outside, link))
			if output == "plan" {
				require.Error(t, writePreparedPlan(link, []byte("replacement")))
			} else {
				t.Setenv("GITHUB_OUTPUT", link)
				require.Error(t, releaseOutputs(map[string]string{"tag": "v1.234.0"}))
			}
			data, err := os.ReadFile(outside)
			require.NoError(t, err)
			require.Equal(t, "preserve", string(data))
		})
	}
	t.Setenv("GITHUB_OUTPUT", filepath.Join(t.TempDir(), "missing", "output"))
	require.Error(t, releaseOutputs(map[string]string{"tag": "v1.234.0"}))
}
