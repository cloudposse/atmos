package step

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var stageInitOnce sync.Once

// initStageTestIO initializes the I/O context for stage tests.
func initStageTestIO(t *testing.T) {
	t.Helper()
	stageInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// StageHandler registration and validation are tested in command_handlers_test.go.
// This file tests helper methods and CountStages function.

func TestCountStages(t *testing.T) {
	tests := []struct {
		name     string
		workflow *schema.WorkflowDefinition
		expected int
	}{
		{
			name:     "empty workflow",
			workflow: &schema.WorkflowDefinition{},
			expected: 0,
		},
		{
			name: "no stage steps",
			workflow: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "step1", Type: "shell"},
					{Name: "step2", Type: "format"},
				},
			},
			expected: 0,
		},
		{
			name: "one stage step",
			workflow: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "step1", Type: "shell"},
					{Name: "stage1", Type: "stage"},
					{Name: "step2", Type: "format"},
				},
			},
			expected: 1,
		},
		{
			name: "multiple stage steps",
			workflow: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "stage1", Type: "stage"},
					{Name: "step1", Type: "shell"},
					{Name: "stage2", Type: "stage"},
					{Name: "step2", Type: "format"},
					{Name: "stage3", Type: "stage"},
				},
			},
			expected: 3,
		},
		{
			name: "all stage steps",
			workflow: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "stage1", Type: "stage"},
					{Name: "stage2", Type: "stage"},
					{Name: "stage3", Type: "stage"},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountStages(tt.workflow)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatStageOutput(t *testing.T) {
	initStageTestIO(t)

	tests := []struct {
		name  string
		index int
		total int
		title string
		check func(t *testing.T, result string)
	}{
		{
			name:  "basic format",
			index: 1,
			total: 3,
			title: "Setup",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "Stage 1/3")
				assert.Contains(t, result, "Setup")
			},
		},
		{
			name:  "middle stage",
			index: 2,
			total: 5,
			title: "Deploy",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "Stage 2/5")
				assert.Contains(t, result, "Deploy")
			},
		},
		{
			name:  "last stage",
			index: 3,
			total: 3,
			title: "Cleanup",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "Stage 3/3")
				assert.Contains(t, result, "Cleanup")
			},
		},
		{
			name:  "empty title",
			index: 1,
			total: 1,
			title: "",
			check: func(t *testing.T, result string) {
				assert.Contains(t, result, "Stage 1/1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStageOutput(tt.index, tt.total, tt.title)
			tt.check(t, result)
		})
	}
}

func TestStageHandlerExecution(t *testing.T) {
	initStageTestIO(t)
	handler, ok := Get("stage")
	require.True(t, ok)

	t.Run("basic stage execution", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "setup",
			Type:  "stage",
			Title: "Setup Phase",
		}
		vars := NewVariables()
		vars.SetTotalStages(3)

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Setup Phase", result.Value)
		assert.Equal(t, 1, vars.GetStageIndex())
	})

	t.Run("stage without title uses name", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "deploy_stage",
			Type: "stage",
		}
		vars := NewVariables()
		vars.SetTotalStages(2)

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "deploy_stage", result.Value)
	})

	t.Run("stage with template title", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "deploy",
			Type:  "stage",
			Title: "Deploying {{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.SetTotalStages(2)
		vars.Set("env", NewStepResult("production"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Deploying production", result.Value)
	})

	t.Run("stage with invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "deploy",
			Type:  "stage",
			Title: "Deploying {{ .steps.env.value",
		}
		vars := NewVariables()
		vars.SetTotalStages(2)

		_, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
	})

	t.Run("multiple stages increment index", func(t *testing.T) {
		vars := NewVariables()
		vars.SetTotalStages(3)

		// First stage.
		step1 := &schema.WorkflowStep{Name: "stage1", Type: "stage", Title: "Stage 1"}
		result1, err1 := handler.Execute(context.Background(), step1, vars)
		require.NoError(t, err1)
		assert.Equal(t, "Stage 1", result1.Value)
		assert.Equal(t, 1, vars.GetStageIndex())

		// Second stage.
		step2 := &schema.WorkflowStep{Name: "stage2", Type: "stage", Title: "Stage 2"}
		result2, err2 := handler.Execute(context.Background(), step2, vars)
		require.NoError(t, err2)
		assert.Equal(t, "Stage 2", result2.Value)
		assert.Equal(t, 2, vars.GetStageIndex())

		// Third stage.
		step3 := &schema.WorkflowStep{Name: "stage3", Type: "stage", Title: "Stage 3"}
		result3, err3 := handler.Execute(context.Background(), step3, vars)
		require.NoError(t, err3)
		assert.Equal(t, "Stage 3", result3.Value)
		assert.Equal(t, 3, vars.GetStageIndex())
	})
}

func TestStageHandlerValidation(t *testing.T) {
	handler, ok := Get("stage")
	require.True(t, ok)

	t.Run("empty step is valid", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test_stage",
			Type: "stage",
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})

	t.Run("step with title is valid", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test_stage",
			Type:  "stage",
			Title: "Stage Title",
		}
		err := handler.Validate(step)
		assert.NoError(t, err)
	})
}
