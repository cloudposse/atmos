package source

import (
	"fmt"
	"os"
	"sort"
)

func report(r *Record, status string) Status {
	return Status{Source: r.Source, Name: r.Name, Track: r.Track, Scope: r.Scope, Client: r.Client, Path: r.Path, Status: status}
}

func (e *Engine) reconcile(o *Options, states map[string]*State, wanted map[string]desired) ([]operation, []Status, error) {
	records := map[string]*Record{}
	for _, s := range states {
		for _, r := range s.Records {
			records[r.Path] = r
		}
	}
	ops, statuses, err := e.planWanted(o, wanted, records)
	if err != nil {
		return nil, statuses, err
	}
	removals, obsolete, err := e.planObsolete(o, wanted, records)
	statuses = append(statuses, obsolete...)
	if err != nil {
		return nil, statuses, err
	}
	for _, state := range states {
		state.Records = nil
	}
	for _, path := range sortedRecordPaths(records) {
		r := records[path]
		states[r.Scope].Records = append(states[r.Scope].Records, r)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Path < statuses[j].Path })
	return append(ops, removals...), statuses, nil
}

func sortedRecordPaths(records map[string]*Record) []string {
	paths := []string{}
	for path := range records {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (e *Engine) planWanted(o *Options, wanted map[string]desired, records map[string]*Record) ([]operation, []Status, error) {
	ops := []operation{}
	statuses := []Status{}
	paths := []string{}
	for path := range wanted {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		w := wanted[path]
		status, err := e.inspectDesired(records[path], w.Record)
		if err != nil {
			return nil, statuses, err
		}
		statuses = append(statuses, report(w.Record, status))
		if status == "drifted" && !o.Force && !o.Check {
			return nil, statuses, fmt.Errorf("%w: %s; use --force to replace modified owned files", ErrDrift, path)
		}
		if status != "current" {
			ops = append(ops, operation{Target: path, Source: w.Tree})
		}
		records[path] = w.Record
	}
	return ops, statuses, nil
}

//nolint:revive // Ownership, missing files, drift and stale content are distinct reconciliation outcomes.
func (e *Engine) inspectDesired(old, wanted *Record) (string, error) {
	if old != nil && (old.Project != e.Project || old.Source != wanted.Source) {
		return "", fmt.Errorf("%w: %s", ErrOwnership, wanted.Path)
	}
	digest, err := treeDigest(wanted.Path)
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if old == nil {
		return "", fmt.Errorf("%w: %s", ErrOwnership, wanted.Path)
	}
	switch {
	case digest != old.Digest:
		return "drifted", nil
	case digest != wanted.Digest || old.Track != wanted.Track:
		return "stale", nil
	default:
		return "current", nil
	}
}

//nolint:revive // Keep pruning guards adjacent to the only branch that schedules removals.
func (e *Engine) planObsolete(o *Options, wanted map[string]desired, records map[string]*Record) ([]operation, []Status, error) {
	ops := []operation{}
	statuses := []Status{}
	for _, path := range sortedRecordPaths(records) {
		r := records[path]
		if _, ok := wanted[path]; ok || !e.recordSelected(r, o) {
			continue
		}
		if r.Client == canonicalClient && o.Clients != nil && hasRetainedClient(records, r, o.Clients) {
			continue
		}
		status := "obsolete"
		if o.Prune || o.Uninstall {
			exists, err := removable(r, o.Force)
			if err != nil {
				return nil, statuses, err
			}
			if exists {
				ops = append(ops, operation{Target: path, Remove: true})
			}
			delete(records, path)
			status = "remove"
		}
		statuses = append(statuses, report(r, status))
	}
	return ops, statuses, nil
}

func removable(r *Record, force bool) (bool, error) {
	digest, err := treeDigest(r.Path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if digest != r.Digest && !force {
		return false, fmt.Errorf("%w: modified obsolete copy %s", ErrDrift, r.Path)
	}
	return true, nil
}

func (e *Engine) recordSelected(r *Record, o *Options) bool {
	if isAdHoc(r.Source) != e.AdHoc || r.Project != e.Project {
		return false
	}
	if !filterMatches(o.Source, r.Source) || !filterMatches(o.Name, r.Name) || !filterMatches(o.Scope, r.Scope) {
		return false
	}
	return o.Clients == nil || r.Client == canonicalClient || contains(o.Clients, r.Client)
}
func filterMatches(filter, value string) bool { return filter == "" || filter == value }

func hasRetainedClient(records map[string]*Record, r *Record, clients []string) bool {
	for _, other := range records {
		if other.Name == r.Name && other.Source == r.Source && other.Project == r.Project && other.Scope == r.Scope && other.Client != canonicalClient && !contains(clients, other.Client) {
			return true
		}
	}
	return false
}
