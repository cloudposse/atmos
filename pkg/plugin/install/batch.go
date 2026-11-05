package install

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"golang.org/x/sync/errgroup"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/reported"
	"github.com/cloudposse/atmos/pkg/ui/batch"
)

const (
	defaultMaxConcurrency = 4
	// Batch failures leave configuration unchanged. Release bytes that
	// were already downloaded and verified can remain in the shared cache.
	batchFailureNotice = "No plugin changes were written."
)

// PrepareBatch prepares packages concurrently using the shared toolchain progress
// display. It returns entries only after every package has passed verification.
// Every package is attempted, so one invocation reports all failing packages, in alias order.
func (s *Service) PrepareBatch(ctx context.Context, requests map[string]*Request) (map[string]pluginlock.Entry, error) {
	defer perf.Track(s.Config, "plugininstall.Service.PrepareBatch")()

	if len(requests) == 0 {
		return map[string]pluginlock.Entry{}, nil
	}
	var entries map[string]pluginlock.Entry
	err := batch.Run(len(requests), func(observe batch.Observer) error {
		var err error
		entries, err = s.prepareBatch(ctx, requests, observe)
		return err
	}, batch.WithToolchainStyle(), batch.WithHeldResults(batchFailureNotice))
	return entries, err
}

func (s *Service) prepareBatch(ctx context.Context, requests map[string]*Request, observe batch.Observer) (map[string]pluginlock.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := s.Config.Plugins.MaxConcurrency
	if limit < 0 {
		return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanationf("`plugins.max_concurrency` is %d.", limit).
			WithHint("Set `plugins.max_concurrency` to a positive number, or omit it to use the default of 4.").Err()
	}
	if limit == 0 {
		limit = defaultMaxConcurrency
	}
	aliases := slices.Sorted(maps.Keys(requests))
	entries := make([]*pluginlock.Entry, len(aliases))
	failures := make([]error, len(aliases))
	// A failed package does not cancel its siblings: each outcome is independent, downloaded
	// releases stay in the cache for the retry, and the caller gets every failure at once.
	var group errgroup.Group
	group.SetLimit(limit)
	for index, alias := range aliases {
		if ctx.Err() != nil {
			break
		}
		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries[index], failures[index] = s.prepareJob(ctx, index, alias, requests[alias], observe)
			return nil
		})
	}
	_ = group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Join in alias order, never completion order, so the output is reproducible.
	if err := errors.Join(failures...); err != nil {
		// The renderer already displayed each failure; the root handler must not repeat them.
		return nil, reported.Wrap(err)
	}
	result := make(map[string]pluginlock.Entry, len(aliases))
	for index, alias := range aliases {
		result[alias] = *entries[index]
	}
	return result, nil
}

func (s *Service) prepareJob(ctx context.Context, id int, alias string, request *Request, observe batch.Observer) (*pluginlock.Entry, error) {
	outcome := "installed"
	entry, err := s.prepare(ctx, request, func(phase string) {
		if phase == "Cached" {
			outcome = "skipped"
		}
		observe(batch.Event{ID: id, Label: alias, Phase: phase})
	})
	version := ""
	if entry != nil {
		version = entry.Version
	}
	label := alias
	if err == nil {
		label, version, outcome = completionLabel(alias, version, outcome, request.Previous, entry)
	} else {
		err = preparationError(err, alias, request.Constraint)
	}
	observe(batch.Event{ID: id, Label: label, Version: version, Done: true, Outcome: outcome, Err: err})
	return entry, err
}

// completionLabel distinguishes checking an existing release from selecting a new one. It returns
// the label, the version shown as a suffix (empty when the label already names it), and the outcome.
func completionLabel(alias, version, outcome string, previous, selected *pluginlock.Entry) (string, string, string) {
	if previous == nil || selected == nil {
		return alias, version, outcome
	}
	if previous.Source == selected.Source && previous.IndexDigest == selected.IndexDigest {
		return fmt.Sprintf("Plugin `%s` is already up to date", alias), version, "installed"
	}
	return fmt.Sprintf("Updated plugin `%s` from `%s` to `%s`", alias, previous.Version, selected.Version), "", "installed"
}

func preparationError(err error, alias, constraint string) error {
	builder := errUtils.Build(err).
		WithExplanationf("Could not prepare plugin `%s` using version constraint `%s`.", alias, constraint)
	if errors.Is(err, errUtils.ErrPluginVersionNotFound) {
		builder.WithHint("Use `atmos plugin search` to find available plugins, or publish a release that satisfies the configured version constraint.")
	}
	if errors.Is(err, errUtils.ErrOCIRegistry) {
		builder.WithHint("Check that the source repository exists and that you can read it; `atmos plugin registry login` stores credentials for a private registry.")
	}
	return builder.Err()
}
