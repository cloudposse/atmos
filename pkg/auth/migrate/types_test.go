package migrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepStatus_String(t *testing.T) {
	tests := []struct {
		status   StepStatus
		expected string
	}{
		{StepNeeded, "needed"},
		{StepComplete, "already complete"},
		{StepNotApplicable, "not applicable"},
		{StepStatus(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}
