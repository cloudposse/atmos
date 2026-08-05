package step

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetShowConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		step     *schema.WorkflowStep
		workflow *schema.WorkflowDefinition
		expected *schema.ShowConfig
	}{
		{
			name:     "nil_step_and_workflow_returns_empty_config",
			step:     nil,
			workflow: nil,
			expected: &schema.ShowConfig{},
		},
		{
			name: "workflow_level_settings_are_applied",
			step: nil,
			workflow: &schema.WorkflowDefinition{
				Show: &schema.ShowConfig{
					Header:   BoolPtr(true),
					Progress: BoolPtr(true),
					Labels:   BoolPtr(false),
				},
			},
			expected: &schema.ShowConfig{
				Header:   BoolPtr(true),
				Progress: BoolPtr(true),
				Labels:   BoolPtr(false),
			},
		},
		{
			name: "step_overrides_workflow",
			step: &schema.WorkflowStep{
				Show: &schema.ShowConfig{
					Header: BoolPtr(false), // Override workflow's true.
					Count:  BoolPtr(true),  // Add new setting.
					Labels: BoolPtr(true),  // Override workflow's false.
				},
			},
			workflow: &schema.WorkflowDefinition{
				Show: &schema.ShowConfig{
					Header:   BoolPtr(true),
					Progress: BoolPtr(true),
					Labels:   BoolPtr(false),
				},
			},
			expected: &schema.ShowConfig{
				Header:   BoolPtr(false), // From step.
				Progress: BoolPtr(true),  // From workflow.
				Count:    BoolPtr(true),  // From step.
				Labels:   BoolPtr(true),  // From step.
			},
		},
		{
			name: "step_only_settings",
			step: &schema.WorkflowStep{
				Show: &schema.ShowConfig{
					Command: BoolPtr(true),
				},
			},
			workflow: nil,
			expected: &schema.ShowConfig{
				Command: BoolPtr(true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := GetShowConfig(tt.step, tt.workflow)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShowHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      *schema.ShowConfig
		header   bool
		flags    bool
		command  bool
		count    bool
		progress bool
		labels   bool
	}{
		{
			name:     "nil_config_returns_false_for_all",
			cfg:      nil,
			header:   false,
			flags:    false,
			command:  false,
			count:    false,
			progress: false,
			labels:   true,
		},
		{
			name:     "empty_config_returns_false_for_all",
			cfg:      &schema.ShowConfig{},
			header:   false,
			flags:    false,
			command:  false,
			count:    false,
			progress: false,
			labels:   true,
		},
		{
			name: "explicit_false_returns_false",
			cfg: &schema.ShowConfig{
				Header:   BoolPtr(false),
				Flags:    BoolPtr(false),
				Command:  BoolPtr(false),
				Count:    BoolPtr(false),
				Progress: BoolPtr(false),
				Labels:   BoolPtr(false),
			},
			header:   false,
			flags:    false,
			command:  false,
			count:    false,
			progress: false,
			labels:   false,
		},
		{
			name: "explicit_true_returns_true",
			cfg: &schema.ShowConfig{
				Header:   BoolPtr(true),
				Flags:    BoolPtr(true),
				Command:  BoolPtr(true),
				Count:    BoolPtr(true),
				Progress: BoolPtr(true),
				Labels:   BoolPtr(true),
			},
			header:   true,
			flags:    true,
			command:  true,
			count:    true,
			progress: true,
			labels:   true,
		},
		{
			name: "mixed_values",
			cfg: &schema.ShowConfig{
				Header:  BoolPtr(true),
				Command: BoolPtr(true),
			},
			header:   true,
			flags:    false,
			command:  true,
			count:    false,
			progress: false,
			labels:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.header, ShowHeader(tt.cfg), "ShowHeader mismatch")
			assert.Equal(t, tt.flags, ShowFlags(tt.cfg), "ShowFlags mismatch")
			assert.Equal(t, tt.command, ShowCommand(tt.cfg), "ShowCommand mismatch")
			assert.Equal(t, tt.count, ShowCount(tt.cfg), "ShowCount mismatch")
			assert.Equal(t, tt.progress, ShowProgress(tt.cfg), "ShowProgress mismatch")
			assert.Equal(t, tt.labels, ShowLabels(tt.cfg), "ShowLabels mismatch")
		})
	}
}

func TestMergeShowConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		dst      *schema.ShowConfig
		src      *schema.ShowConfig
		expected *schema.ShowConfig
	}{
		{
			name:     "nil_src_returns_dst",
			dst:      &schema.ShowConfig{Header: BoolPtr(true)},
			src:      nil,
			expected: &schema.ShowConfig{Header: BoolPtr(true)},
		},
		{
			name:     "nil_dst_creates_new",
			dst:      nil,
			src:      &schema.ShowConfig{Header: BoolPtr(true)},
			expected: &schema.ShowConfig{Header: BoolPtr(true)},
		},
		{
			name: "src_overrides_dst",
			dst: &schema.ShowConfig{
				Header: BoolPtr(true),
				Flags:  BoolPtr(true),
				Labels: BoolPtr(true),
			},
			src: &schema.ShowConfig{
				Header:  BoolPtr(false),
				Command: BoolPtr(true),
				Labels:  BoolPtr(false),
			},
			expected: &schema.ShowConfig{
				Header:  BoolPtr(false), // Overridden.
				Flags:   BoolPtr(true),  // Preserved.
				Command: BoolPtr(true),  // Added.
				Labels:  BoolPtr(false), // Overridden.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := mergeShowConfig(tt.dst, tt.src)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBoolPtr(t *testing.T) {
	t.Parallel()

	truePtr := BoolPtr(true)
	falsePtr := BoolPtr(false)

	assert.NotNil(t, truePtr)
	assert.NotNil(t, falsePtr)
	assert.True(t, *truePtr)
	assert.False(t, *falsePtr)
}
