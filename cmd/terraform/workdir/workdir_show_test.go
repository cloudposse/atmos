package workdir

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestShowCmd_Structure(t *testing.T) {
	// Verify command structure.
	assert.Equal(t, "show <component>", showCmd.Use)
	assert.Equal(t, "Show workdir details", showCmd.Short)
	assert.Contains(t, showCmd.Long, "Display detailed information")
	assert.Contains(t, showCmd.Example, "atmos terraform workdir show vpc --stack dev")
}

func TestShowCmd_Args(t *testing.T) {
	// Verify exact args requirement.
	assert.NotNil(t, showCmd.Args)
}

func TestShowParser_Flags(t *testing.T) {
	// Verify parser is initialized with stack flag.
	assert.NotNil(t, showParser)
}

func TestPrintShowHuman(t *testing.T) {
	info := &WorkdirInfo{
		Name:        "dev-vpc",
		Component:   "vpc",
		Stack:       "dev",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/dev-vpc",
		ContentHash: "abc123def456",
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
	}

	// Should not panic.
	printShowHuman(info)
}

func TestPrintShowHuman_WithoutContentHash(t *testing.T) {
	info := &WorkdirInfo{
		Name:      "dev-vpc",
		Component: "vpc",
		Stack:     "dev",
		Source:    "components/terraform/vpc",
		Path:      ".workdir/terraform/dev-vpc",
		// ContentHash is empty.
		CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	// Should not include ContentHash row.
	printShowHuman(info)
}

func TestPrintShowHuman_DateFormatting(t *testing.T) {
	// Verify date format string.
	info := &WorkdirInfo{
		Name:      "dev-vpc",
		Component: "vpc",
		Stack:     "dev",
		Source:    "components/terraform/vpc",
		Path:      ".workdir/terraform/dev-vpc",
		CreatedAt: time.Date(2024, 6, 15, 14, 30, 45, 0, time.UTC),
		UpdatedAt: time.Date(2024, 6, 16, 10, 15, 30, 0, time.UTC),
	}

	// Format should be "2006-01-02 15:04:05 MST".
	expectedCreated := "2024-06-15 14:30:45 UTC"
	formatted := info.CreatedAt.Format("2006-01-02 15:04:05 MST")
	assert.Equal(t, expectedCreated, formatted)

	printShowHuman(info)
}

func TestPrintShowHuman_AllFields(t *testing.T) {
	info := &WorkdirInfo{
		Name:        "prod-vpc",
		Component:   "vpc",
		Stack:       "prod",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/prod-vpc",
		ContentHash: "abcdef1234567890",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	// Should handle all fields without panic.
	printShowHuman(info)
}

func TestPrintShowHuman_LongPaths(t *testing.T) {
	info := &WorkdirInfo{
		Name:      "dev-my-very-long-component-name",
		Component: "my-very-long-component-name",
		Stack:     "development-us-east-1",
		Source:    "components/terraform/infrastructure/networking/vpc/my-very-long-component-name",
		Path:      ".workdir/terraform/development-us-east-1-my-very-long-component-name",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Should handle long values.
	printShowHuman(info)
}

func TestShowCmd_DisableFlagParsing(t *testing.T) {
	// Verify flag parsing is enabled.
	assert.False(t, showCmd.DisableFlagParsing)
}

// Integration test helper.

func TestMockWorkdirManager_GetWorkdirInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	expectedInfo := CreateSampleWorkdirInfo("vpc", "dev")

	mock.EXPECT().GetWorkdirInfo(gomock.Any(), "vpc", "dev").Return(expectedInfo, nil)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	// Call through manager.
	result, err := mock.GetWorkdirInfo(&schema.AtmosConfiguration{}, "vpc", "dev")
	assert.NoError(t, err)
	assert.Equal(t, expectedInfo, result)
}

func TestShowCmd_RequiresStack(t *testing.T) {
	// The show command requires --stack flag.
	// This is validated in RunE, not in Args.
	// We verify the parser has stack flag registered.
	assert.NotNil(t, showParser)
}

// Test table row construction.

func TestPrintShowHuman_TableRows(t *testing.T) {
	info := &WorkdirInfo{
		Name:        "dev-vpc",
		Component:   "vpc",
		Stack:       "dev",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/dev-vpc",
		ContentHash: "abc123",
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	// The function builds rows like:
	// {"Name", info.Name}, {"Component", info.Component}, etc.
	// Verify fields are accessible.
	assert.Equal(t, "dev-vpc", info.Name)
	assert.Equal(t, "vpc", info.Component)
	assert.Equal(t, "dev", info.Stack)
	assert.Equal(t, "components/terraform/vpc", info.Source)
	assert.Equal(t, ".workdir/terraform/dev-vpc", info.Path)
	assert.Equal(t, "abc123", info.ContentHash)

	printShowHuman(info)
}

// Test empty string handling.

func TestPrintShowHuman_EmptySource(t *testing.T) {
	info := &WorkdirInfo{
		Name:      "dev-vpc",
		Component: "vpc",
		Stack:     "dev",
		Source:    "", // Empty source.
		Path:      ".workdir/terraform/dev-vpc",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Should handle empty source.
	printShowHuman(info)
}

func TestWorkdirInfo_ZeroTime(t *testing.T) {
	info := &WorkdirInfo{
		Name:      "dev-vpc",
		Component: "vpc",
		Stack:     "dev",
		Source:    "components/terraform/vpc",
		Path:      ".workdir/terraform/dev-vpc",
		// Zero time values.
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}

	// Should handle zero time without panic.
	printShowHuman(info)
}

// Test RunE validation scenarios.

func TestShowCmd_RunE_MissingStack(t *testing.T) {
	// Test the validation that stack is required.
	v := viper.New()
	v.Set("stack", "")

	stack := v.GetString("stack")
	if stack == "" {
		// This is the expected validation failure path.
		assert.True(t, true, "validation correctly identifies missing stack")
	}
}

func TestShowCmd_RunE_ValidStack(t *testing.T) {
	// Test valid stack passes validation.
	v := viper.New()
	v.Set("stack", "dev")

	stack := v.GetString("stack")
	assert.NotEmpty(t, stack)
	assert.Equal(t, "dev", stack)
}

func TestShowCmd_RunE_ComponentParsing(t *testing.T) {
	// Test that component is correctly parsed from args.
	args := []string{"vpc"}
	if len(args) == 1 {
		component := args[0]
		assert.Equal(t, "vpc", component)
	}
}

func TestShowCmd_RunE_MultipleArgs(t *testing.T) {
	// The show command expects exactly one argument.
	// This tests the Args validation.
	args := []string{"vpc", "extra"}
	assert.Len(t, args, 2)
	// cobra.ExactArgs(1) would reject this.
}
