package function

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultRegistry creates a new registry with all default functions registered.
// The shellExecutor parameter provides the implementation for the !exec function.
// If shellExecutor is nil, the !exec function will not be available.
func DefaultRegistry(shellExecutor ShellExecutor) *Registry {
	defer perf.Track(nil, "function.DefaultRegistry")()

	r := NewRegistry()

	// Register pre-merge functions.
	_ = r.Register(NewEnvFunction())
	_ = r.Register(NewTemplateFunction())
	_ = r.Register(NewGitRootFunction())

	// Register exec function only if executor is provided.
	if shellExecutor != nil {
		_ = r.Register(NewExecFunction(shellExecutor))
	}

	return r
}

// Tags returns a map of function tags to function names.
// This is useful for detecting function calls in string values.
func Tags() map[string]string {
	defer perf.Track(nil, "function.Tags")()

	return map[string]string{
		TagEnv:      "env",
		TagExec:     "exec",
		TagTemplate: "template",
		TagGitRoot:  "repo-root",
	}
}

// AllTags returns a list of all known function tags.
func AllTags() []string {
	defer perf.Track(nil, "function.AllTags")()

	return []string{
		TagEnv,
		TagExec,
		TagTemplate,
		TagGitRoot,
	}
}
