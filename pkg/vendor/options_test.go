package vendor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPullOptions(t *testing.T) {
	// Test default values.
	opts := &PullOptions{}
	assert.Empty(t, opts.Component)
	assert.Empty(t, opts.Stack)
	assert.Empty(t, opts.Tags)
	assert.False(t, opts.DryRun)
	assert.Empty(t, opts.ComponentType)

	// Test WithComponent.
	WithComponent("vpc")(opts)
	assert.Equal(t, "vpc", opts.Component)

	// Test WithStack.
	WithStack("dev-us-east-1")(opts)
	assert.Equal(t, "dev-us-east-1", opts.Stack)

	// Test WithTags.
	WithTags([]string{"networking", "core"})(opts)
	assert.Equal(t, []string{"networking", "core"}, opts.Tags)

	// Test WithDryRun.
	WithDryRun(true)(opts)
	assert.True(t, opts.DryRun)

	// Test WithComponentType.
	WithComponentType("helmfile")(opts)
	assert.Equal(t, "helmfile", opts.ComponentType)
}

func TestComponentOptions(t *testing.T) {
	// Test default values.
	opts := &ComponentOptions{}
	assert.Empty(t, opts.ComponentType)
	assert.False(t, opts.DryRun)

	// Test WithComponentDryRun.
	WithComponentDryRun(true)(opts)
	assert.True(t, opts.DryRun)

	// Test WithComponentComponentType.
	WithComponentComponentType("packer")(opts)
	assert.Equal(t, "packer", opts.ComponentType)
}

func TestStackOptions(t *testing.T) {
	// Test default values.
	opts := &StackOptions{}
	assert.False(t, opts.DryRun)

	// Test WithStackDryRun.
	WithStackDryRun(true)(opts)
	assert.True(t, opts.DryRun)
}

func TestPullOptionsChaining(t *testing.T) {
	// Test that options can be chained.
	opts := &PullOptions{
		ComponentType: "terraform",
	}

	// Apply multiple options.
	optionFuncs := []PullOption{
		WithComponent("vpc"),
		WithDryRun(true),
		WithTags([]string{"networking"}),
	}

	for _, opt := range optionFuncs {
		opt(opts)
	}

	assert.Equal(t, "vpc", opts.Component)
	assert.True(t, opts.DryRun)
	assert.Equal(t, []string{"networking"}, opts.Tags)
	// ComponentType should remain unchanged.
	assert.Equal(t, "terraform", opts.ComponentType)
}
