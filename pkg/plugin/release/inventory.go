package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/inventory"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_runner_test.go -package=release github.com/cloudposse/atmos/pkg/git Runner

const gitCommand = "git"

// BackendFactory binds publication settings and the isolated repository to a package.
type BackendFactory func(releaseplan.Entry, string) Backend

type inventorySettings struct {
	runner             pkgit.Runner
	temporaryDirectory string
	check              bool
	failFast           bool
}

// InventoryRelease pairs a successful release with the inventory entry that produced it.
// Release is nil in check mode, where nothing is published.
type InventoryRelease struct {
	Entry   releaseplan.Entry
	Release *pluginoci.Release
}

// InventoryOption configures execution of committed inventory entries.
type InventoryOption func(*inventorySettings)

// WithInventoryRunner injects repository access.
func WithInventoryRunner(runner pkgit.Runner) InventoryOption {
	defer perf.Track(nil, "pluginrelease.WithInventoryRunner")()

	return func(s *inventorySettings) { s.runner = runner }
}

// WithInventoryTemporaryDirectory selects the parent of isolated source trees.
func WithInventoryTemporaryDirectory(directory string) InventoryOption {
	defer perf.Track(nil, "pluginrelease.WithInventoryTemporaryDirectory")()

	return func(s *inventorySettings) { s.temporaryDirectory = directory }
}

// WithInventoryCheck checks packages without publishing them.
func WithInventoryCheck(check bool) InventoryOption {
	defer perf.Track(nil, "pluginrelease.WithInventoryCheck")()

	return func(s *inventorySettings) { s.check = check }
}

// WithInventoryFailFast stops at the first failing package instead of continuing
// with the remaining packages.
func WithInventoryFailFast(failFast bool) InventoryOption {
	defer perf.Track(nil, "pluginrelease.WithInventoryFailFast")()

	return func(s *inventorySettings) { s.failFast = failFast }
}

// RunInventory processes packages in plan order, using detached source snapshots.
// Target compilation remains bounded and parallel within each package. The caller
// retains its checkout, including uncommitted files, throughout the operation.
//
// A failing package does not stop the packages after it: each failure is recorded
// and the remaining packages are still released, so one broken package cannot block
// unrelated releases. The returned slice holds every package whose release completed,
// in plan order, including one whose source worktree could not be removed afterward;
// that cleanup failure is still reported. The error joins every package failure under
// ErrPluginInventoryRelease. Cancellation always stops the run, and
// WithInventoryFailFast restores stopping at the first failure.
func RunInventory(ctx context.Context, entries []releaseplan.Entry, factory BackendFactory, options ...InventoryOption) ([]InventoryRelease, error) {
	defer perf.Track(nil, "pluginrelease.RunInventory")()

	s := inventorySettings{runner: pkgit.NewExecRunner()}
	for _, option := range options {
		option(&s)
	}
	if factory == nil || s.runner == nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	tally := inventoryTally{total: len(entries)}
	results := make([]InventoryRelease, 0, len(entries))
	for index := range entries {
		entry := entries[index]
		if err := ctx.Err(); err != nil {
			tally.errors = append(tally.errors, err)
			return results, tally.err()
		}
		tally.attempted++
		result, runErr, cleanupErr := s.run(ctx, &entry, factory)
		if runErr == nil {
			results = append(results, InventoryRelease{Entry: entry, Release: result})
		}
		if err := errors.Join(runErr, cleanupErr); err != nil {
			tally.failed++
			tally.errors = append(tally.errors, fmt.Errorf("plugin release %s: %w", entry.Name, err))
			if s.failFast || ctx.Err() != nil {
				return results, tally.err()
			}
		}
	}
	return results, tally.err()
}

// inventoryTally counts package outcomes for the summary error.
type inventoryTally struct {
	total     int
	attempted int
	failed    int
	errors    []error
}

// err summarizes package failures; it returns nil when every package succeeded. The count
// covers packages that failed and, after cancellation or fail-fast, packages that never ran.
func (t *inventoryTally) err() error {
	if len(t.errors) == 0 {
		return nil
	}
	summary := fmt.Errorf("%w: %d of %d packages failed or did not run", errUtils.ErrPluginInventoryRelease, t.failed+t.total-t.attempted, t.total)
	return errors.Join(append([]error{summary}, t.errors...)...)
}

// run releases one package from an isolated worktree. The cleanup error is reported
// separately so a release that completed is still recorded when its worktree could not
// be removed.
func (s *inventorySettings) run(ctx context.Context, entry *releaseplan.Entry, factory BackendFactory) (result *pluginoci.Release, runErr, cleanupErr error) {
	if !inventory.SafePath(entry.Path) || entry.Source == "" {
		return nil, errUtils.ErrPluginInvalidConfig, nil
	}
	source, err := s.runner.Run(ctx, gitCommand, []string{"rev-parse", "--verify", "--end-of-options", entry.Source + "^{commit}"}, &pkgit.RunOptions{})
	if err != nil {
		return nil, err, nil
	}
	revision := strings.TrimSpace(source.Stdout)
	if revision == "" || strings.HasPrefix(revision, "-") {
		return nil, errUtils.ErrPluginInvalidConfig, nil
	}
	temporary, err := os.MkdirTemp(s.temporaryDirectory, "atmos-plugin-source-*")
	if err != nil {
		return nil, err, nil
	}
	defer os.RemoveAll(temporary)
	directory := filepath.Join(temporary, "source")
	// Cleanup must survive cancellation of a compiler or registry operation.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if _, err := s.runner.Run(cleanup, gitCommand, []string{"worktree", "remove", "--force", directory}, &pkgit.RunOptions{}); err != nil {
			cleanupErr = fmt.Errorf("remove plugin source worktree: %w", err)
		}
	}()
	if _, err := s.runner.Run(ctx, gitCommand, []string{"worktree", "add", "--detach", directory, revision}, &pkgit.RunOptions{}); err != nil {
		return nil, err, nil
	}
	result, runErr = Run(ctx, filepath.Join(directory, filepath.FromSlash(entry.Path)), factory(*entry, directory), WithCheck(s.check), WithTemporaryDirectory(temporary))
	return result, runErr, nil
}

// CurrentInventoryHead fetches the remote default branch at reconciliation time.
// Reading FETCH_HEAD pins the accepted snapshot independently of a queued run's
// older checkout. Callers serialize publication before invoking this function.
func CurrentInventoryHead(ctx context.Context, runner pkgit.Runner) (string, error) {
	defer perf.Track(nil, "pluginrelease.CurrentInventoryHead")()

	if runner == nil {
		return "", errUtils.ErrPluginInvalidConfig
	}
	if _, err := runner.Run(ctx, gitCommand, []string{"fetch", "--no-tags", "origin", "HEAD"}, &pkgit.RunOptions{}); err != nil {
		return "", err
	}
	result, err := runner.Run(ctx, gitCommand, []string{"rev-parse", "--verify", "FETCH_HEAD^{commit}"}, &pkgit.RunOptions{})
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(result.Stdout)
	if head == "" {
		return "", errUtils.ErrPluginInvalidConfig
	}
	return head, nil
}
