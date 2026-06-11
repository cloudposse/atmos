package git

import (
	"context"
	"sync"

	"github.com/charmbracelet/lipgloss"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Status value: workdir or .git directory is absent.
	statusMissing = "missing"
	// Status value: workdir exists and the working tree is clean.
	statusCloned = "cloned"
	// Status value: workdir exists but has uncommitted changes.
	statusDirty = "dirty"

	// Branch display value used when no branch is explicitly configured.
	defaultBranchPlaceholder = "(default)"

	// Worker pool size for concurrent status probes.
	statusWorkerPoolSize = 4

	statusDot = "●"
)

var isGitListTTYCached = sync.OnceValue(func() bool {
	term := terminal.New()
	return term.IsTTY(terminal.Stdout)
})

// StatusProber is a seam for status probing used in tests.
// Production code uses the git provider; tests substitute a stub.
type StatusProber interface {
	ProbeStatus(ctx context.Context, workdir string) string
}

// providerStatusProber probes status via the registered git provider.
type providerStatusProber struct{}

// ProbeStatus runs `git status --porcelain` in workdir and returns
// "missing", "cloned", or "dirty".
func (p *providerStatusProber) ProbeStatus(ctx context.Context, workdir string) string {
	defer perf.Track(nil, "git.providerStatusProber.ProbeStatus")()

	exec, err := providerForName("")
	if err != nil {
		return statusMissing
	}

	result, err := exec.Status(ctx, &atmosgit.StatusOptions{
		RepoContext: atmosgit.RepoContext{
			Workdir: workdir,
		},
	})
	if err != nil {
		return statusMissing
	}

	if result.Clean {
		return statusCloned
	}
	return statusDirty
}

// defaultProber is the production status prober.
var defaultProber StatusProber = &providerStatusProber{}

// extractGitRepoRows returns one row map per configured repository.
// When checkStatus is false, the "status" key is omitted from every row and
// no filesystem or git subprocess work is performed.
// When checkStatus is true, status probes run concurrently with a bounded
// worker pool (statusWorkerPoolSize) and the resolved value is materialised
// into each row before rendering (column templates remain pure).
func extractGitRepoRows(cfg *schema.GitConfig, checkStatus bool) ([]map[string]any, error) {
	return extractGitRepoRowsWithProber(context.Background(), cfg, checkStatus, defaultProber)
}

// extractGitRepoRowsWithProber is the testable variant that accepts an
// injected StatusProber so tests can verify gating without real filesystem I/O.
func extractGitRepoRowsWithProber(
	ctx context.Context,
	cfg *schema.GitConfig,
	checkStatus bool,
	prober StatusProber,
) ([]map[string]any, error) {
	defer perf.Track(nil, "git.extractGitRepoRowsWithProber")()

	if cfg == nil || len(cfg.Repositories) == 0 {
		return nil, nil
	}

	names := atmosgit.ConfiguredRepositoryNames(cfg)

	// Build base rows (no status probes yet).
	rows := make([]map[string]any, len(names))
	for i, name := range names {
		rows[i] = buildBaseRow(cfg, name)
	}

	if !checkStatus {
		return rows, nil
	}

	// Probe status concurrently with a bounded worker pool.
	probeStatusConcurrently(ctx, rows, prober)

	return rows, nil
}

// buildBaseRow builds a repository row without status.
func buildBaseRow(cfg *schema.GitConfig, name string) map[string]any {
	defer perf.Track(nil, "git.buildBaseRow")()

	repo := cfg.Repositories[name]

	// Resolve provider default.
	provider := repo.Provider
	if provider == "" {
		provider = atmosgit.DefaultProviderName
	}

	// Resolve branch placeholder.
	branch := repo.Branch
	if branch == "" {
		branch = defaultBranchPlaceholder
	}

	// Resolve workdir (XDG automatic when not explicit).
	workdir := repo.Workdir
	if workdir == "" {
		resolved, err := atmosgit.DefaultWorkdir(name)
		if err != nil {
			workdir = ""
		} else {
			workdir = resolved
		}
	}

	return map[string]any{
		"name":     name,
		"uri":      repo.URI,
		"provider": provider,
		"branch":   branch,
		"workdir":  workdir,
	}
}

// probeStatusConcurrently materialises the "status" field in each row using a
// bounded worker pool. Rows without a "workdir" value get status "missing".
func probeStatusConcurrently(ctx context.Context, rows []map[string]any, prober StatusProber) {
	defer perf.Track(nil, "git.probeStatusConcurrently")()

	type workItem struct {
		index   int
		workdir string
	}

	jobs := make(chan workItem, len(rows))
	for i, row := range rows {
		workdir, _ := row["workdir"].(string)
		jobs <- workItem{index: i, workdir: workdir}
	}
	close(jobs)

	var wg sync.WaitGroup
	// results is indexed by row position; no mutex needed (each goroutine
	// writes to a distinct index).
	results := make([]string, len(rows))

	workers := statusWorkerPoolSize
	if workers > len(rows) {
		workers = len(rows)
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if job.workdir == "" {
					results[job.index] = statusMissing
					continue
				}
				results[job.index] = prober.ProbeStatus(ctx, job.workdir)
			}
		}()
	}

	wg.Wait()

	for i, status := range results {
		rows[i]["status"] = gitStatusIndicator(status)
		rows[i]["status_text"] = status
	}
}

func gitStatusIndicator(status string) string {
	return gitStatusIndicatorWithTTY(status, isGitListTTYCached())
}

func gitStatusIndicatorWithTTY(status string, isTTY bool) string {
	if !isTTY {
		return status
	}

	switch status {
	case statusCloned:
		return theme.GetSuccessStyle().Render(statusDot)
	case statusDirty:
		return theme.GetWarningStyle().Render(statusDot)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray)).Render(statusDot)
	}
}
