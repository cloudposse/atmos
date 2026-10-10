package bench

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

const benchArchiveLimit = 32 << 20

type benchArtifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Expired bool   `json:"expired"`
}

func benchArtifactName(run *githubapi.Run) string {
	return fmt.Sprintf("benchmarks-%d-%d", run.ID, run.Attempt)
}

func benchFindArtifact(ctx context.Context, client githubapi.API, repo string, run *githubapi.Run) (*benchArtifact, error) {
	for page := 1; page <= githubapi.PageLimit; page++ {
		var response struct {
			Artifacts []benchArtifact `json:"artifacts"`
		}
		path := fmt.Sprintf("repos/%s/actions/runs/%d/artifacts?per_page=100&page=%d", repo, run.ID, page)
		if err := githubapi.Get(ctx, client, path, &response); err != nil {
			return nil, err
		}
		for _, artifact := range response.Artifacts {
			if artifact.Name == benchArtifactName(run) && !artifact.Expired {
				return &artifact, nil
			}
		}
		if len(response.Artifacts) < githubapi.PageSize {
			return nil, nil
		}
	}
	return nil, Error("artifact pagination limit exceeded")
}

// Extract exactly the two data files, never archive paths, executables or links.
//
//nolint:revive // Every archive invariant is checked before writing either allowed file.
func benchExtract(data []byte, dir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, file := range reader.File {
		if file.Name != "manifest.json" && file.Name != "bench.txt" {
			return Error("unexpected benchmark artifact entry: %q", file.Name)
		}
		if seen[file.Name] || !file.Mode().IsRegular() || file.UncompressedSize64 > benchArchiveLimit/2 {
			return Error("invalid benchmark archive entry")
		}
		seen[file.Name] = true
		contents, err := benchReadArchiveEntry(file)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, file.Name), contents, PrivateFile); err != nil { // #nosec G305 -- Entry name is restricted above to two literal filenames.
			return err
		}
	}
	if !seen["manifest.json"] || !seen["bench.txt"] {
		return Error("benchmark artifact is missing measurements or manifest")
	}
	return nil
}

func benchReadArchiveEntry(file *zip.File) ([]byte, error) {
	stream, openErr := file.Open()
	if openErr != nil {
		return nil, openErr
	}
	contents, readErr := io.ReadAll(io.LimitReader(stream, benchArchiveLimit/2+1))
	closeErr := stream.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(contents) > benchArchiveLimit/2 {
		return nil, Error("benchmark entry too large")
	}
	return contents, nil
}

func benchDownload(ctx context.Context, client githubapi.API, repo string, artifact benchArtifact, dir string) error {
	response, err := client.RequestWithContext(ctx, http.MethodGet, fmt.Sprintf("repos/%s/actions/artifacts/%d/zip", repo, artifact.ID), nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, benchArchiveLimit+1))
	if err != nil {
		return err
	}
	if len(data) > benchArchiveLimit {
		return Error("benchmark archive too large")
	}
	return benchExtract(data, dir)
}

func benchAncestor(ctx context.Context, client githubapi.API, repo, ref, sha string) (bool, error) {
	var response struct {
		Status string `json:"status"`
	}
	if err := githubapi.Get(ctx, client, "repos/"+repo+"/compare/"+url.PathEscape(ref)+"..."+url.PathEscape(sha), &response); err != nil {
		return false, err
	}
	return response.Status == "identical" || response.Status == "behind", nil
}

// Fetch retrieves up to ten compatible successful main snapshots. Candidate
// metadata defaults to the sibling candidate directory; override with
// ATMOS_BENCH_CANDIDATE_DIR when using another local layout.
func (Bench) Fetch(repo, ref, outputDir string) error {
	candidateDir := os.Getenv("ATMOS_BENCH_CANDIDATE_DIR")
	if candidateDir == "" {
		candidateDir = filepath.Join(filepath.Dir(outputDir), "candidate")
	}
	candidate, err := benchLoad(candidateDir)
	if err != nil {
		return err
	}
	client, err := api.DefaultRESTClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if candidate.Manifest.RunID != "" {
		var run githubapi.Run
		path := fmt.Sprintf("repos/%s/actions/runs/%s", repo, candidate.Manifest.RunID)
		if err = githubapi.Get(ctx, client, path, &run); err != nil {
			return err
		}
		if err = benchValidateRun(&candidate.Manifest, repo, &run); err != nil {
			return err
		}
		candidate.Cutoff = run.Created
	}
	return benchFetch(ctx, client, repo, ref, outputDir, candidate)
}

//nolint:revive // Explicit download dependencies keep the mocked API boundary small.
func benchFetch(ctx context.Context, client githubapi.API, repo, ref, outputDir string, candidate *benchSnapshot) error {
	if !githubapi.RepoPattern.MatchString(repo) || !githubapi.SHAPattern.MatchString(ref) {
		return Error("benchmark fetch requires owner/repo and an immutable 40-character main SHA")
	}
	if entries, err := os.ReadDir(outputDir); err == nil && len(entries) != 0 {
		return Error("snapshot output directory is not empty")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(outputDir, benchPrivateDir); err != nil {
		return err
	}
	fetcher := benchFetcher{ctx: ctx, client: client, repo: repo, ref: ref, dir: outputDir, candidate: candidate, seen: map[string]bool{}}
	return fetcher.fetch()
}

type benchFetcher struct {
	ctx            context.Context
	client         githubapi.API
	repo, ref, dir string
	candidate      *benchSnapshot
	seen           map[string]bool
	selected       int
	unavailable    []string
}

func (f *benchFetcher) fetch() error {
	for page := 1; page <= benchHistoryPages && f.selected < benchHistoryLimit; page++ {
		runs, err := f.runs(page)
		if err != nil {
			return err
		}
		for i := range runs {
			if f.selected == benchHistoryLimit {
				break
			}
			if err = f.consider(&runs[i]); err != nil {
				return err
			}
		}
		if len(runs) < githubapi.PageSize {
			break
		}
		if page == benchHistoryPages && f.selected < benchHistoryLimit {
			f.unavailable = append(f.unavailable, "History search stopped after 1,000 runs")
		}
	}
	return benchWriteJSON(filepath.Join(f.dir, "unavailable.json"), f.unavailable)
}

func (f *benchFetcher) runs(page int) ([]githubapi.Run, error) {
	var response struct {
		Runs []githubapi.Run `json:"workflow_runs"`
	}
	cutoff := f.candidate.Manifest.Started
	if !f.candidate.Cutoff.IsZero() {
		cutoff = f.candidate.Cutoff
	}
	path := fmt.Sprintf("repos/%s/actions/workflows/%s/runs?branch=main&status=success&per_page=100&page=%d&created=%s", f.repo, Workflow, page, url.QueryEscape("<="+cutoff.UTC().Format(time.RFC3339)))
	if err := githubapi.Get(f.ctx, f.client, path, &response); err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func (f *benchFetcher) eligible(run *githubapi.Run) bool {
	cutoff := f.candidate.Manifest.Started
	if !f.candidate.Cutoff.IsZero() {
		cutoff = f.candidate.Cutoff
	}
	return run.Branch == "main" && run.Status == "completed" && run.Conclusion == "success" &&
		(run.Event == "push" || run.Event == "workflow_dispatch") && run.Path == ".github/workflows/"+Workflow &&
		!run.Updated.After(cutoff) && !run.Created.After(cutoff) && !f.seen[run.HeadSHA]
}

//nolint:revive // Linear artifact validation intentionally returns each error immediately.
func (f *benchFetcher) consider(run *githubapi.Run) error {
	if !f.eligible(run) {
		return nil
	}
	ancestor, err := benchAncestor(f.ctx, f.client, f.repo, f.ref, run.HeadSHA)
	if err != nil {
		return err
	}
	if !ancestor {
		return nil
	}
	artifact, err := benchFindArtifact(f.ctx, f.client, f.repo, run)
	if err != nil {
		return err
	}
	if artifact == nil {
		f.unavailable = append(f.unavailable, fmt.Sprintf("Run %d: artifact missing or expired", run.ID))
		return nil
	}
	tmp, err := os.MkdirTemp(f.dir, ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err = benchDownload(f.ctx, f.client, f.repo, *artifact, tmp); err != nil {
		if githubapi.NotFound(err) {
			f.unavailable = append(f.unavailable, fmt.Sprintf("Run %d: artifact expired during download", run.ID))
			return nil
		}
		return err
	}
	snapshot, err := benchLoad(tmp)
	if err != nil {
		return err
	}
	if err = benchValidateRun(&snapshot.Manifest, f.repo, run); err != nil {
		return err
	}
	comparison := benchCompareSnapshots(snapshot, f.candidate, benchDefaultThreshold)
	if len(comparison.Metrics) == 0 {
		f.unavailable = append(f.unavailable, fmt.Sprintf("%s: incompatible environment or benchmark definitions", run.HeadSHA))
		return nil
	}
	snapshot.Manifest.RunCreated = run.Created
	if err = benchWriteJSON(filepath.Join(tmp, "manifest.json"), snapshot.Manifest); err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(f.dir, strconv.FormatInt(run.ID, Decimal))); err != nil {
		return err
	}
	f.seen[run.HeadSHA] = true
	f.selected++
	return nil
}

func benchValidateRun(m *benchManifest, repo string, run *githubapi.Run) error {
	if m.Repository != repo || m.SHA != run.HeadSHA || m.RunID != strconv.FormatInt(run.ID, Decimal) || m.Attempt != strconv.Itoa(run.Attempt) || m.Event != run.Event || m.Started.Before(run.Created) || m.Started.After(run.Updated) {
		return Error("artifact provenance does not match workflow run")
	}
	return nil
}

// ReportRun validates PR run provenance and compares data-only artifacts using
// trusted code, then hands each report to the caller's composite publisher.
func ReportRun(ctx context.Context, client githubapi.API, repo string, id int64, publish func(int, string, *githubapi.Run) error) error {
	var run githubapi.Run
	if err := githubapi.Get(ctx, client, fmt.Sprintf("repos/%s/actions/runs/%d", repo, id), &run); err != nil {
		return err
	}
	if !benchReportable(&run, id) {
		return Error("not a completed PR benchmark run")
	}
	prs, err := benchAssociatedPRs(ctx, client, repo, &run)
	if err != nil {
		return err
	}
	for i := range prs {
		pr := &prs[i]
		if !PRMatches(pr, &run, repo) {
			continue
		}
		body := Marker + "\n## Atmos performance\n\nComparison unavailable: benchmark execution " + Escape(run.Conclusion) + ".\n"
		if run.Conclusion == "success" {
			var err error
			body, err = benchSuccessfulReport(ctx, client, repo, &run, pr)
			if err != nil {
				return err
			}
		}
		body = benchReportLinks(body, repo, &run)

		if err := publish(pr.Number, body, &run); err != nil {
			return err
		}
	}
	return nil
}

func benchReportable(run *githubapi.Run, id int64) bool {
	return run.ID == id && run.Event == "pull_request" && run.Path == ".github/workflows/"+Workflow && run.Status == "completed"
}

func benchAssociatedPRs(ctx context.Context, client githubapi.API, repo string, run *githubapi.Run) ([]githubapi.PR, error) {
	var prs []githubapi.PR
	// The commit association endpoint also works when workflow_run.pull_requests
	// is empty for a fork. Validate every association against the current PR.
	for page := 1; page <= githubapi.PageLimit; page++ {
		var batch []githubapi.PR
		if err := githubapi.Get(ctx, client, fmt.Sprintf("repos/%s/commits/%s/pulls?per_page=100&page=%d", repo, run.HeadSHA, page), &batch); err != nil {
			return nil, err
		}
		prs = append(prs, batch...)
		if len(batch) < githubapi.PageSize {
			return prs, nil
		}
	}
	return nil, Error("benchmark PR association pagination limit exceeded")
}

func benchReportLinks(body, repo string, run *githubapi.Run) string {
	links := fmt.Sprintf("\n[Measurements and workflow](https://github.com/%s/actions/runs/%d)", repo, run.ID)
	if reporterID := os.Getenv("GITHUB_RUN_ID"); reporterID != "" {
		links += fmt.Sprintf(" · [Full comparison artifacts](https://github.com/%s/actions/runs/%s)", repo, reporterID)
	}
	// Keep the sticky comment below GitHub's size limit; the complete report
	// remains in the reporting run artifact.
	body = TruncateReport(body, benchPerformanceLimit)
	body += links + fmt.Sprintf("\n<!-- atmos-benchmark-run:%d:%d -->\n", run.ID, run.Attempt)

	return body
}

func PRMatches(pr *githubapi.PR, run *githubapi.Run, repo string) bool {
	return pr.Number > 0 && pr.State == "open" && pr.Head.SHA == run.HeadSHA && pr.Head.Repo.FullName == run.HeadRepository.FullName && pr.Base.Ref == "main" && pr.Base.Repo.FullName == repo
}

//nolint:revive // Linear provenance and artifact validation keep each failure explicit.
func benchSuccessfulReport(ctx context.Context, client githubapi.API, repo string, run *githubapi.Run, pr *githubapi.PR) (string, error) {
	artifact, err := benchFindArtifact(ctx, client, repo, run)
	if err != nil {
		return "", err
	}
	if artifact == nil {
		return "", Error("successful benchmark run has no result artifact")
	}
	dir, err := os.MkdirTemp("", "atmos-benchmark-report-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	candidateDir := filepath.Join(dir, "candidate")
	if err = os.Mkdir(candidateDir, benchPrivateDir); err != nil {
		return "", err
	}
	if err = benchDownload(ctx, client, repo, *artifact, candidateDir); err != nil {
		return "", err
	}
	candidate, err := benchLoad(candidateDir)
	if err != nil {
		return "", err
	}
	if err = benchValidateRun(&candidate.Manifest, repo, run); err != nil {
		return "", err
	}
	if !githubapi.SHAPattern.MatchString(candidate.Manifest.BaseSHA) {
		return "", Error("missing immutable main base SHA")
	}
	ancestor, err := benchAncestor(ctx, client, repo, pr.Base.SHA, candidate.Manifest.BaseSHA)
	if err != nil {
		return "", err
	}
	if !ancestor {
		return "", Error("artifact base SHA is not on main")
	}
	candidate.Cutoff = run.Created
	history := filepath.Join(dir, "history")
	if err = benchFetch(ctx, client, repo, candidate.Manifest.BaseSHA, history, candidate); err != nil {
		return "", err
	}
	output := filepath.Join("build", "benchmarks", "report", fmt.Sprintf("%d-%d", run.ID, run.Attempt))
	if err = (Bench{}).Compare(history, candidateDir, output); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(output, "report.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
