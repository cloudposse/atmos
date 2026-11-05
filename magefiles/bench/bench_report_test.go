//go:build mage

package bench

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func TestBenchReportRunValidationAndPublication(t *testing.T) {
	for _, scenario := range []string{"success", "fork", "base advanced", "cached base behind", "wrong merge", "failure", "stale", "invalid run", "missing artifact", "malformed", "wrong provenance", "base not on main", "base ahead of main", "ancestry API error", "API error", "publish error"} {
		t.Run(scenario, func(t *testing.T) {
			benchSetUpRun(t)
			run := githubtest.Run()
			run.Event = "pull_request"
			if scenario == "fork" {
				run.HeadRepository.FullName = "contributor/atmos"
			}
			pr := githubtest.PR(run)
			if scenario == "base advanced" {
				pr.Base.SHA = strings.Repeat("d", 40)
				pr.MergeSHA = strings.Repeat("e", 40)
			}
			if scenario == "cached base behind" {
				pr.Base.SHA = strings.Repeat("f", 40)
			}
			if scenario == "failure" {
				run.Conclusion = "failure"
			}
			if scenario == "stale" {
				pr.Head.SHA = githubtest.Base
			}
			if scenario == "invalid run" {
				run.Event = "push"
			}
			archive := benchSnapshotArchive(t, run)
			if scenario == "malformed" {
				archive = []byte("not a zip")
			}
			if scenario == "wrong provenance" {
				other := run
				other.Attempt++
				archive = benchSnapshotArchive(t, other)
			}
			fake := &githubtest.FakeAPI{T: t, Archive: archive}
			fake.Get = func(path string, response any) error {
				switch {
				case strings.HasSuffix(path, "actions/runs/10"):
					if scenario == "API error" {
						return errors.New("unavailable")
					}
					return githubtest.Respond(run, response)
				case strings.Contains(path, "/commits/") && strings.Contains(path, "/pulls?"):
					require.Contains(t, path, "/commits/"+run.HeadSHA+"/pulls?", "association must use the PR head, not the merge")
					return githubtest.Respond([]githubapi.PR{pr}, response)
				case strings.Contains(path, "/pulls?"):
					// The head-branch fallback runs when the listed PR does not match.
					require.Equal(t, "stale", scenario, "only a moved head needs the fallback")
					return githubtest.Respond([]githubapi.PR{pr}, response)
				case strings.Contains(path, "/git/commits/"):
					require.True(t, strings.HasSuffix(path, "/"+benchTestMerge))
					head := run.HeadSHA
					if scenario == "wrong merge" {
						head = githubtest.Base
					}
					return githubtest.Respond(map[string]any{"sha": benchTestMerge, "parents": []map[string]string{{"sha": githubtest.Base}, {"sha": head}}}, response)
				case strings.Contains(path, "/artifacts?"):
					artifacts := []benchArtifact{{ID: 1, Name: benchArtifactName(&run)}}
					if scenario == "missing artifact" {
						artifacts = nil
					}
					return githubtest.Respond(map[string]any{"artifacts": artifacts}, response)
				case strings.Contains(path, "/compare/"):
					status := "identical"
					if scenario == "ancestry API error" {
						return errors.New("comparison unavailable")
					}
					if scenario == "base not on main" {
						status = "diverged"
					}
					if scenario == "base ahead of main" {
						status = "ahead"
					}
					if scenario == "base advanced" {
						status = "behind"
					}
					// GitHub may cache a PR base older than the measured merge's
					// first parent, even though that parent is on current main.
					if scenario == "cached base behind" && strings.Contains(path, pr.Base.SHA+"...") {
						status = "ahead"
					} else {
						require.Equal(t, "repos/"+githubtest.Repo+"/compare/main..."+githubtest.Base, path)
					}
					return githubtest.Respond(map[string]string{"status": status}, response)
				case strings.Contains(path, "/workflows/"):
					return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{}}, response)
				default:
					t.Fatalf("unexpected path %s", path)
					return nil
				}
			}
			published := 0
			err := ReportRun(context.Background(), fake, githubtest.Repo, 10, func(number int, body string, reported *githubapi.Run) error {
				published++
				require.Equal(t, 42, number)
				require.Equal(t, run.ID, reported.ID)
				require.Equal(t, pr.Head.SHA, reported.HeadSHA)
				require.Contains(t, body, "Comparison unavailable")
				require.Contains(t, body, "<!-- atmos-benchmark-run:10:2 -->")
				if scenario == "failure" {
					require.Contains(t, body, "execution failure")
				}
				if scenario == "publish error" {
					return errors.New("write failed")
				}
				return nil
			})
			switch scenario {
			case "success", "fork", "base advanced", "cached base behind", "failure":
				require.NoError(t, err)
				require.Equal(t, 1, published)
			case "stale":
				require.NoError(t, err)
				require.Zero(t, published)
			default:
				require.Error(t, err)
				if scenario != "publish error" {
					require.Zero(t, published, "invalid or unverifiable artifacts must not be published")
				}
			}
		})
	}
}

// GitHub's commit association endpoint returns [] for a commit that exists only
// in a fork, so the PR must be resolved through the fork's head branch.
func TestBenchAssociatedPRsResolveForkOnlyCommits(t *testing.T) {
	const forkOwner, forkBranch = "contributor", "fix/example-branch"
	cases := []struct {
		name        string
		commitPRs   func(pr githubapi.PR) []githubapi.PR
		headPRs     func(pr githubapi.PR) []githubapi.PR
		wantNumbers []int
		wantHead    bool
	}{
		{
			name:        "fork-only commit is found through the head branch",
			commitPRs:   func(githubapi.PR) []githubapi.PR { return []githubapi.PR{} },
			headPRs:     func(pr githubapi.PR) []githubapi.PR { return []githubapi.PR{pr} },
			wantNumbers: []int{42},
			wantHead:    true,
		},
		{
			name:        "fallback is not used when the commit endpoint lists the PR",
			commitPRs:   func(pr githubapi.PR) []githubapi.PR { return []githubapi.PR{pr} },
			headPRs:     func(pr githubapi.PR) []githubapi.PR { return []githubapi.PR{pr} },
			wantNumbers: []int{42},
		},
		{
			name:      "head that moved after the run is not associated",
			commitPRs: func(githubapi.PR) []githubapi.PR { return []githubapi.PR{} },
			headPRs: func(pr githubapi.PR) []githubapi.PR {
				pr.Head.SHA = githubtest.Base
				return []githubapi.PR{pr}
			},
			wantHead: true,
		},
		{
			name:      "PR on a non-main base is not associated",
			commitPRs: func(githubapi.PR) []githubapi.PR { return []githubapi.PR{} },
			headPRs: func(pr githubapi.PR) []githubapi.PR {
				pr.Base.Ref = "release"
				return []githubapi.PR{pr}
			},
			wantHead: true,
		},
		{
			name:      "closed PR is not associated",
			commitPRs: func(githubapi.PR) []githubapi.PR { return []githubapi.PR{} },
			headPRs: func(pr githubapi.PR) []githubapi.PR {
				pr.State = "closed"
				return []githubapi.PR{pr}
			},
			wantHead: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := githubtest.Run()
			run.Event, run.Branch = "pull_request", forkBranch
			run.HeadRepository.FullName = forkOwner + "/atmos"
			pr := githubtest.PR(run)
			headCalls := 0
			fake := &githubtest.FakeAPI{T: t, Get: func(path string, response any) error {
				switch {
				case strings.Contains(path, "/commits/"+run.HeadSHA+"/pulls?"):
					return githubtest.Respond(tc.commitPRs(pr), response)
				case strings.Contains(path, "/pulls?"):
					headCalls++
					parsed, err := url.Parse(path)
					require.NoError(t, err)
					require.Equal(t, "repos/"+githubtest.Repo+"/pulls", parsed.Path)
					require.Equal(t, "open", parsed.Query().Get("state"))
					require.Equal(t, "main", parsed.Query().Get("base"))
					require.Equal(t, forkOwner+":"+forkBranch, parsed.Query().Get("head"))
					return githubtest.Respond(tc.headPRs(pr), response)
				}
				t.Fatalf("unexpected path %s", path)
				return nil
			}}
			prs, err := benchAssociatedPRs(context.Background(), fake, githubtest.Repo, &run)
			require.NoError(t, err)
			var got []int
			for i := range prs {
				if PRMatches(&prs[i], &run, githubtest.Repo) {
					got = append(got, prs[i].Number)
				}
			}
			require.Equal(t, tc.wantNumbers, got)
			require.Equal(t, tc.wantHead, headCalls == 1, "head-branch lookups: %d", headCalls)
		})
	}
}

// The reporter must publish exactly once for a fork-only commit even though the
// commit association endpoint returns nothing.
func TestBenchReportRunPublishesForkOnlyCommit(t *testing.T) {
	benchSetUpRun(t)
	run := githubtest.Run()
	run.Event, run.Branch = "pull_request", "fix/example-branch"
	run.Conclusion = "failure"
	run.HeadRepository.FullName = "contributor/atmos"
	pr := githubtest.PR(run)
	fake := &githubtest.FakeAPI{T: t, Get: func(path string, response any) error {
		switch {
		case strings.HasSuffix(path, "actions/runs/10"):
			return githubtest.Respond(run, response)
		case strings.Contains(path, "/commits/"+run.HeadSHA+"/pulls?"):
			return githubtest.Respond([]githubapi.PR{}, response)
		case strings.Contains(path, "/pulls?") && strings.Contains(path, "head=contributor%3Afix%2Fexample-branch"):
			return githubtest.Respond([]githubapi.PR{pr}, response)
		}
		t.Fatalf("unexpected path %s", path)
		return nil
	}}
	var numbers []int
	err := ReportRun(context.Background(), fake, githubtest.Repo, 10, func(number int, body string, _ *githubapi.Run) error {
		numbers = append(numbers, number)
		require.Contains(t, body, "execution failure")
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []int{42}, numbers)
}

func TestBenchPRAndArtifactPaginationErrors(t *testing.T) {
	run := githubtest.Run()
	fake := &githubtest.FakeAPI{T: t, Get: func(_ string, _ any) error { return errors.New("API unavailable") }}
	_, err := benchAssociatedPRs(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "API unavailable")
	_, err = benchFindArtifact(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "API unavailable")
	fake.Get = func(_ string, response any) error { return githubtest.Respond(make([]githubapi.PR, 100), response) }
	_, err = benchAssociatedPRs(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "pagination limit")
	fake.Get = func(_ string, response any) error {
		return githubtest.Respond(map[string]any{"artifacts": make([]benchArtifact, 100)}, response)
	}
	_, err = benchFindArtifact(context.Background(), fake, githubtest.Repo, &run)
	require.ErrorContains(t, err, "pagination limit")
}

func TestBenchMergeProvenance(t *testing.T) {
	for _, scenario := range []string{"valid", "old head-only artifact", "unmerged head", "wrong head", "invalid SHA", "invalid base", "wrong commit", "non-merge", "extra parent", "wrong base parent", "wrong head parent", "API failure"} {
		t.Run(scenario, func(t *testing.T) {
			run := githubtest.Run()
			run.Event = "pull_request"
			m := benchTestSnapshot(100).Manifest
			m.Repository, m.RunID, m.Attempt, m.Event = githubtest.Repo, "10", "2", run.Event
			m.SHA, m.BaseSHA, m.PRHeadSHA = benchTestMerge, githubtest.Base, run.HeadSHA
			sha, parents := benchTestMerge, []map[string]string{{"sha": githubtest.Base}, {"sha": run.HeadSHA}}
			switch scenario {
			case "old head-only artifact":
				m.SHA, m.PRHeadSHA = run.HeadSHA, ""
			case "unmerged head":
				m.SHA = run.HeadSHA
			case "wrong head":
				m.PRHeadSHA = githubtest.Base
			case "invalid SHA":
				m.SHA = "../../untrusted"
			case "invalid base":
				m.BaseSHA = "main"
			case "wrong commit":
				sha = githubtest.Base
			case "non-merge":
				parents = parents[:1]
			case "extra parent":
				parents = append(parents, map[string]string{"sha": githubtest.SHA})
			case "wrong base parent":
				parents[0]["sha"] = run.HeadSHA
			case "wrong head parent":
				parents[1]["sha"] = githubtest.Base
			}
			calls := 0
			fake := &githubtest.FakeAPI{T: t, Get: func(path string, response any) error {
				calls++
				require.Equal(t, "repos/"+githubtest.Repo+"/git/commits/"+benchTestMerge, path)
				if scenario == "API failure" {
					return errors.New("GitHub unavailable")
				}
				return githubtest.Respond(map[string]any{"sha": sha, "parents": parents}, response)
			}}
			err := benchValidateCandidate(context.Background(), fake, githubtest.Repo, &m, &run)
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, 1, calls)
				return
			}
			require.Error(t, err)
			if scenario == "old head-only artifact" || scenario == "unmerged head" || scenario == "wrong head" || scenario == "invalid SHA" || scenario == "invalid base" {
				require.Zero(t, calls, "reject malformed provenance before making API requests")
			}
		})
	}
}

func TestBenchArchiveRejectsBeforeWriting(t *testing.T) {
	for _, scenario := range []string{"traversal", "duplicate", "symlink", "oversized", "malformed", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			var raw bytes.Buffer
			writer := zip.NewWriter(&raw)
			first, err := writer.Create("manifest.json")
			require.NoError(t, err)
			_, err = first.Write([]byte("{}"))
			require.NoError(t, err)
			name := "bench.txt"
			if scenario == "traversal" {
				name = "../outside"
			}
			if scenario == "duplicate" {
				name = "manifest.json"
			}
			if scenario != "missing" {
				header := &zip.FileHeader{Name: name, Method: zip.Deflate}
				if scenario == "symlink" {
					header.SetMode(os.ModeSymlink | 0o777)
				}
				entry, err := writer.CreateHeader(header)
				require.NoError(t, err)
				contents := []byte("data")
				if scenario == "oversized" {
					contents = bytes.Repeat([]byte("x"), benchArchiveLimit/2+1)
				}
				_, err = entry.Write(contents)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			data := raw.Bytes()
			if scenario == "malformed" {
				data = data[:10]
			}
			dir := t.TempDir()
			require.Error(t, benchExtract(data, dir))
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "manifest.json"), 0o700))
	require.Error(t, benchExtract(benchSnapshotArchive(t, githubtest.Run()), dir))
}

func TestBenchFetchRejectsInvalidDestinationsAndInputs(t *testing.T) {
	candidate := benchTestSnapshot(100)
	for _, repo := range []string{"invalid", githubtest.Repo} {
		err := benchFetch(context.Background(), nil, repo, "bad sha", t.TempDir(), candidate)
		require.ErrorContains(t, err, "immutable")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing"), nil, 0o600))
	require.ErrorContains(t, benchFetch(context.Background(), nil, githubtest.Repo, githubtest.Base, dir, candidate), "not empty")
	require.Error(t, benchFetch(context.Background(), nil, githubtest.Repo, githubtest.Base, filepath.Join(dir, "existing"), candidate))
	require.Error(t, (Bench{}).Fetch(githubtest.Repo, githubtest.Base, filepath.Join(dir, "snapshots")))
}

func TestBenchFetchHistorySearchBound(t *testing.T) {
	candidate := benchTestSnapshot(100)
	fake := &githubtest.FakeAPI{T: t, Get: func(_ string, response any) error {
		return githubtest.Respond(map[string]any{"workflow_runs": make([]githubapi.Run, 100)}, response)
	}}
	dir := t.TempDir()
	require.NoError(t, benchFetch(context.Background(), fake, githubtest.Repo, githubtest.Base, dir, candidate))
	data, err := os.ReadFile(filepath.Join(dir, "unavailable.json"))
	require.NoError(t, err)
	require.Contains(t, string(data), "1,000 runs")
}
