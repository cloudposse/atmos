package component

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutionContextGoContext(t *testing.T) {
	type key struct{}
	caller := context.WithValue(context.Background(), key{}, "caller")
	tests := []struct {
		name     string
		execCtx  *ExecutionContext
		want     context.Context
		identity bool
	}{
		{name: "nil receiver", want: context.Background()},
		{name: "nil context", execCtx: &ExecutionContext{}, want: context.Background()},
		{name: "caller context", execCtx: &ExecutionContext{Context: caller}, want: caller, identity: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.execCtx.GoContext()
			assert.Equal(t, tt.want, got)
			if tt.identity {
				assert.Same(t, tt.want, got)
			}
		})
	}
}
