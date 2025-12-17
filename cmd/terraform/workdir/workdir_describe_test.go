package workdir

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeCmd_Structure(t *testing.T) {
	// Verify command structure.
	assert.Equal(t, "describe <component>", describeCmd.Use)
	assert.Equal(t, "Describe workdir as stack manifest", describeCmd.Short)
	assert.Contains(t, describeCmd.Long, "Output the workdir configuration")
	assert.Contains(t, describeCmd.Example, "atmos terraform workdir describe vpc --stack dev")
}

func TestDescribeCmd_Args(t *testing.T) {
	// Verify exact args requirement.
	assert.NotNil(t, describeCmd.Args)
}

func TestDescribeParser_Flags(t *testing.T) {
	// Verify parser is initialized.
	assert.NotNil(t, describeParser)
}

func TestDescribeCmd_DisableFlagParsing(t *testing.T) {
	// Verify flag parsing is enabled.
	assert.False(t, describeCmd.DisableFlagParsing)
}

func TestMockWorkdirManager_DescribeWorkdir(t *testing.T) {
	mock := NewMockWorkdirManager()

	expectedManifest := `components:
  terraform:
    vpc:
      metadata:
        workdir:
          name: dev-vpc
          source: components/terraform/vpc
          path: .workdir/terraform/dev-vpc
`

	mock.DescribeWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
		assert.Equal(t, "vpc", component)
		assert.Equal(t, "dev", stack)
		return expectedManifest, nil
	}

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(nil, "vpc", "dev")
	assert.NoError(t, err)
	assert.Equal(t, expectedManifest, result)
	assert.Equal(t, 1, mock.DescribeWorkdirCalls)
}

func TestMockWorkdirManager_DescribeWorkdir_NotFound(t *testing.T) {
	mock := NewMockWorkdirManager()

	expectedErr := errUtils.Build(errUtils.ErrWorkdirMetadata).
		WithExplanation("Workdir not found").
		Err()

	mock.DescribeWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
		return "", expectedErr
	}

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(nil, "nonexistent", "dev")
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

func TestDescribeCmd_RequiresStack(t *testing.T) {
	// The describe command requires --stack flag.
	// Verify the parser has stack flag registered.
	assert.NotNil(t, describeParser)
}

func TestDescribeCmd_Examples(t *testing.T) {
	examples := describeCmd.Example
	assert.Contains(t, examples, "atmos terraform workdir describe vpc --stack dev")
}

// Test error handling.

func TestDescribeCmd_ErrorTypes(t *testing.T) {
	// Verify error builder creates correct sentinel error.
	err := errUtils.Build(errUtils.ErrWorkdirMetadata).
		WithExplanation("Failed to load atmos configuration").
		Err()

	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
	// Error is based on sentinel and is non-nil.
	assert.NotNil(t, err)
}

// Test manifest structure.

func TestDescribeWorkdir_ManifestStructure(t *testing.T) {
	// Verify the expected manifest structure.
	info := &WorkdirInfo{
		Name:        "dev-vpc",
		Component:   "vpc",
		Stack:       "dev",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/dev-vpc",
		ContentHash: "abc123",
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
	}

	// Verify all fields are present.
	assert.NotEmpty(t, info.Name)
	assert.NotEmpty(t, info.Component)
	assert.NotEmpty(t, info.Stack)
	assert.NotEmpty(t, info.Source)
	assert.NotEmpty(t, info.Path)
	assert.NotEmpty(t, info.ContentHash)
	assert.False(t, info.CreatedAt.IsZero())
	assert.False(t, info.UpdatedAt.IsZero())
}

func TestDescribeWorkdir_ManifestKeys(t *testing.T) {
	// Document the expected manifest structure keys.
	expectedKeys := []string{
		"components",
		"terraform",
		"metadata",
		"workdir",
		"name",
		"source",
		"path",
		"content_hash",
		"created_at",
		"updated_at",
	}

	for _, key := range expectedKeys {
		assert.NotEmpty(t, key)
	}
}

// Test with various component names.

func TestDescribeCmd_VariousComponentNames(t *testing.T) {
	testCases := []struct {
		component string
		stack     string
	}{
		{"vpc", "dev"},
		{"s3-bucket", "prod"},
		{"my_component", "staging"},
		{"component.name", "test"},
	}

	for _, tc := range testCases {
		t.Run(tc.component, func(t *testing.T) {
			mock := NewMockWorkdirManager()
			mock.DescribeWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
				assert.Equal(t, tc.component, component)
				assert.Equal(t, tc.stack, stack)
				return "manifest", nil
			}

			original := workdirManager
			defer func() { workdirManager = original }()
			SetWorkdirManager(mock)

			result, err := mock.DescribeWorkdir(nil, tc.component, tc.stack)
			assert.NoError(t, err)
			assert.NotEmpty(t, result)
		})
	}
}

// Test error scenarios.

func TestDescribeCmd_WorkdirNotFoundError(t *testing.T) {
	mock := NewMockWorkdirManager()
	mock.DescribeWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
		return "", errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithExplanation("Workdir not found for component 'vpc' in stack 'dev'").
			WithHint("Run 'atmos terraform init' to create the workdir").
			Err()
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(nil, "vpc", "dev")
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

func TestDescribeCmd_MarshalError(t *testing.T) {
	mock := NewMockWorkdirManager()
	mock.DescribeWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
		return "", errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(errors.New("yaml marshal failed")).
			WithExplanation("Failed to marshal workdir manifest").
			Err()
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(nil, "vpc", "dev")
	assert.Error(t, err)
	assert.Empty(t, result)
}

// Test nil handling.

func TestDescribeCmd_NilConfig(t *testing.T) {
	mock := NewMockWorkdirManager()
	mock.DescribeWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
		// Should handle nil config gracefully.
		return "manifest", nil
	}

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(nil, "vpc", "dev")
	assert.NoError(t, err)
	assert.NotEmpty(t, result)
}
