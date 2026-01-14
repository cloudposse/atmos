package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewExecutionContext(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stack := "tenant1-ue2-dev"
	component := "vpc"

	ctx := NewExecutionContext(atmosConfig, stack, component)

	require.NotNil(t, ctx)
	assert.Same(t, atmosConfig, ctx.AtmosConfig)
	assert.Equal(t, stack, ctx.Stack)
	assert.Equal(t, component, ctx.Component)
	assert.Empty(t, ctx.BaseDir)
	assert.Empty(t, ctx.File)
	assert.Nil(t, ctx.StackInfo)
}

func TestNewExecutionContext_NilConfig(t *testing.T) {
	ctx := NewExecutionContext(nil, "stack", "component")

	require.NotNil(t, ctx)
	assert.Nil(t, ctx.AtmosConfig)
	assert.Equal(t, "stack", ctx.Stack)
	assert.Equal(t, "component", ctx.Component)
}

func TestNewExecutionContext_EmptyValues(t *testing.T) {
	ctx := NewExecutionContext(nil, "", "")

	require.NotNil(t, ctx)
	assert.Empty(t, ctx.Stack)
	assert.Empty(t, ctx.Component)
}

func TestExecutionContext_WithFile(t *testing.T) {
	original := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "stack1",
		Component:   "comp1",
		BaseDir:     "/base",
	}

	newCtx := original.WithFile("/path/to/file.yaml")

	// New context should have the file set.
	assert.Equal(t, "/path/to/file.yaml", newCtx.File)

	// Original should be unchanged.
	assert.Empty(t, original.File)

	// Other fields should be copied.
	assert.Same(t, original.AtmosConfig, newCtx.AtmosConfig)
	assert.Equal(t, original.Stack, newCtx.Stack)
	assert.Equal(t, original.Component, newCtx.Component)
	assert.Equal(t, original.BaseDir, newCtx.BaseDir)
}

func TestExecutionContext_WithFile_Chaining(t *testing.T) {
	ctx := NewExecutionContext(&schema.AtmosConfiguration{}, "stack", "component")

	result := ctx.WithFile("/file1.yaml").WithFile("/file2.yaml")

	assert.Equal(t, "/file2.yaml", result.File)
}

func TestExecutionContext_WithBaseDir(t *testing.T) {
	original := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "stack1",
		Component:   "comp1",
		File:        "/some/file.yaml",
	}

	newCtx := original.WithBaseDir("/new/base/dir")

	// New context should have the base dir set.
	assert.Equal(t, "/new/base/dir", newCtx.BaseDir)

	// Original should be unchanged.
	assert.Empty(t, original.BaseDir)

	// Other fields should be copied.
	assert.Same(t, original.AtmosConfig, newCtx.AtmosConfig)
	assert.Equal(t, original.Stack, newCtx.Stack)
	assert.Equal(t, original.Component, newCtx.Component)
	assert.Equal(t, original.File, newCtx.File)
}

func TestExecutionContext_WithBaseDir_Chaining(t *testing.T) {
	ctx := NewExecutionContext(&schema.AtmosConfiguration{}, "stack", "component")

	result := ctx.WithBaseDir("/dir1").WithBaseDir("/dir2")

	assert.Equal(t, "/dir2", result.BaseDir)
}

func TestExecutionContext_WithStackInfo(t *testing.T) {
	original := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "stack1",
		Component:   "comp1",
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		Stack:     "test-stack",
		Component: "test-component",
	}

	newCtx := original.WithStackInfo(stackInfo)

	// New context should have the stack info set.
	assert.Same(t, stackInfo, newCtx.StackInfo)

	// Original should be unchanged.
	assert.Nil(t, original.StackInfo)

	// Other fields should be copied.
	assert.Same(t, original.AtmosConfig, newCtx.AtmosConfig)
	assert.Equal(t, original.Stack, newCtx.Stack)
	assert.Equal(t, original.Component, newCtx.Component)
}

func TestExecutionContext_WithStackInfo_Nil(t *testing.T) {
	original := &ExecutionContext{
		AtmosConfig: &schema.AtmosConfiguration{},
		Stack:       "stack1",
		StackInfo:   &schema.ConfigAndStacksInfo{},
	}

	newCtx := original.WithStackInfo(nil)

	assert.Nil(t, newCtx.StackInfo)
}

func TestExecutionContext_MethodChaining(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackInfo := &schema.ConfigAndStacksInfo{
		Stack: "info-stack",
	}

	ctx := NewExecutionContext(atmosConfig, "stack", "component").
		WithFile("/path/to/config.yaml").
		WithBaseDir("/base/dir").
		WithStackInfo(stackInfo)

	assert.Same(t, atmosConfig, ctx.AtmosConfig)
	assert.Equal(t, "stack", ctx.Stack)
	assert.Equal(t, "component", ctx.Component)
	assert.Equal(t, "/path/to/config.yaml", ctx.File)
	assert.Equal(t, "/base/dir", ctx.BaseDir)
	assert.Same(t, stackInfo, ctx.StackInfo)
}

func TestExecutionContext_ImmutableCopy(t *testing.T) {
	// Verify that With* methods create immutable copies.
	original := NewExecutionContext(&schema.AtmosConfiguration{}, "stack", "component")

	copy1 := original.WithFile("/file1.yaml")
	copy2 := original.WithFile("/file2.yaml")

	// Copies should be independent.
	assert.Equal(t, "/file1.yaml", copy1.File)
	assert.Equal(t, "/file2.yaml", copy2.File)
	assert.Empty(t, original.File)

	// Copies should not be the same pointer.
	assert.NotSame(t, copy1, copy2)
	assert.NotSame(t, original, copy1)
	assert.NotSame(t, original, copy2)
}

func TestExecutionContext_FieldAccess(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackInfo := &schema.ConfigAndStacksInfo{
		Stack:     "info-stack",
		Component: "info-component",
		AuthContext: &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				Profile: "prod",
			},
		},
	}

	ctx := &ExecutionContext{
		AtmosConfig: atmosConfig,
		Stack:       "my-stack",
		Component:   "my-component",
		BaseDir:     "/home/user/project",
		File:        "/home/user/project/stacks/config.yaml",
		StackInfo:   stackInfo,
	}

	// Direct field access.
	assert.Same(t, atmosConfig, ctx.AtmosConfig)
	assert.Equal(t, "my-stack", ctx.Stack)
	assert.Equal(t, "my-component", ctx.Component)
	assert.Equal(t, "/home/user/project", ctx.BaseDir)
	assert.Equal(t, "/home/user/project/stacks/config.yaml", ctx.File)
	assert.Same(t, stackInfo, ctx.StackInfo)

	// Nested access.
	assert.Equal(t, "info-stack", ctx.StackInfo.Stack)
	assert.Equal(t, "prod", ctx.StackInfo.AuthContext.AWS.Profile)
}
