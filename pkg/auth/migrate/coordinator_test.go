package migrate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/auth/migrate/mocks"
)

// newStep creates a mock step with Name and Description pre-configured.
func newStep(ctrl *gomock.Controller, name, desc string) *mocks.MockMigrationStep {
	step := mocks.NewMockMigrationStep(ctrl)
	step.EXPECT().Name().Return(name).AnyTimes()
	step.EXPECT().Description().Return(desc).AnyTimes()
	return step
}

func TestCoordinator_Run_AllComplete(t *testing.T) {
	ctrl := gomock.NewController(t)

	step1 := newStep(ctrl, "step-1", "First step")
	step1.EXPECT().Detect(gomock.Any()).Return(migrate.StepComplete, nil)

	step2 := newStep(ctrl, "step-2", "Second step")
	step2.EXPECT().Detect(gomock.Any()).Return(migrate.StepComplete, nil)

	// No Plan, Apply, or prompts should be called.
	prompter := mocks.NewMockPrompter(ctrl)

	coord := migrate.NewCoordinator(
		[]migrate.MigrationStep{step1, step2},
		prompter,
	)

	err := coord.Run(context.Background(), false, false)
	require.NoError(t, err)
}

func TestCoordinator_Run_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)

	step1 := newStep(ctrl, "step-1", "First step")
	step1.EXPECT().Detect(gomock.Any()).Return(migrate.StepNeeded, nil)
	step1.EXPECT().Plan(gomock.Any()).Return([]migrate.Change{
		{FilePath: "a.yaml", Description: "update a"},
	}, nil)

	// No Apply or prompts should be called.
	prompter := mocks.NewMockPrompter(ctrl)

	coord := migrate.NewCoordinator(
		[]migrate.MigrationStep{step1},
		prompter,
	)

	err := coord.Run(context.Background(), true, false)
	require.NoError(t, err)
}

func TestCoordinator_Run_Force(t *testing.T) {
	ctrl := gomock.NewController(t)

	step1 := newStep(ctrl, "step-1", "First step")
	step1.EXPECT().Detect(gomock.Any()).Return(migrate.StepNeeded, nil)
	step1.EXPECT().Plan(gomock.Any()).Return([]migrate.Change{
		{FilePath: "a.yaml", Description: "update a"},
	}, nil)
	step1.EXPECT().Apply(gomock.Any()).Return(nil)

	// No prompts should be called.
	prompter := mocks.NewMockPrompter(ctrl)

	coord := migrate.NewCoordinator(
		[]migrate.MigrationStep{step1},
		prompter,
	)

	err := coord.Run(context.Background(), false, true)
	require.NoError(t, err)
}

func TestCoordinator_Run_StepByStep(t *testing.T) {
	ctrl := gomock.NewController(t)

	step1 := newStep(ctrl, "step-1", "First step")
	step1.EXPECT().Detect(gomock.Any()).Return(migrate.StepNeeded, nil)
	step1.EXPECT().Plan(gomock.Any()).Return([]migrate.Change{
		{FilePath: "a.yaml", Description: "update a"},
	}, nil)
	step1.EXPECT().Apply(gomock.Any()).Return(nil)

	step2 := newStep(ctrl, "step-2", "Second step")
	step2.EXPECT().Detect(gomock.Any()).Return(migrate.StepNeeded, nil)
	step2.EXPECT().Plan(gomock.Any()).Return([]migrate.Change{
		{FilePath: "b.yaml", Description: "update b"},
	}, nil)
	// step2.Apply should NOT be called because user skips it.

	prompter := mocks.NewMockPrompter(ctrl)
	prompter.EXPECT().SelectAction().Return(migrate.ActionStepByStep, nil)
	gomock.InOrder(
		prompter.EXPECT().Confirm(`Apply step "First step"?`).Return(true, nil),
		prompter.EXPECT().Confirm(`Apply step "Second step"?`).Return(false, nil),
	)

	coord := migrate.NewCoordinator(
		[]migrate.MigrationStep{step1, step2},
		prompter,
	)

	err := coord.Run(context.Background(), false, false)
	require.NoError(t, err)
}

func TestCoordinator_Run_Cancel(t *testing.T) {
	ctrl := gomock.NewController(t)

	step1 := newStep(ctrl, "step-1", "First step")
	step1.EXPECT().Detect(gomock.Any()).Return(migrate.StepNeeded, nil)
	step1.EXPECT().Plan(gomock.Any()).Return([]migrate.Change{
		{FilePath: "a.yaml", Description: "update a"},
	}, nil)
	// No Apply should be called.

	prompter := mocks.NewMockPrompter(ctrl)
	prompter.EXPECT().SelectAction().Return(migrate.ActionCancel, nil)

	coord := migrate.NewCoordinator(
		[]migrate.MigrationStep{step1},
		prompter,
	)

	err := coord.Run(context.Background(), false, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrMigrationAborted))
}

func TestCoordinator_Run_ApplyAll(t *testing.T) {
	ctrl := gomock.NewController(t)

	step1 := newStep(ctrl, "step-1", "First step")
	step1.EXPECT().Detect(gomock.Any()).Return(migrate.StepNeeded, nil)
	step1.EXPECT().Plan(gomock.Any()).Return([]migrate.Change{
		{FilePath: "a.yaml", Description: "update a"},
	}, nil)
	step1.EXPECT().Apply(gomock.Any()).Return(nil)

	step2 := newStep(ctrl, "step-2", "Second step")
	step2.EXPECT().Detect(gomock.Any()).Return(migrate.StepNeeded, nil)
	step2.EXPECT().Plan(gomock.Any()).Return([]migrate.Change{
		{FilePath: "b.yaml", Description: "update b"},
	}, nil)
	step2.EXPECT().Apply(gomock.Any()).Return(nil)

	prompter := mocks.NewMockPrompter(ctrl)
	prompter.EXPECT().SelectAction().Return(migrate.ActionApplyAll, nil)

	coord := migrate.NewCoordinator(
		[]migrate.MigrationStep{step1, step2},
		prompter,
	)

	err := coord.Run(context.Background(), false, false)
	require.NoError(t, err)
}
