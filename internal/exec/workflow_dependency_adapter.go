package exec

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/taskgraph"
)

// WorkflowLookup resolves a taskgraph.Ref{Kind: KindWorkflow} to its own nested
// dependencies.workflows/dependencies.commands entries, used by both workflow-depends-on-
// workflow (defaultFile = the current workflow's own path -- same-file resolution when
// ref.File is empty) and command-depends-on-workflow (defaultFile = "" -- ref.File is
// required there, since a custom command has no "current workflow file" to default to).
func WorkflowLookup(atmosConfig *schema.AtmosConfiguration, defaultFile string) taskgraph.Lookup {
	defer perf.Track(atmosConfig, "exec.WorkflowLookup")()

	return func(ref taskgraph.Ref) ([]taskgraph.Ref, bool, error) {
		file := ref.File
		if file == "" {
			file = defaultFile
		}
		if file == "" {
			return nil, false, errUtils.Build(errUtils.ErrWorkflowNoWorkflow).
				WithExplanationf("dependencies.workflows entry %q must set 'file' outside a workflow file", ref.Name).
				WithHint("Add 'file: <workflow-file>.yaml' to this dependency entry").
				Err()
		}
		workflowConfig, err := LoadWorkflowConfig(ResolveWorkflowFilePath(atmosConfig, file))
		if err != nil {
			return nil, false, err
		}
		def, ok := workflowConfig[ref.Name]
		if !ok {
			return nil, false, nil
		}
		return taskgraph.RefsFromDependencies(def.Dependencies.OrEmpty()), true, nil
	}
}

// WorkflowRunner resolves and executes a taskgraph.Ref{Kind: KindWorkflow} dependency,
// reusing the same file-resolution rules as WorkflowLookup and the already-exported
// ExecuteWorkflow entry point.
func WorkflowRunner(atmosConfig *schema.AtmosConfiguration, defaultFile string, dryRun bool, commandLineIdentity string) taskgraph.Runner {
	defer perf.Track(atmosConfig, "exec.WorkflowRunner")()

	return func(ctx context.Context, ref taskgraph.Ref) error {
		file := ref.File
		if file == "" {
			file = defaultFile
		}
		workflowPath := ResolveWorkflowFilePath(atmosConfig, file)
		workflowConfig, err := LoadWorkflowConfig(workflowPath)
		if err != nil {
			return err
		}
		def, ok := workflowConfig[ref.Name]
		if !ok {
			return fmt.Errorf("%w: %q", errUtils.ErrWorkflowNoWorkflow, ref.Name)
		}
		stack := ref.Flags["stack"]
		return ExecuteWorkflow(*atmosConfig, ref.Name, workflowPath, &def, dryRun, stack, "", commandLineIdentity)
	}
}

// CommandLookup resolves a taskgraph.Ref{Kind: KindCommand} to its own nested
// dependencies.commands/dependencies.workflows entries by searching atmosConfig.Commands,
// pure config data with no dependency on the cmd package's cobra command tree.
func CommandLookup(atmosConfig *schema.AtmosConfiguration) taskgraph.Lookup {
	defer perf.Track(atmosConfig, "exec.CommandLookup")()

	return func(ref taskgraph.Ref) ([]taskgraph.Ref, bool, error) {
		found, ok := schema.FindCommandByName(atmosConfig.Commands, ref.Name)
		if !ok {
			return nil, false, nil
		}
		if found.Dependencies == nil {
			return nil, true, nil
		}
		return taskgraph.RefsFromDependencies(*found.Dependencies), true, nil
	}
}

// commandRunnerViaSubprocess executes a taskgraph.Ref{Kind: KindCommand} dependency declared
// on a WORKFLOW by shelling out to `atmos <name> [flags] [args]`, exactly like an existing
// `type: atmos` workflow step already does (ExecuteShellCommand). This is deliberate, not a
// missed optimization: internal/exec cannot invoke a registered *cobra.Command in-process
// without importing the cmd package, which already imports internal/exec -- doing so directly
// would be an import cycle. Command-depends-on-command (both resolved within the cmd package)
// runs in-process instead; only the workflow-depends-on-command edge pays the subprocess cost.
func commandRunnerViaSubprocess(atmosConfig *schema.AtmosConfiguration) taskgraph.Runner {
	return func(ctx context.Context, ref taskgraph.Ref) error {
		args := []string{ref.Name}
		for name, value := range ref.Flags {
			args = append(args, fmt.Sprintf("--%s=%s", name, value))
		}
		args = append(args, ref.Args...)
		return ExecuteShellCommand(*atmosConfig, "atmos", args, ".", nil, false, "")
	}
}
