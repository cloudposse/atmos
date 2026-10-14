package step

import "github.com/cloudposse/atmos/pkg/perf"

// SetTotalStages sets the total number of stage steps in the workflow.
func (v *Variables) SetTotalStages(total int) {
	defer perf.Track(nil, "step.Variables.SetTotalStages")()

	v.totalStages = total
}

// GetTotalStages returns the total number of stage steps.
func (v *Variables) GetTotalStages() int {
	defer perf.Track(nil, "step.Variables.GetTotalStages")()

	return v.totalStages
}

// IncrementStageIndex increments and returns the current stage index.
func (v *Variables) IncrementStageIndex() int {
	defer perf.Track(nil, "step.Variables.IncrementStageIndex")()

	v.stageIndex++
	return v.stageIndex
}

// GetStageIndex returns the current stage index.
func (v *Variables) GetStageIndex() int {
	defer perf.Track(nil, "step.Variables.GetStageIndex")()

	return v.stageIndex
}
