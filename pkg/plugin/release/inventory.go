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

// RunInventory processes packages in plan order, using detached source snapshots.
// Target compilation remains bounded and parallel within each package. The caller
// retains its checkout, including uncommitted files, throughout the operation.
func RunInventory(ctx context.Context, entries []releaseplan.Entry, factory BackendFactory, options ...InventoryOption) ([]*pluginoci.Release, error) {
	defer perf.Track(nil, "pluginrelease.RunInventory")()

	s := inventorySettings{runner: pkgit.NewExecRunner()}
	for _, option := range options {
		option(&s)
	}
	if factory == nil || s.runner == nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	results := make([]*pluginoci.Release, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := s.run(ctx, &entry, factory)
		if err != nil {
			return nil, fmt.Errorf("plugin release %s: %w", entry.Name, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *inventorySettings) run(ctx context.Context, entry *releaseplan.Entry, factory BackendFactory) (result *pluginoci.Release, resultErr error) {
	if !inventory.SafePath(entry.Path) || entry.Source == "" {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	source, err := s.runner.Run(ctx, gitCommand, []string{"rev-parse", "--verify", "--end-of-options", entry.Source + "^{commit}"}, pkgit.RunOptions{})
	if err != nil {
		return nil, err
	}
	revision := strings.TrimSpace(source.Stdout)
	if revision == "" || strings.HasPrefix(revision, "-") {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	temporary, err := os.MkdirTemp(s.temporaryDirectory, "atmos-plugin-source-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	directory := filepath.Join(temporary, "source")
	// Cleanup must survive cancellation of a compiler or registry operation.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, cleanupErr := s.runner.Run(cleanup, gitCommand, []string{"worktree", "remove", "--force", directory}, pkgit.RunOptions{})
		if cleanupErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove plugin source worktree: %w", cleanupErr))
		}
	}()
	if _, err := s.runner.Run(ctx, gitCommand, []string{"worktree", "add", "--detach", directory, revision}, pkgit.RunOptions{}); err != nil {
		return nil, err
	}
	return Run(ctx, filepath.Join(directory, filepath.FromSlash(entry.Path)), factory(*entry, directory), WithCheck(s.check), WithTemporaryDirectory(temporary))
}

// CurrentInventoryHead fetches the remote default branch at reconciliation time.
// Reading FETCH_HEAD pins the accepted snapshot independently of a queued run's
// older checkout. Callers serialize publication before invoking this function.
func CurrentInventoryHead(ctx context.Context, runner pkgit.Runner) (string, error) {
	defer perf.Track(nil, "pluginrelease.CurrentInventoryHead")()

	if runner == nil {
		return "", errUtils.ErrPluginInvalidConfig
	}
	if _, err := runner.Run(ctx, gitCommand, []string{"fetch", "--no-tags", "origin", "HEAD"}, pkgit.RunOptions{}); err != nil {
		return "", err
	}
	result, err := runner.Run(ctx, gitCommand, []string{"rev-parse", "--verify", "FETCH_HEAD^{commit}"}, pkgit.RunOptions{})
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(result.Stdout)
	if head == "" {
		return "", errUtils.ErrPluginInvalidConfig
	}
	return head, nil
}
