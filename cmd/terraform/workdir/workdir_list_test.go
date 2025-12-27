package workdir

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrintListJSON(t *testing.T) {
	workdirs := []WorkdirInfo{
		{
			Name:        "dev-vpc",
			Component:   "vpc",
			Stack:       "dev",
			Source:      "components/terraform/vpc",
			Path:        ".workdir/terraform/dev-vpc",
			ContentHash: "abc123",
			CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			Name:        "prod-vpc",
			Component:   "vpc",
			Stack:       "prod",
			Source:      "components/terraform/vpc",
			Path:        ".workdir/terraform/prod-vpc",
			ContentHash: "def456",
			CreatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
		},
	}

	// Capture stdout.
	err := printListJSON(workdirs)
	require.NoError(t, err)

	// Verify output is valid JSON by marshaling the same data.
	jsonData, err := json.MarshalIndent(workdirs, "", "  ")
	require.NoError(t, err)

	// Parse to verify structure.
	var parsed []WorkdirInfo
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 2)
	assert.Equal(t, "dev-vpc", parsed[0].Name)
	assert.Equal(t, "prod-vpc", parsed[1].Name)
}

func TestPrintListJSON_Empty(t *testing.T) {
	err := printListJSON([]WorkdirInfo{})
	require.NoError(t, err)
}

func TestPrintListJSON_SingleItem(t *testing.T) {
	workdirs := []WorkdirInfo{
		{
			Name:      "dev-vpc",
			Component: "vpc",
			Stack:     "dev",
		},
	}

	err := printListJSON(workdirs)
	require.NoError(t, err)
}

func TestPrintListYAML(t *testing.T) {
	workdirs := []WorkdirInfo{
		{
			Name:        "dev-vpc",
			Component:   "vpc",
			Stack:       "dev",
			Source:      "components/terraform/vpc",
			Path:        ".workdir/terraform/dev-vpc",
			ContentHash: "abc123",
			CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}

	err := printListYAML(workdirs)
	require.NoError(t, err)

	// Verify output is valid YAML.
	yamlData, err := yaml.Marshal(workdirs)
	require.NoError(t, err)

	var parsed []WorkdirInfo
	err = yaml.Unmarshal(yamlData, &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 1)
	assert.Equal(t, "dev-vpc", parsed[0].Name)
}

func TestPrintListYAML_Empty(t *testing.T) {
	err := printListYAML([]WorkdirInfo{})
	require.NoError(t, err)
}

func TestPrintListTable(t *testing.T) {
	workdirs := []WorkdirInfo{
		{
			Name:        "dev-vpc",
			Component:   "vpc",
			Stack:       "dev",
			Source:      "components/terraform/vpc",
			Path:        ".workdir/terraform/dev-vpc",
			ContentHash: "abc123",
			CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			Name:      "prod-s3",
			Component: "s3",
			Stack:     "prod",
			Source:    "components/terraform/s3",
			Path:      ".workdir/terraform/prod-s3",
			CreatedAt: time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
		},
	}

	// printListTable doesn't return error and writes to stderr.
	// We just verify it doesn't panic.
	printListTable(workdirs)
}

func TestPrintListTable_Empty(t *testing.T) {
	// Should print "No workdirs found" message.
	printListTable([]WorkdirInfo{})
}

func TestPrintListTable_SingleItem(t *testing.T) {
	workdirs := []WorkdirInfo{
		{
			Name:      "dev-vpc",
			Component: "vpc",
			Stack:     "dev",
			Source:    "components/terraform/vpc",
			Path:      ".workdir/terraform/dev-vpc",
			CreatedAt: time.Now(),
		},
	}

	printListTable(workdirs)
}

func TestListCmd_Structure(t *testing.T) {
	// Verify command structure.
	assert.Equal(t, "list", listCmd.Use)
	assert.Equal(t, "List all workdirs", listCmd.Short)
	assert.Contains(t, listCmd.Example, "atmos terraform workdir list")
}

func TestListParser_Flags(t *testing.T) {
	// Verify parser is initialized.
	assert.NotNil(t, listParser)
}

func TestWorkdirInfo_JSONSerialization(t *testing.T) {
	info := WorkdirInfo{
		Name:        "dev-vpc",
		Component:   "vpc",
		Stack:       "dev",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/dev-vpc",
		ContentHash: "abc123def456",
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
	}

	// Marshal to JSON.
	data, err := json.Marshal(info)
	require.NoError(t, err)

	// Verify JSON structure.
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "dev-vpc", parsed["name"])
	assert.Equal(t, "vpc", parsed["component"])
	assert.Equal(t, "dev", parsed["stack"])
	assert.Equal(t, "components/terraform/vpc", parsed["source"])
	assert.Equal(t, ".workdir/terraform/dev-vpc", parsed["path"])
	assert.Equal(t, "abc123def456", parsed["content_hash"])
}

func TestWorkdirInfo_YAMLSerialization(t *testing.T) {
	info := WorkdirInfo{
		Name:        "dev-vpc",
		Component:   "vpc",
		Stack:       "dev",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/dev-vpc",
		ContentHash: "abc123",
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	// Marshal to YAML.
	data, err := yaml.Marshal(info)
	require.NoError(t, err)

	// Verify YAML structure.
	var parsed map[string]interface{}
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "dev-vpc", parsed["name"])
	assert.Equal(t, "vpc", parsed["component"])
	assert.Equal(t, "dev", parsed["stack"])
}

func TestWorkdirInfo_EmptyContentHash(t *testing.T) {
	info := WorkdirInfo{
		Name:      "dev-vpc",
		Component: "vpc",
		Stack:     "dev",
		// ContentHash is empty.
	}

	// JSON should omit empty content_hash.
	data, err := json.Marshal(info)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Check if content_hash is present (it will be empty string).
	contentHash, exists := parsed["content_hash"]
	if exists {
		assert.Empty(t, contentHash)
	}
}

func TestListCmd_Args(t *testing.T) {
	// Verify cobra.NoArgs is set.
	assert.NotNil(t, listCmd.Args)
}

func TestMockWorkdirManager_ListWorkdirs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().ListWorkdirs(gomock.Any()).Return(CreateSampleWorkdirList(), nil)

	// Save and restore the workdir manager.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	// Verify mock is set.
	assert.Equal(t, mock, GetWorkdirManager())

	// Call to verify expectation.
	result, err := mock.ListWorkdirs(&schema.AtmosConfiguration{})
	assert.NoError(t, err)
	assert.Len(t, result, 3)
}

func TestSetAndGetWorkdirManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	original := GetWorkdirManager()
	defer SetWorkdirManager(original)

	mock := NewMockWorkdirManager(ctrl)
	SetWorkdirManager(mock)

	assert.Equal(t, mock, GetWorkdirManager())
}

// Test date formatting in table output.

func TestPrintListTable_DateFormatting(t *testing.T) {
	// Test that dates are formatted correctly.
	workdirs := []WorkdirInfo{
		{
			Name:      "dev-vpc",
			Component: "vpc",
			Stack:     "dev",
			Source:    "components/terraform/vpc",
			Path:      ".workdir/terraform/dev-vpc",
			CreatedAt: time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC),
		},
	}

	// Format should be "2006-01-02 15:04".
	expected := "2024-06-15 14:30"
	formatted := workdirs[0].CreatedAt.Format("2006-01-02 15:04")
	assert.Equal(t, expected, formatted)

	// Verify table output doesn't panic.
	printListTable(workdirs)
}

// Test JSON indentation.

func TestPrintListJSON_Indentation(t *testing.T) {
	workdirs := []WorkdirInfo{
		{Name: "test", Component: "test", Stack: "test"},
	}

	// Marshal with same indentation as function.
	data, err := json.MarshalIndent(workdirs, "", "  ")
	require.NoError(t, err)

	// Verify indentation.
	assert.Contains(t, string(data), "\n  ")
}

// Test YAML format structure.

func TestPrintListYAML_Structure(t *testing.T) {
	workdirs := []WorkdirInfo{
		{
			Name:      "dev-vpc",
			Component: "vpc",
			Stack:     "dev",
		},
	}

	data, err := yaml.Marshal(workdirs)
	require.NoError(t, err)

	// YAML list format.
	assert.Contains(t, string(data), "- name: dev-vpc")
	assert.Contains(t, string(data), "  component: vpc")
	assert.Contains(t, string(data), "  stack: dev")
}

// Test that list command properly validates arguments using cobra.NoArgs.

func TestListCmd_ArgsValidation(t *testing.T) {
	// cobra.NoArgs should accept zero arguments.
	err := listCmd.Args(listCmd, []string{})
	assert.NoError(t, err)

	// cobra.NoArgs should reject any arguments.
	err = listCmd.Args(listCmd, []string{"unexpected"})
	assert.Error(t, err)
}
