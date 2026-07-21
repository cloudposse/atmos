package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlanfileVerifyMode_IsValid(t *testing.T) {
	tests := []struct {
		name string
		mode PlanfileVerifyMode
		want bool
	}{
		{"empty (unset) is valid", "", true},
		{"fail is valid", PlanfileVerifyFail, true},
		{"warn is valid", PlanfileVerifyWarn, true},
		{"off is valid", PlanfileVerifyOff, true},
		{"unknown value is invalid", PlanfileVerifyMode("bogus"), false},
		{"uppercase is invalid (case-sensitive)", PlanfileVerifyMode("FAIL"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mode.IsValid())
		})
	}
}
