package component

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutionContextGoContext(t *testing.T) {
	assert.NotNil(t, (*ExecutionContext)(nil).GoContext())
	assert.NotNil(t, (&ExecutionContext{}).GoContext())

	type key struct{}
	want := context.WithValue(context.Background(), key{}, "caller")
	assert.Same(t, want, (&ExecutionContext{Context: want}).GoContext())
}
