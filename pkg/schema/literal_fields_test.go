package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsContainerRunStep(t *testing.T) {
	tests := []struct {
		name     string
		stepType string
		action   string
		want     bool
	}{
		{name: "container run", stepType: "container", action: "run", want: true},
		{name: "container with no action defaults to run", stepType: "container", action: "", want: true},
		{name: "surrounding whitespace is ignored", stepType: " container ", action: " run ", want: true},
		{name: "container build", stepType: "container", action: "build", want: false},
		{name: "container push", stepType: "container", action: "push", want: false},
		{name: "container inspect", stepType: "container", action: "inspect", want: false},
		{name: "other type with run action", stepType: "shell", action: "run", want: false},
		{name: "no type", stepType: "", action: "run", want: false},
		{name: "type that only starts with container", stepType: "containerized", action: "run", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsContainerRunStep(tt.stepType, tt.action))
		})
	}
}
