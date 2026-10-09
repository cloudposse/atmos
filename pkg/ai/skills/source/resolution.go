package source

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	versionmanager "github.com/cloudposse/atmos/pkg/version/manager"
)

type resolutionRequest struct{ Track, Label, Temp string }

func (e *Engine) resolve(ctx context.Context, o *Options, track string, lock *Lock, temp string) (map[string]desired, []Status, error) {
	wanted := map[string]desired{}
	statuses := []Status{}
	labels := e.selectedLabels(o.Source)
	if o.Source != "" && len(labels) == 0 && !o.Prune {
		return wanted, statuses, fmt.Errorf("%w: unknown source %s", ErrInvalid, o.Source)
	}
	for i, label := range labels {
		resolution, roots, err := e.resolveOne(ctx, o, resolutionRequest{track, label, filepath.Join(temp, fmt.Sprint(i))}, lock)
		if err != nil {
			return wanted, append(statuses, Status{Source: label, Track: track, Status: "stale"}), err
		}
		if err = e.desiredSkills(o, resolutionRequest{Track: track, Label: label}, &resolution, roots, wanted); err != nil {
			return wanted, statuses, err
		}
		lock.Tracks[track][label] = resolution
	}
	return wanted, statuses, nil
}

func (e *Engine) selectedLabels(selected string) []string {
	labels := []string{}
	for label, d := range e.Config.AI.Skills {
		if d != nil && d.Source != "" && (selected == "" || selected == label) {
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	return labels
}

//nolint:revive // Keep literal, managed, frozen and update resolution decisions in one ordered flow.
func (e *Engine) resolveOne(ctx context.Context, o *Options, request resolutionRequest, lock *Lock) (Resolution, []string, error) {
	track, label, temp := request.Track, request.Label, request.Temp
	d := normalize(e.Config.AI.Skills[label])
	if err := validateDeclaration(&d); err != nil {
		return Resolution{}, nil, fmt.Errorf("%s: %w", label, err)
	}
	ref := d.Ref.Literal
	if d.Ref.Dependency != "" {
		var err error
		ref, err = versionmanager.ResolveLocked(e.Config, track, d.Ref.Dependency)
		if err != nil {
			return Resolution{}, nil, err
		}
	}
	previous, exists := lock.Tracks[track][label]
	matching := exists && reflect.DeepEqual(previous.Declaration, d) && previous.Version == ref
	if !matching && (o.Frozen || o.Check) {
		return previous, nil, fmt.Errorf("%w: missing or stale lock for %s", ErrDrift, label)
	}
	if o.Check {
		return previous, nil, e.checkLocalSources(&previous)
	}
	refresh := e.shouldRefresh(o, label, ref)
	if !matching || refresh {
		resolved, roots, err := e.discover(ctx, &d, ref, temp)
		if err == nil && !o.Update {
			err = unchangedLocalSnapshots(&previous, &resolved)
		}
		return resolved, roots, err
	}
	roots, err := e.replay(ctx, &previous, temp)
	return previous, roots, err
}

func (e *Engine) checkLocalSources(resolution *Resolution) error {
	for _, repo := range resolution.Repositories {
		local, ok := localPath(repo.Source, e.Project)
		if !ok {
			continue
		}
		if filepath.IsAbs(repo.Source) {
			local = repo.Source
		}
		digest, err := treeDigest(local)
		if err != nil {
			return err
		}
		if digest != repo.Digest {
			return ErrDrift
		}
	}
	return nil
}

//nolint:revive // Scope, name and client selection are evaluated before producing destinations.
func (e *Engine) desiredSkills(o *Options, request resolutionRequest, r *Resolution, roots []string, wanted map[string]desired) error {
	track, label := request.Track, request.Label
	scope := r.Declaration.Scope
	if o.Scope != "" {
		scope = o.Scope
	}
	clients := r.Declaration.Clients
	if o.Clients != nil {
		clients = o.Clients
	} else if len(clients) == 0 {
		clients = marketplace.DetectClients(e.Project, e.Home, scope)
	}
	if o.Path != "" {
		clients = []string{"manual"}
	}
	matched := false
	for _, skill := range r.Skills {
		if o.Name != "" && o.Name != skill.Name {
			continue
		}
		matched = true
		tree, err := lockedTree(&skill, roots, o.Check)
		if err != nil {
			return err
		}
		for _, client := range append([]string{canonicalClient}, clients...) {
			record := &Record{Project: e.Project, Source: label, Track: track, Scope: scope, Name: skill.Name, Client: client, Digest: skill.Digest}
			if client == "manual" {
				record.ManualRoot = o.Path
			}
			if err := e.addDesired(record, tree, wanted); err != nil {
				return err
			}
		}
	}
	if o.Name != "" && !matched {
		return fmt.Errorf("%w: skill %s is not selected by source %s", ErrInvalid, o.Name, label)
	}
	return nil
}

func lockedTree(skill *LockedSkill, roots []string, check bool) (string, error) {
	if check {
		return "", nil
	}
	if skill.Repository < 0 || skill.Repository >= len(roots) {
		return "", ErrInvalid
	}
	return within(roots[skill.Repository], skill.Path)
}

func immutableRef(ref string) bool {
	if len(ref) != gitSHA1Length && len(ref) != gitSHA256Length {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}

//nolint:revive // Each locked repository and skill has a separate integrity check.
func (e *Engine) replay(ctx context.Context, r *Resolution, temp string) ([]string, error) {
	roots := []string{}
	for i, repo := range r.Repositories {
		root := filepath.Join(temp, fmt.Sprint(i))
		resolved, err := e.Fetcher.Fetch(ctx, repo, root)
		if err != nil {
			return nil, err
		}
		if repo.Commit != resolved.Commit || repo.Digest != resolved.Digest {
			return nil, fmt.Errorf("%w: local source changed; run skill update", ErrDrift)
		}
		roots = append(roots, root)
	}
	for _, skill := range r.Skills {
		if skill.Repository < 0 || skill.Repository >= len(roots) {
			return nil, ErrInvalid
		}
		p, err := within(roots[skill.Repository], skill.Path)
		if err != nil {
			return nil, err
		}
		digest, err := treeDigest(p)
		if err != nil {
			return nil, err
		}
		if digest != skill.Digest {
			return nil, fmt.Errorf("%w: skill content differs from lock", ErrDrift)
		}
	}
	return roots, nil
}

func (e *Engine) addDesired(record *Record, tree string, wanted map[string]desired) error {
	target, err := e.recordDestination(record)
	if err != nil {
		return err
	}
	if existing, ok := wanted[target]; ok && existing.Record.Source != record.Source {
		return fmt.Errorf("%w: colliding skill %s", ErrInvalid, record.Name)
	}
	record.Path = target
	wanted[target] = desired{Record: record, Tree: tree}
	return nil
}

func unchangedLocalSnapshots(previous, resolved *Resolution) error {
	for _, old := range previous.Repositories {
		if old.Commit != "" || old.Digest == "" {
			continue
		}
		for _, next := range resolved.Repositories {
			if next.Source == old.Source && next.Commit == "" && next.Digest != old.Digest {
				return fmt.Errorf("%w: local source changed; run skill update", ErrDrift)
			}
		}
	}
	return nil
}

func (e *Engine) shouldRefresh(o *Options, label, ref string) bool {
	if !o.Update {
		return false
	}
	d := e.Config.AI.Skills[label]
	if _, local := localPath(d.Source, e.Project); local {
		return true
	}
	return d.Ref.Dependency == "" && !immutableRef(ref)
}
