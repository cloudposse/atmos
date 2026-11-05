package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

// InstalledSources resolves installed names to source owners without writing.
func (e *Engine) InstalledSources(name, scope string) ([]string, error) {
	states, err := e.loadStates()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	labels := []string{}
	for _, state := range states {
		for _, r := range state.Records {
			if r.Project == e.Project && (name == "" || r.Name == name) && (scope == "" || scope == r.Scope) && !seen[r.Source] {
				seen[r.Source] = true
				labels = append(labels, r.Source)
			}
		}
	}
	sort.Strings(labels)
	return labels, nil
}

// Status lists declarations and owned destinations without network or writes.
//
//nolint:gocritic // Check mode modifies a private copy of the caller's options.
func (e *Engine) Status(ctx context.Context, opts Options) ([]Status, error) {
	if err := e.pending(); err != nil {
		return nil, err
	}
	opts.Check = true
	statuses := []Status{}
	seen := map[string]bool{}
	for _, label := range e.selectedLabels(opts.Source) {
		selected := opts
		selected.Source = label
		result, err := e.Run(ctx, selected)
		if err != nil && !errors.Is(err, ErrDrift) {
			return statuses, err
		}
		for _, status := range result {
			seen[status.Path] = true
			statuses = append(statuses, status)
		}
	}
	states, err := e.loadStates()
	if err != nil {
		return statuses, err
	}
	remaining, err := e.recordStatuses(states, seen)
	if err != nil {
		return statuses, err
	}
	statuses = append(statuses, remaining...)
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Path < statuses[j].Path })
	return statuses, nil
}

func (e *Engine) recordStatuses(states map[string]*State, seen map[string]bool) ([]Status, error) {
	statuses := []Status{}
	for _, state := range states {
		for _, r := range state.Records {
			if seen[r.Path] {
				continue
			}
			status, err := e.recordStatus(r)
			if err != nil {
				return nil, err
			}
			statuses = append(statuses, report(r, status))
		}
	}
	return statuses, nil
}

func (e *Engine) recordStatus(r *Record) (string, error) {
	digest, err := treeDigest(r.Path)
	switch {
	case os.IsNotExist(err):
		return "missing", nil
	case err != nil:
		return "", err
	case digest != r.Digest:
		return "drifted", nil
	}
	if r.Project != e.Project || e.Config.AI.Skills[r.Source] != nil || isAdHoc(r.Source) {
		return "current", nil
	}
	return "obsolete", nil
}

func (e *Engine) ResolveInstalled(name, scope string) (string, error) {
	sources, err := e.InstalledSources(name, scope)
	if err != nil {
		return "", err
	}
	if len(sources) > 1 {
		return "", fmt.Errorf("%w: skill %s has multiple source owners; use --source", ErrInvalid, name)
	}
	if len(sources) == 1 {
		return sources[0], nil
	}
	return "", nil
}

func isAdHoc(label string) bool { return strings.HasPrefix(label, "ad-hoc:") }

// InstalledConfig selects declarations that have existing installations.
func (e *Engine) InstalledConfig(scope string) (*schema.AtmosConfiguration, error) {
	labels, err := e.InstalledSources("", scope)
	if err != nil {
		return nil, err
	}
	config := *e.Config
	config.AI.Skills = map[string]*schema.AISkillConfig{}
	for _, label := range labels {
		if d := e.Config.AI.Skills[label]; d != nil {
			config.AI.Skills[label] = d
		}
	}
	return &config, nil
}
