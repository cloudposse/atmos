//go:build mage

package release

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

// rehearseReleaseLifecycle connects the production Go planner/verifier and CJS
// publisher to one stateful GitHub mock, using actual GoReleaser-built artifacts.
// Signing and attestation cryptography have separate tests; no OIDC is faked here.
func rehearseReleaseLifecycle(t *testing.T, artifacts, tag, commit string) {
	t.Helper()
	_, err := exec.LookPath("node")
	require.NoError(t, err, "the publication rehearsal requires Node.js")
	cases := []string{"success", "interrupted upload retry"}
	if !strings.Contains(tag, "-") {
		cases = append(cases, "wrong binary version", "wrong binary SHA", "moved milestone", "incompatible label", "unmerged PR", "tag moved", "draft renamed", "upload corrupted", "extra asset", "local asset changed", "downstream tampered")
	}
	for _, scenario := range cases {
		t.Run("mock lifecycle/"+scenario, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.CopyFS(dir, os.DirFS(artifacts)))
			state := newRehearsalState(t, tag, commit)
			endpoint := githubtest.Server(t, state.serve)
			client, err := newMilestoneClient(context.Background(), githubtest.Repo)
			require.NoError(t, err)
			env := releaseEnvironment()
			env["GITHUB_RUN_ID"], env["RELEASE_COMMIT"] = "1", commit
			env["RELEASE_VERSION"], env["RELEASE_MODE"] = state.version, state.mode
			env["RELEASE_PR"] = "42"
			plan, err := client.prepare(environmentReader(env))
			require.NoError(t, err)
			require.Equal(t, tag, plan.Tag)
			require.Nil(t, state.release, "tag reservation must precede draft creation")
			require.Equal(t, commit, state.tag)

			if scenario == "wrong binary version" || scenario == "wrong binary SHA" {
				corruptRehearsalBinary(t, dir, tag, commit, scenario)
				err = (Release{}).VerifyArtifacts(dir, tag, commit)
				require.ErrorContains(t, err, "binary version/commit identity mismatch")
				require.Zero(t, state.published)
				require.Nil(t, state.release, "bad build must not even reach draft creation")
				return
			}
			require.NoError(t, (Release{}).VerifyBuild(dir, tag, commit))
			require.NoError(t, (Release{}).VerifyArtifacts(dir, tag, commit))
			id, err := client.draft(plan)
			require.NoError(t, err)
			require.EqualValues(t, 99, id)
			state.inject(scenario, dir)
			err = client.validateLivePlan(plan, environmentReader(env))
			if slices.Contains([]string{"moved milestone", "incompatible label", "unmerged PR", "tag moved"}, scenario) {
				require.Error(t, err)
				require.Zero(t, state.published)
				require.Empty(t, state.assets)
				return
			}
			require.NoError(t, err)
			err = runMockPublisher(t, endpoint, dir, tag, commit, "publish")
			if scenario == "interrupted upload retry" {
				require.Error(t, err)
				require.Zero(t, state.published)
				require.Len(t, state.assets, 1)
				require.NoError(t, client.validateLivePlan(plan, environmentReader(env)))
				reused, draftErr := client.draft(plan)
				require.NoError(t, draftErr)
				require.Equal(t, id, reused, "retry must reuse the same draft and release identity")
				err = runMockPublisher(t, endpoint, dir, tag, commit, "publish")
			}
			if slices.Contains([]string{"draft renamed", "upload corrupted", "extra asset", "local asset changed"}, scenario) {
				require.Error(t, err)
				require.Zero(t, state.published)
				require.Equal(t, true, state.release["draft"])
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, state.published)
			require.Equal(t, commit, state.release["target_commitish"])
			require.Equal(t, !strings.Contains(tag, "-"), state.release["make_latest"] == "true")
			if scenario == "downstream tampered" {
				state.assets[0]["digest"] = "sha256:" + strings.Repeat("0", 64)
				require.Error(t, runMockPublisher(t, endpoint, dir, tag, commit, "verify"))
				return
			}
			planFile := filepath.Join(t.TempDir(), "plan.json")
			data, err := json.Marshal(plan)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(planFile, data, 0o600))
			require.NoError(t, (Release{}).Complete(githubtest.Repo, planFile))
			require.Equal(t, state.mode == releaseModeCut, state.closed)
			require.Len(t, state.assets, state.assetCount, "retry must neither duplicate nor omit assets")
		})
	}
}

func runMockPublisher(t *testing.T, endpoint, dir, tag, commit, operation string) error {
	t.Helper()
	cmd := exec.Command("node", "testdata/publisher-rehearsal.cjs", endpoint, dir, tag, commit, operation)
	// No real credentials are inherited by the child transport.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("publisher: %w: %s", err, output)
	}
	return nil
}

func corruptRehearsalBinary(t *testing.T, dir, tag, commit, scenario string) {
	t.Helper()
	name := "atmos_" + strings.TrimPrefix(tag, "v") + "_linux_amd64"
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	old := "atmos-release:" + tag + ":" + commit + ":end"
	altered := strings.Replace(old, commit, strings.Repeat("b", 40), 1)
	if scenario == "wrong binary version" {
		altered = strings.Replace(old, tag, "v1.234.0", 1)
	}
	require.Equal(t, len(old), len(altered), "replace only the embedded marker, preserve the executable")
	require.Contains(t, string(data), old)
	data = []byte(strings.ReplaceAll(string(data), old, altered))
	require.NoError(t, os.WriteFile(path, data, 0o600))
	// Recompute the checksum: identity verification must catch the mismatch even
	// when the binary's name, package metadata and checksums look consistent.
	checksums := filepath.Join(dir, "atmos_"+tag[1:]+"_SHA256SUMS")
	contents, err := os.ReadFile(checksums)
	require.NoError(t, err)
	lines := strings.Split(string(contents), "\n")
	for i, line := range lines {
		if strings.HasSuffix(line, "  "+name) {
			lines[i] = fmt.Sprintf("%x  %s", sha256.Sum256(data), name)
		}
	}
	require.NoError(t, os.WriteFile(checksums, []byte(strings.Join(lines, "\n")), 0o600))
}

type rehearsalState struct {
	t                              *testing.T
	mu                             sync.Mutex
	routes                         map[string]any
	tag, version, mode, fault      string
	release                        map[string]any
	assets                         []map[string]any
	assetBytes                     map[int][]byte
	published, uploads, assetCount int
	closed                         bool
	candidate                      *candidateAPI
}

func newRehearsalState(t *testing.T, tag, commit string) *rehearsalState {
	t.Helper()
	routes := planRoutes()
	data, err := json.Marshal(routes)
	require.NoError(t, err)
	data = []byte(strings.ReplaceAll(string(data), githubtest.SHA, commit))
	version := strings.Split(tag, "-")[0]
	data = []byte(strings.ReplaceAll(string(data), "v1.233.0", "v1.232.0"))
	data = []byte(strings.ReplaceAll(string(data), "1.234.0", version[1:]))
	require.NoError(t, json.Unmarshal(data, &routes))
	s := &rehearsalState{t: t, routes: routes, version: version, mode: releaseModeCut, assetBytes: map[int][]byte{}}
	switch {
	case strings.Contains(tag, "-rc."):
		s.mode = releaseModeCandidate
		s.candidate = newCandidateAPI(t)
	case strings.Contains(tag, "-nightly."):
		s.mode = releaseModeNightly
	case strings.Contains(tag, "-feature."):
		s.mode = releaseModeFeature
		pr := routes["pulls/42"].(map[string]any)
		pr["head"] = map[string]string{"sha": commit}
		pr["state"] = "open"
	}
	return s
}

func (s *rehearsalState) inject(scenario, dir string) {
	s.fault = scenario
	data, err := os.ReadFile(filepath.Join(dir, "release-manifest.json"))
	require.NoError(s.t, err)
	var manifest ArtifactManifest
	require.NoError(s.t, json.Unmarshal(data, &manifest))
	s.assetCount = len(manifest.Assets) + 1
	pr := s.routes["pulls/42"].(map[string]any)
	switch scenario {
	case "moved milestone":
		s.routes["milestones"].([]any)[1].(map[string]any)["title"] = "v1.234.1"
	case "incompatible label":
		pr["labels"] = []map[string]string{{"name": "major"}}
	case "unmerged PR":
		pr["merged"] = false
	case "tag moved":
		s.tag = githubtest.Base
	case "draft renamed":
		s.release["tag_name"] = "v1.234.0"
	case "extra asset":
		s.assets = append(s.assets, map[string]any{"name": "stale", "state": "uploaded", "digest": "sha256:bad"})
	case "local asset changed":
		require.NoError(s.t, os.WriteFile(filepath.Join(dir, "atmos-sbom.spdx.json"), []byte("changed"), 0o600))
	}
}

func (s *rehearsalState) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/__publisher/") {
		s.publisher(w, r)
		return
	}
	path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(planPrefix, "/")), "/")
	if s.candidate != nil && s.candidate.serve(w, r) {
		for _, sha := range s.candidate.refs {
			s.tag = s.candidate.objects[sha].Object.SHA
		}
		return
	}
	switch {
	case r.URL.Path == "/users/osterman":
		githubtest.JSON(s.t, w, map[string]any{"id": 52489, "type": "User"})
	case strings.HasPrefix(path, "git/ref/tags/"):
		if s.tag == "" {
			http.NotFound(w, r)
			return
		}
		githubtest.JSON(s.t, w, tagObject("commit", s.tag))
	case path == "git/refs":
		var ref struct {
			SHA string `json:"sha"`
		}
		require.NoError(s.t, json.NewDecoder(r.Body).Decode(&ref))
		s.tag = ref.SHA
		githubtest.JSON(s.t, w, map[string]any{})
	case path == "releases" && r.Method == http.MethodPost:
		require.NoError(s.t, json.NewDecoder(r.Body).Decode(&s.release))
		s.release["id"] = 99
		githubtest.JSON(s.t, w, s.release)
	case strings.HasPrefix(path, "releases/tags/"):
		githubtest.JSON(s.t, w, s.release)
	case path == "milestones/1":
		m := s.routes["milestones"].([]any)[1].(map[string]any)
		if r.Method == http.MethodPatch {
			require.Equal(s.t, 1, s.published)
			s.closed = true
			m["state"] = "closed"
		}
		githubtest.JSON(s.t, w, m)
	default:
		require.Equal(s.t, http.MethodGet, r.Method)
		value, ok := s.routes[path]
		if !ok {
			s.t.Errorf("unexpected mock request %s", r.URL)
			http.NotFound(w, r)
			return
		}
		if path == "releases" && s.release != nil {
			value = append(append([]any{}, value.([]any)...), s.release)
		}
		githubtest.JSON(s.t, w, value)
	}
}

func (s *rehearsalState) publisher(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	require.NoError(s.t, json.NewDecoder(r.Body).Decode(&body))
	var response any
	switch strings.TrimPrefix(r.URL.Path, "/__publisher/") {
	case "getRef":
		response = tagObject("commit", s.tag)
	case "getRelease":
		response = s.release
	case "listReleaseAssets":
		response = s.assets
		if s.assets == nil {
			response = []any{}
		}
	case "uploadReleaseAsset":
		s.uploads++
		if s.fault == "interrupted upload retry" && s.uploads == 2 {
			http.Error(w, "interrupted", http.StatusServiceUnavailable)
			return
		}
		data, err := base64.StdEncoding.DecodeString(body["data"].(string))
		require.NoError(s.t, err)
		id := len(s.assets) + 1
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		if s.fault == "upload corrupted" {
			digest = "sha256:" + strings.Repeat("0", 64)
		}
		asset := map[string]any{"id": id, "name": body["name"], "state": "uploaded", "digest": digest}
		s.assets = append(s.assets, asset)
		s.assetBytes[id] = data
		response = asset
	case "updateRelease":
		require.Equal(s.t, false, body["draft"])
		s.published++
		s.release["draft"] = false
		s.release["immutable"] = true
		s.release["make_latest"] = body["make_latest"]
		response = s.release
	case "getReleaseAsset":
		response = base64.StdEncoding.EncodeToString(s.assetBytes[int(body["asset_id"].(float64))])
	default:
		s.t.Errorf("unexpected publisher operation %s", r.URL.Path)
		http.NotFound(w, r)
		return
	}
	githubtest.JSON(s.t, w, response)
}
