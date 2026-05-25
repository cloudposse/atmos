package hooks

import (
	"sort"
	"sync"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Engine runs a hook of a given kind. Different kinds may implement
// different engines: StoreEngine reads terraform outputs into a store;
// CommandEngine (added in a later commit) executes a binary and captures
// its structured output.
type Engine interface {
	// Run executes the hook with the given context.
	// Implementations that produce no structured output return (nil, err).
	Run(ctx *ExecContext) (*Output, error)
}

// ExecContext is everything an engine needs at run time. Built per
// hook invocation by RunAll after kind defaults have been applied. Some
// fields (OutputFile, OutputDir, ExitCode, CommandError) are populated by
// the engine before calling a kind's ResultHandler.
type ExecContext struct {
	// Hook is the resolved hook (kind defaults applied; user overrides preserved).
	Hook *Hook
	// Kind is the registered kind handling this hook.
	Kind *Kind
	// Event is the lifecycle event triggering the hook.
	Event HookEvent
	// AtmosConfig is the global Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
	// Info carries component and stack information.
	Info *schema.ConfigAndStacksInfo
	// Cmd is the cobra command currently executing.
	Cmd *cobra.Command
	// Args are the command-line args passed to Cmd.
	Args []string

	// ToolchainPATH is the PATH fragment containing directories for any
	// dependencies.tools that were auto-installed for this component's
	// hooks. Populated by Hooks.preflight; consumed by CommandEngine
	// so the installed pinned versions take precedence over the operator's
	// PATH. Empty when the component declares no hook dependencies.
	ToolchainPATH string

	// OutputFile is the temp file path the tool wrote structured output to.
	// Populated by CommandEngine before calling ResultHandler.
	OutputFile string
	// OutputDir is the temp directory containing OutputFile. Exposed for
	// directory-output tools (e.g., KICS writes results.sarif into a dir).
	OutputDir string
	// ExitCode is the subprocess exit code (0 = success).
	ExitCode int
	// CommandError is the subprocess error, if any.
	CommandError error
}

// Output is what one hook invocation produces. Engines that don't emit
// structured output (e.g. store) return nil.
type Output struct {
	// Artifact is the opaque blob produced by the hook (tool output file,
	// rendered report, etc.). May be nil.
	Artifact *Artifact
	// Summary is the typed envelope used for run pages, PR comments, and
	// terminal rendering. May be nil.
	Summary *Summary
}

// Artifact is the opaque blob produced by a hook. Subsequent commits
// extend this with backend-routing metadata.
type Artifact struct {
	// Name is the filename inside the upload bundle (e.g. "breakdown.json").
	Name string
	// Body is the artifact content.
	Body []byte
	// Format is the optional generic-renderer hint ("markdown" or "").
	// For named kinds, Pro derives rendering from the kind name, so this
	// remains empty.
	Format string
	// Metadata is arbitrary tags attached to the artifact.
	Metadata map[string]string
}

// SummaryStatus is the run status reported by a hook for the Pro/PR/terminal
// summary card.
type SummaryStatus string

// Summary statuses understood by Pro and the terminal renderer.
const (
	StatusSuccess SummaryStatus = "success"
	StatusWarning SummaryStatus = "warning"
	StatusFailure SummaryStatus = "failure"
)

// Summary is the typed envelope every hook kind fills the same way.
type Summary struct {
	// Kind is the registered kind name (Pro selects its renderer from this).
	Kind string
	// Status is success | warning | failure.
	Status SummaryStatus
	// Title is the short headline (e.g. "+$47.20/mo" or "2 HIGH, 5 MED").
	Title string
	// Counts is an optional grouped breakdown (e.g. {"high": 2, "medium": 5}).
	Counts map[string]int
	// Body is the single markdown rendering used in every surface that
	// renders markdown: terminal, Pro run page, PR comments, step summaries.
	Body string
}

// ResultHandler parses the kind's structured output and produces a Summary.
// Returning (nil, nil) is valid for kinds with no structured summary.
type ResultHandler func(ctx *ExecContext) (*Summary, error)

// Kind is a registered hook type. Built-ins self-register from
// pkg/hooks/kinds/*/kind.go via init().
type Kind struct {
	// Name is the kind discriminator (e.g. "store", "command", "infracost").
	Name string

	// Defaults for the generic command engine. Named kinds set these;
	// user Hook fields override.
	Command     string
	DefaultArgs []string
	DefaultEnv  map[string]string

	// OnFailure is the default failure mode if the hook doesn't override.
	OnFailure string

	// Engine runs hooks of this kind.
	Engine Engine

	// ResultHandler parses structured output into a Summary. Optional.
	ResultHandler ResultHandler
}

// ResolveDefaults returns a copy of hook with kind defaults filled in for
// any fields the hook didn't set explicitly. The original hook is not
// modified.
func (k *Kind) ResolveDefaults(hook *Hook) *Hook {
	defer perf.Track(nil, "hooks.Kind.ResolveDefaults")()

	resolved := *hook // shallow copy
	if resolved.Command == "" {
		resolved.Command = k.Command
	}
	if len(resolved.Args) == 0 && len(k.DefaultArgs) > 0 {
		resolved.Args = append([]string(nil), k.DefaultArgs...)
	}
	if len(resolved.Env) == 0 && len(k.DefaultEnv) > 0 {
		resolved.Env = make(map[string]string, len(k.DefaultEnv))
		for kk, vv := range k.DefaultEnv {
			resolved.Env[kk] = vv
		}
	}
	if resolved.OnFailure == "" {
		resolved.OnFailure = k.OnFailure
	}
	return &resolved
}

var (
	kindsMu sync.RWMutex
	kinds   = make(map[string]*Kind)
)

// RegisterKind registers a hook kind. Kinds self-register from
// pkg/hooks/kinds/*/kind.go via init() — this is the only registration path
// (there is no YAML-defined kind registry; reuse uses stack imports).
func RegisterKind(k *Kind) error {
	defer perf.Track(nil, "hooks.RegisterKind")()

	if k == nil {
		return errUtils.ErrNilParam
	}
	if k.Name == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Kind has empty name").
			Err()
	}
	if k.Engine == nil {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Kind has no Engine").
			WithContext("kind", k.Name).
			Err()
	}

	kindsMu.Lock()
	defer kindsMu.Unlock()

	if _, exists := kinds[k.Name]; exists {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("Kind already registered").
			WithContext("kind", k.Name).
			Err()
	}
	kinds[k.Name] = k
	return nil
}

// GetKind returns a registered kind by name.
func GetKind(name string) (*Kind, bool) {
	defer perf.Track(nil, "hooks.GetKind")()

	kindsMu.RLock()
	defer kindsMu.RUnlock()

	k, ok := kinds[name]
	return k, ok
}

// ListKinds returns all registered kind names, sorted.
func ListKinds() []string {
	defer perf.Track(nil, "hooks.ListKinds")()

	kindsMu.RLock()
	defer kindsMu.RUnlock()

	names := make([]string, 0, len(kinds))
	for n := range kinds {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ClearKinds removes all registered kinds. For testing only.
func ClearKinds() {
	kindsMu.Lock()
	defer kindsMu.Unlock()
	kinds = make(map[string]*Kind)
}
