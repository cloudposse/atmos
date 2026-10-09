package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/gofrs/flock"
	"gopkg.in/yaml.v3"

	versionmanager "github.com/cloudposse/atmos/pkg/version/manager"
)

type (
	operation struct {
		Target, Source string
		Data           []byte
		Remove         bool
	}
	desired struct {
		Record *Record
		Tree   string
	}
)

// Run resolves, checks and transactionally reconciles the selected declarations.
//
//nolint:gocritic // Public options are copied deliberately so callers retain an immutable invocation.
func (e *Engine) Run(ctx context.Context, opts Options) ([]Status, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateOptions(&opts); err != nil {
		return nil, err
	}
	unlock, err := e.acquire(opts.Check || opts.DryRun)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err = e.pending(); err != nil {
		return nil, err
	}
	plan, err := e.runPlan(ctx, &opts)
	if err != nil {
		return plan.statuses, err
	}
	if opts.Check {
		return plan.statuses, checkStatuses(plan.statuses)
	}
	if opts.DryRun || len(plan.ops) == 0 {
		return plan.statuses, nil
	}
	return plan.statuses, nil
}

type reconcilePlan struct {
	ops      []operation
	statuses []Status
}

func validateOptions(o *Options) error {
	switch {
	case o.Check && (o.Update || o.Uninstall):
		return fmt.Errorf("%w: check cannot update or uninstall", ErrInvalid)
	case o.Frozen && o.Update:
		return fmt.Errorf("%w: frozen cannot update", ErrInvalid)
	case o.Check && o.DryRun:
		return fmt.Errorf("%w: choose check or dry-run", ErrInvalid)
	case !contains([]string{"", scopeProject, scopeUser}, o.Scope):
		return ErrInvalid
	}
	return nil
}

func checkStatuses(statuses []Status) error {
	for _, status := range statuses {
		if status.Status != "current" {
			return ErrDrift
		}
	}
	return nil
}

func (e *Engine) loadLock(track string) (*Lock, error) {
	lock := &Lock{Version: 1, Tracks: map[string]map[string]Resolution{}}
	if err := readYAML(e.lockPath(), lock); err != nil {
		return nil, err
	}
	if lock.Version != 1 {
		return nil, fmt.Errorf("%w: unsupported skills lock version", ErrInvalid)
	}
	if lock.Tracks == nil {
		lock.Tracks = map[string]map[string]Resolution{}
	}
	if lock.Tracks[track] == nil {
		lock.Tracks[track] = map[string]Resolution{}
	}
	return lock, nil
}

func resolutionTemp(o *Options) (string, func(), error) {
	if o.Check || o.Uninstall {
		return "", func() {}, nil
	}
	temp, err := os.MkdirTemp("", "atmos-skills-resolve-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(temp) }
	canonical, err := filepath.EvalSymlinks(temp)
	return canonical, cleanup, err
}

//nolint:revive // Keep sequential validation and I/O errors next to the operation they guard.
func (e *Engine) runPlan(ctx context.Context, o *Options) (reconcilePlan, error) {
	plan := reconcilePlan{}
	track := versionmanager.EffectiveTrack(e.Config, o.Track)
	lock, err := e.loadLock(track)
	if err != nil {
		return plan, err
	}
	before, err := yaml.Marshal(lock)
	if err != nil {
		return plan, err
	}
	states, err := e.loadStates()
	if err != nil {
		return plan, err
	}
	temp, cleanup, err := resolutionTemp(o)
	defer cleanup()
	if err != nil {
		return plan, err
	}
	if err = e.prepareManualRoots(o, states); err != nil {
		return plan, err
	}
	wanted := map[string]desired{}
	if !o.Uninstall {
		wanted, plan.statuses, err = e.resolve(ctx, o, track, lock, temp)
		if err != nil {
			return plan, err
		}
	}
	e.retainManualUpdates(o, states, wanted)
	ops, statuses, err := e.reconcile(o, states, wanted)
	plan.statuses = append(plan.statuses, statuses...)
	if err != nil {
		return plan, err
	}
	metadata, err := e.metadataOperations(o, lock, before, states)
	if err != nil {
		return plan, err
	}
	// Stages must survive until apply. Copy trees into durable stages before the
	// resolution temporary directory is released by this function.
	ops = append(ops, metadata...)
	plan.ops = ops
	if !o.Check && !o.DryRun && len(plan.ops) > 0 {
		return plan, e.apply(plan.ops)
	}
	plan.ops = nil
	return plan, nil
}

//nolint:revive // Keep sequential validation and I/O errors next to the operation they guard.
func (e *Engine) metadataOperations(o *Options, lock *Lock, before []byte, states map[string]*State) ([]operation, error) {
	ops := []operation{}
	after, err := yaml.Marshal(lock)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(before, after) && !o.Uninstall {
		if o.Frozen || o.Check {
			return nil, fmt.Errorf("%w: resolution lock differs", ErrDrift)
		}
		ops = append(ops, operation{Target: e.lockPath(), Data: after})
	}
	if o.Check || o.DryRun {
		return ops, nil
	}
	for scope, state := range states {
		raw, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return nil, err
		}
		path := filepath.Join(e.stateDir(scope), "installations.json")
		old, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if len(state.Records) == 0 && len(old) == 0 {
			continue
		}
		if string(old) != string(raw) {
			ops = append(ops, operation{Target: path, Data: raw})
		}
	}
	return ops, nil
}

func (e *Engine) acquire(readOnly bool) (func(), error) {
	if readOnly {
		return func() {}, nil
	}
	// Project and user state can both participate in one operation. Lock globally
	// in sorted order so two projects never race on shared user destinations.
	dirs := []string{e.stateDir(scopeProject), e.stateDir(scopeUser)}
	sort.Strings(dirs)
	locks := []*flock.Flock{}
	release := func() {
		for i := len(locks) - 1; i >= 0; i-- {
			_ = locks[i].Unlock()
		}
	}
	for _, dir := range dirs {
		if err := safePath(dir); err != nil {
			release()
			return nil, err
		}
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			release()
			return nil, err
		}
		l := flock.New(filepath.Join(dir, "writer.lock"))
		ok, err := l.TryLock()
		if err != nil || !ok {
			release()
			return nil, fmt.Errorf("%w: another skill writer is active", ErrRecovery)
		}
		locks = append(locks, l)
	}
	return release, nil
}

func (e *Engine) loadStates() (map[string]*State, error) {
	states := map[string]*State{}
	for _, scope := range []string{scopeProject, scopeUser} {
		state, err := e.loadState(scope)
		if err != nil {
			return nil, err
		}
		states[scope] = state
	}
	return states, nil
}

func (e *Engine) loadState(scope string) (*State, error) {
	state := &State{Version: 1}
	path := filepath.Join(e.stateDir(scope), "installations.json")
	if err := safePath(path); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, state); err != nil {
		return nil, err
	}
	if state.Version != 1 {
		return nil, fmt.Errorf("%w: installation state version", ErrInvalid)
	}
	return state, e.validateRecords(scope, state.Records)
}

func (e *Engine) validateRecords(scope string, records []*Record) error {
	seen := map[string]bool{}
	for _, r := range records {
		if r == nil {
			return ErrInvalid
		}
		target, err := e.recordDestination(r)
		if err != nil || r.Scope != scope || target != r.Path || seen[r.Path] {
			return fmt.Errorf("%w: invalid recorded destination %s", ErrInvalid, r.Path)
		}
		seen[r.Path] = true
	}
	return nil
}

func (e *Engine) destination(scope, client, name string) (string, error) {
	if !skillNamePattern.MatchString(name) {
		return "", ErrInvalid
	}
	if client == canonicalClient {
		return filepath.Join(e.stateDir(scope), "content", name), nil
	}
	leaf := map[string]string{"claude-code": ".claude", "vscode": ".github", "gemini": ".gemini"}[client]
	if leaf == "" {
		return "", fmt.Errorf("%w: unknown client %s", ErrInvalid, client)
	}
	root := e.Project
	if scope == scopeUser {
		root = e.Home
		if client == "vscode" {
			leaf = ".copilot"
		}
	}
	return filepath.Join(root, leaf, skillsDir, name), nil
}

func (e *Engine) recordDestination(r *Record) (string, error) {
	if r.Client != "manual" {
		return e.destination(r.Scope, r.Client, r.Name)
	}
	if !skillNamePattern.MatchString(r.Name) || !filepath.IsAbs(r.ManualRoot) || filepath.Dir(r.ManualRoot) == r.ManualRoot {
		return "", ErrInvalid
	}
	return within(r.ManualRoot, r.Name)
}

func (e *Engine) retainManualUpdates(o *Options, states map[string]*State, wanted map[string]desired) {
	if !o.Update || o.Path != "" || o.Clients != nil {
		return
	}
	for _, state := range states {
		for _, r := range state.Records {
			if r.ManualRoot == "" || !e.recordSelected(r, o) {
				continue
			}
			canonical, _ := e.destination(r.Scope, canonicalClient, r.Name)
			w, ok := wanted[canonical]
			if !ok || w.Record.Source != r.Source {
				continue
			}
			updated := *r
			updated.Digest, updated.Track = w.Record.Digest, w.Record.Track
			wanted[r.Path] = desired{Record: &updated, Tree: w.Tree}
		}
	}
}

func (e *Engine) prepareManualRoots(o *Options, states map[string]*State) error {
	var err error
	e.manualRoots = nil
	for _, state := range states {
		for _, record := range state.Records {
			if record.ManualRoot != "" {
				e.manualRoots = append(e.manualRoots, record.ManualRoot)
			}
		}
	}
	if o.Path != "" {
		o.Path, err = filepath.Abs(o.Path)
		if err != nil {
			return err
		}
		if err = safePath(o.Path); err != nil {
			return err
		}
		e.manualRoots = append(e.manualRoots, o.Path)
	}

	return nil
}
