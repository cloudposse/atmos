package step

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeEmulatorRunner records the calls made by the emulator step.
type fakeEmulatorRunner struct {
	calls     []string
	component string
	stack     string
	ephemeral bool
	err       error
}

func (f *fakeEmulatorRunner) Up(_ context.Context, component, stack string, ephemeral bool) error {
	f.calls = append(f.calls, "up")
	f.component, f.stack, f.ephemeral = component, stack, ephemeral
	return f.err
}

func (f *fakeEmulatorRunner) Down(_ context.Context, component, stack string) error {
	f.calls = append(f.calls, "down")
	f.component, f.stack = component, stack
	return f.err
}

func (f *fakeEmulatorRunner) Reset(_ context.Context, component, stack string) error {
	f.calls = append(f.calls, "reset")
	f.component, f.stack = component, stack
	return f.err
}

// withEmulatorRunner swaps the package-level runner for the duration of a test.
func withEmulatorRunner(t *testing.T, r EmulatorRunner) {
	t.Helper()
	prev := emulatorRunner
	emulatorRunner = r
	t.Cleanup(func() { emulatorRunner = prev })
}

func TestEmulatorHandler_Registered(t *testing.T) {
	h, ok := Get(emulatorStepType)
	require.True(t, ok, "emulator step type must be registered")
	assert.Equal(t, emulatorStepType, h.GetName())
}

func TestEmulatorHandler_UpDefaultsActionAndStack(t *testing.T) {
	fake := &fakeEmulatorRunner{}
	withEmulatorRunner(t, fake)

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	vars := NewVariables()
	vars.SetEnv("ATMOS_STACK", "local")

	// No action -> defaults to "up"; no stack on the step -> ATMOS_STACK.
	step := &schema.WorkflowStep{Name: "start", Type: "emulator", Component: "aws"}
	result, err := h.Execute(context.Background(), step, vars)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"up"}, fake.calls)
	assert.Equal(t, "aws", fake.component)
	assert.Equal(t, "local", fake.stack)
	assert.False(t, fake.ephemeral)
}

func TestEmulatorHandler_DownExplicitStackAndEphemeral(t *testing.T) {
	fake := &fakeEmulatorRunner{}
	withEmulatorRunner(t, fake)

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	vars := NewVariables()
	vars.SetEnv("ATMOS_STACK", "ignored")

	step := &schema.WorkflowStep{Name: "stop", Type: "emulator", Component: "aws", Action: "down", Stack: "prod"}
	_, err := h.Execute(context.Background(), step, vars)

	require.NoError(t, err)
	assert.Equal(t, []string{"down"}, fake.calls)
	assert.Equal(t, "prod", fake.stack, "explicit step stack should override ATMOS_STACK")
}

func TestEmulatorHandler_UpEphemeral(t *testing.T) {
	fake := &fakeEmulatorRunner{}
	withEmulatorRunner(t, fake)

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	vars := NewVariables()

	step := &schema.WorkflowStep{Name: "start", Type: "emulator", Component: "aws", Action: "up", Ephemeral: true}
	_, err := h.Execute(context.Background(), step, vars)

	require.NoError(t, err)
	assert.True(t, fake.ephemeral)
}

func TestEmulatorHandler_Reset(t *testing.T) {
	fake := &fakeEmulatorRunner{}
	withEmulatorRunner(t, fake)

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	step := &schema.WorkflowStep{Name: "reset", Type: "emulator", Component: "aws", Action: "reset"}
	_, err := h.Execute(context.Background(), step, NewVariables())

	require.NoError(t, err)
	assert.Equal(t, []string{"reset"}, fake.calls)
}

func TestEmulatorHandler_PropagatesRunnerError(t *testing.T) {
	sentinel := errors.New("runtime not available")
	fake := &fakeEmulatorRunner{err: sentinel}
	withEmulatorRunner(t, fake)

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	step := &schema.WorkflowStep{Name: "start", Type: "emulator", Component: "aws"}
	_, err := h.Execute(context.Background(), step, NewVariables())

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestEmulatorHandler_Validate(t *testing.T) {
	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}

	// Missing component.
	require.Error(t, h.Validate(&schema.WorkflowStep{Name: "x", Type: "emulator", Action: "up"}))

	// Invalid action.
	require.Error(t, h.Validate(&schema.WorkflowStep{Name: "x", Type: "emulator", Component: "aws", Action: "bogus"}))

	// Valid up/down/reset and empty (defaults to up).
	for _, action := range []string{"", "up", "down", "reset"} {
		require.NoError(t, h.Validate(&schema.WorkflowStep{Name: "x", Type: "emulator", Component: "aws", Action: action}))
	}
}

func TestEmulatorHandler_ComponentResolveError(t *testing.T) {
	withEmulatorRunner(t, &fakeEmulatorRunner{})

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	// A malformed template in `component` fails to resolve.
	step := &schema.WorkflowStep{Name: "start", Type: "emulator", Component: "{{ .nope"}
	_, err := h.Execute(context.Background(), step, NewVariables())

	require.Error(t, err)
}

func TestEmulatorHandler_StackResolveError(t *testing.T) {
	withEmulatorRunner(t, &fakeEmulatorRunner{})

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	// A malformed template in an explicit `stack` fails to resolve.
	step := &schema.WorkflowStep{Name: "start", Type: "emulator", Component: "aws", Stack: "{{ .nope"}
	_, err := h.Execute(context.Background(), step, NewVariables())

	require.Error(t, err)
}

func TestEmulatorHandler_NoRunnerRegistered(t *testing.T) {
	withEmulatorRunner(t, nil)

	h := &EmulatorHandler{BaseHandler: NewBaseHandler(emulatorStepType, CategoryCommand, false)}
	step := &schema.WorkflowStep{Name: "start", Type: "emulator", Component: "aws"}
	_, err := h.Execute(context.Background(), step, NewVariables())

	require.Error(t, err)
}
