package taskgraph

import (
	"encoding/json"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// KindCommand identifies a Ref that targets a named custom command.
	KindCommand = "command"
	// KindWorkflow identifies a Ref that targets a named workflow.
	KindWorkflow = "workflow"
)

// Fail modes, mirroring pkg/workflow/control.go's ControlFailWaitAll/ControlFailFast/
// ControlFailBestEffort exactly (same vocabulary, not re-exported to avoid pulling
// pkg/taskgraph's lean, execution-callback-only dependency surface into pkg/workflow's much
// larger one).
const (
	FailWaitAll    = "wait_all"
	FailFast       = "fail_fast"
	FailBestEffort = "best_effort"
)

// Ref identifies one dependency to resolve and run: a named command or workflow,
// optionally parameterized (Flags/Args) and, for workflows, an explicit File for
// cross-file resolution.
type Ref struct {
	Kind  string
	Name  string
	File  string
	Flags map[string]string
	Args  []string
	Fail  string
}

// NodeID returns the deterministic graph-node identity for a Ref. Two Refs with the same
// Kind/Name/File/Flags/Args always produce the same NodeID, so the graph builder collapses
// them into a single executed node (automatic dedup); any difference in parameters produces
// a distinct NodeID, so differently-parameterized invocations of the same command/workflow
// both execute.
func (r *Ref) NodeID() string {
	defer perf.Track(nil, "taskgraph.Ref.NodeID")()

	// Canonicalize Flags by sorted key order so map iteration order never affects the ID.
	keys := make([]string, 0, len(r.Flags))
	for k := range r.Flags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	flags := make([][2]string, 0, len(keys))
	for _, k := range keys {
		flags = append(flags, [2]string{k, r.Flags[k]})
	}

	// Encoding to JSON (rather than hashing) keeps node IDs human-readable in error
	// messages and test failures, at the cost of longer IDs -- acceptable since dependency
	// graphs are small.
	payload, _ := json.Marshal(struct {
		Kind  string      `json:"kind"`
		Name  string      `json:"name"`
		File  string      `json:"file,omitempty"`
		Flags [][2]string `json:"flags,omitempty"`
		Args  []string    `json:"args,omitempty"`
	}{Kind: r.Kind, Name: r.Name, File: r.File, Flags: flags, Args: r.Args})
	return string(payload)
}
