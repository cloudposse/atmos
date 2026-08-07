package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/taskgraph"
)

// customAnnotation marks a fake *cobra.Command as "custom" for isCustom in tests, decoupled
// from cmd package's real annotation implementation (that's the whole point of the injected
// IsCustomCommand seam).
const customAnnotation = "test.custom"

func markCustom(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[customAnnotation] = "true"
	return cmd
}

func isCustomForTest(cmd *cobra.Command) bool {
	return cmd.Annotations != nil && cmd.Annotations[customAnnotation] == "true"
}

func newFakeRoot() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	root.SetContext(context.Background())
	return root
}

func TestFindRegisteredCommand_DirectMatch(t *testing.T) {
	root := newFakeRoot()
	build := markCustom(&cobra.Command{Use: "build"})
	root.AddCommand(build)

	found, ok := FindRegisteredCommand(root, "build", isCustomForTest)
	require.True(t, ok)
	assert.Same(t, build, found)
}

func TestFindRegisteredCommand_NestedMatch(t *testing.T) {
	root := newFakeRoot()
	parent := markCustom(&cobra.Command{Use: "parent"})
	child := markCustom(&cobra.Command{Use: "child"})
	parent.AddCommand(child)
	root.AddCommand(parent)

	found, ok := FindRegisteredCommand(root, "child", isCustomForTest)
	require.True(t, ok)
	assert.Same(t, child, found)
}

func TestFindRegisteredCommand_NotFound(t *testing.T) {
	root := newFakeRoot()
	root.AddCommand(markCustom(&cobra.Command{Use: "build"}))

	_, ok := FindRegisteredCommand(root, "missing", isCustomForTest)
	assert.False(t, ok)
}

// TestFindRegisteredCommand_SkipsNonCustomCommands verifies the guard that motivated this
// function's restriction in the first place: a built-in (non-custom) command sharing a leaf
// name with the target must never shadow it, and must not be traversed into either.
func TestFindRegisteredCommand_SkipsNonCustomCommands(t *testing.T) {
	root := newFakeRoot()
	builtin := &cobra.Command{Use: "test"} // NOT marked custom
	builtinChild := markCustom(&cobra.Command{Use: "test"})
	builtin.AddCommand(builtinChild)
	root.AddCommand(builtin)

	_, ok := FindRegisteredCommand(root, "test", isCustomForTest)
	assert.False(t, ok, "a non-custom command must not match, even when its name matches, and its subtree must not be searched")
}

func TestDependenciesAlreadyResolved(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		cmd := &cobra.Command{Use: "x"}
		assert.False(t, DependenciesAlreadyResolved(cmd))
	})

	t.Run("unmarked context", func(t *testing.T) {
		cmd := &cobra.Command{Use: "x"}
		cmd.SetContext(context.Background())
		assert.False(t, DependenciesAlreadyResolved(cmd))
	})

	t.Run("marked context", func(t *testing.T) {
		cmd := &cobra.Command{Use: "x"}
		cmd.SetContext(WithDependenciesResolved(context.Background()))
		assert.True(t, DependenciesAlreadyResolved(cmd))
	})
}

func TestCustomCommandRunner_UnregisteredDependencyErrors(t *testing.T) {
	root := newFakeRoot()
	runner := CustomCommandRunner(root, isCustomForTest)

	err := runner(context.Background(), taskgraph.Ref{Kind: taskgraph.KindCommand, Name: "missing"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrCustomCommandDependencyNotRegistered))
}

func TestCustomCommandRunner_SetsFlagsAndInvokesRun(t *testing.T) {
	root := newFakeRoot()
	var ran bool
	var preRan bool
	var receivedArgs []string
	var resolvedDuringRun bool

	target := markCustom(&cobra.Command{Use: "build"})
	target.PersistentFlags().String("env", "dev", "")
	target.PreRun = func(_ *cobra.Command, _ []string) { preRan = true }
	target.Run = func(cmd *cobra.Command, args []string) {
		ran = true
		receivedArgs = args
		resolvedDuringRun = DependenciesAlreadyResolved(cmd)
		// Custom-command flags are registered via PersistentFlags; cmd.Flags() (the local set)
		// does not merge them in outside Cobra's own Execute() path (see CustomCommandRunner's
		// doc comment) -- read back via PersistentFlags() to match.
		val, err := cmd.PersistentFlags().GetString("env")
		require.NoError(t, err)
		assert.Equal(t, "prod", val)
	}
	root.AddCommand(target)

	runner := CustomCommandRunner(root, isCustomForTest)
	err := runner(context.Background(), taskgraph.Ref{
		Kind:  taskgraph.KindCommand,
		Name:  "build",
		Flags: map[string]string{"env": "prod"},
		Args:  []string{"arg1"},
	})

	require.NoError(t, err)
	assert.True(t, preRan, "PreRun must run before Run, mirroring Cobra's own lifecycle")
	assert.True(t, ran)
	assert.Equal(t, []string{"arg1"}, receivedArgs)
	assert.True(t, resolvedDuringRun, "the target's context must be marked while it is actually running, so it does not re-resolve its own dependencies")
	assert.False(t, DependenciesAlreadyResolved(target), "the target's context must be restored to its pre-dispatch state once the dispatch returns, so it doesn't leak into a later invocation")
}

// TestCustomCommandRunner_RestoresContextForSubsequentTopLevelInvocation is the regression case
// for the context-leak bug: after a command runs once as a dependency, invoking the SAME
// *cobra.Command object again directly (e.g. a long-lived process like `atmos mcp server`, or a
// test suite reusing RootCmd across cases) must NOT see the stale dependencies-resolved marker or
// route its own error into the first dispatch's already-drained error sink.
func TestCustomCommandRunner_RestoresContextForSubsequentTopLevelInvocation(t *testing.T) {
	root := newFakeRoot()

	target := markCustom(&cobra.Command{Use: "build"})
	target.Run = func(*cobra.Command, []string) {}
	root.AddCommand(target)

	runner := CustomCommandRunner(root, isCustomForTest)
	require.NoError(t, runner(context.Background(), taskgraph.Ref{Kind: taskgraph.KindCommand, Name: "build"}))

	// Simulate a later, direct (non-dependency) invocation of the same command object.
	var sawResolvedMarker bool
	target.Run = func(cmd *cobra.Command, _ []string) {
		sawResolvedMarker = DependenciesAlreadyResolved(cmd)
		// A genuine failure from THIS invocation must not be swallowed by the first
		// dispatch's now-stale (and already-read) error sink.
		RecordDependencyError(cmd, errors.New("second invocation failed"))
	}
	target.Run(target, nil)

	assert.False(t, sawResolvedMarker, "a subsequent top-level invocation must not inherit the dependency dispatch's resolved marker")
}

func TestCustomCommandRunner_InvalidFlagErrors(t *testing.T) {
	root := newFakeRoot()
	target := markCustom(&cobra.Command{Use: "build"})
	target.Run = func(*cobra.Command, []string) {}
	root.AddCommand(target)

	runner := CustomCommandRunner(root, isCustomForTest)
	err := runner(context.Background(), taskgraph.Ref{
		Kind:  taskgraph.KindCommand,
		Name:  "build",
		Flags: map[string]string{"does-not-exist": "value"},
	})
	require.Error(t, err)
}

func TestCustomCommandRunner_NoPreRunIsOptional(t *testing.T) {
	root := newFakeRoot()
	var ran bool
	target := markCustom(&cobra.Command{Use: "build"})
	target.Run = func(*cobra.Command, []string) { ran = true }
	root.AddCommand(target)

	runner := CustomCommandRunner(root, isCustomForTest)
	err := runner(context.Background(), taskgraph.Ref{Kind: taskgraph.KindCommand, Name: "build"})
	require.NoError(t, err)
	assert.True(t, ran)
}

func TestCustomCommandDependencyOptions_ReturnsAllFourOptions(t *testing.T) {
	root := newFakeRoot()
	opts := CustomCommandDependencyOptions(&schema.AtmosConfiguration{}, root, isCustomForTest)
	assert.Len(t, opts, 4, "must wire command runner, command lookup, workflow runner, and workflow lookup")
}
