package workdir

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdio "io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// testStreams is a simple streams implementation for testing.
type testStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// testIOContext holds buffers for capturing I/O output during tests.
type testIOContext struct {
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

// initTestIO initializes the I/O and UI contexts for testing.
// This must be called before tests that use printListJSON, printListYAML, or printListTable.
func initTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to initialize I/O context: %v", err)
	}
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
}

// initTestIOWithCapture initializes I/O contexts with buffers for capturing output.
func initTestIOWithCapture(t *testing.T) *testIOContext {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	streams := &testStreams{
		stdin:  &bytes.Buffer{},
		stdout: stdout,
		stderr: stderr,
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		t.Fatalf("failed to initialize I/O context: %v", err)
	}
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
	return &testIOContext{stdout: stdout, stderr: stderr}
}

func TestPrintListJSON(t *testing.T) {
	ioCtx := initTestIOWithCapture(t)
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

	// Call printListJSON and capture output.
	err := printListJSON(workdirs)
	require.NoError(t, err)

	// Verify actual output from printListJSON.
	actualOutput := ioCtx.stdout.String()
	require.NotEmpty(t, actualOutput, "printListJSON should produce output")

	// Parse actual output to verify it's valid JSON.
	var parsed []WorkdirInfo
	err = json.Unmarshal([]byte(actualOutput), &parsed)
	require.NoError(t, err, "printListJSON output should be valid JSON")
	assert.Len(t, parsed, 2)
	assert.Equal(t, "dev-vpc", parsed[0].Name)
	assert.Equal(t, "prod-vpc", parsed[1].Name)
}

func TestPrintListJSON_Empty(t *testing.T) {
	initTestIO(t)
	err := printListJSON([]WorkdirInfo{})
	require.NoError(t, err)
}

func TestPrintListJSON_SingleItem(t *testing.T) {
	initTestIO(t)
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
	ioCtx := initTestIOWithCapture(t)
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

	// Call printListYAML and capture output.
	err := printListYAML(workdirs)
	require.NoError(t, err)

	// Verify actual output from printListYAML.
	actualOutput := ioCtx.stdout.String()
	require.NotEmpty(t, actualOutput, "printListYAML should produce output")

	// Parse actual output to verify it's valid YAML.
	var parsed []WorkdirInfo
	err = yaml.Unmarshal([]byte(actualOutput), &parsed)
	require.NoError(t, err, "printListYAML output should be valid YAML")
	assert.Len(t, parsed, 1)
	assert.Equal(t, "dev-vpc", parsed[0].Name)
}

func TestPrintListYAML_Empty(t *testing.T) {
	initTestIO(t)
	err := printListYAML([]WorkdirInfo{})
	require.NoError(t, err)
}

func TestPrintListTable(t *testing.T) {
	initTestIO(t)
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
	initTestIO(t)
	// Should print "No workdirs found" message.
	printListTable([]WorkdirInfo{})
}

func TestPrintListTable_SingleItem(t *testing.T) {
	initTestIO(t)
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

	// JSON should omit empty content_hash due to omitempty tag.
	data, err := json.Marshal(info)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// content_hash should NOT be present due to omitempty.
	_, exists := parsed["content_hash"]
	assert.False(t, exists, "content_hash should be omitted when empty")
}

func TestListCmd_Args(t *testing.T) {
	// Verify cobra.NoArgs is set.
	assert.NotNil(t, listCmd.Args)
}

func TestListCmd_Flags(t *testing.T) {
	// Verify format flag is registered.
	flag := listCmd.Flags().Lookup("format")
	assert.NotNil(t, flag, "format flag should be registered")
	assert.Equal(t, "f", flag.Shorthand)
	assert.Equal(t, "table", flag.DefValue)
}

func TestListCmd_DisableFlagParsing(t *testing.T) {
	// Verify flag parsing is enabled.
	assert.False(t, listCmd.DisableFlagParsing)
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
	initTestIO(t)
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

// Test that list command properly validates arguments using cobra.NoArgs.

func TestListCmd_ArgsValidation(t *testing.T) {
	// cobra.NoArgs should accept zero arguments.
	err := listCmd.Args(listCmd, []string{})
	assert.NoError(t, err)

	// cobra.NoArgs should reject any arguments.
	err = listCmd.Args(listCmd, []string{"unexpected"})
	assert.Error(t, err)
}

// Test printListTable with varying data sizes.

func TestPrintListTable_MultipleRows(t *testing.T) {
	initTestIO(t)
	// Test with multiple workdirs of varying lengths.
	workdirs := []WorkdirInfo{
		{
			Name:      "short",
			Component: "a",
			Stack:     "b",
			Source:    "c",
			Path:      "d",
			CreatedAt: time.Now(),
		},
		{
			Name:      "medium-component-name",
			Component: "medium-component",
			Stack:     "staging",
			Source:    "components/terraform/medium",
			Path:      ".workdir/terraform/medium",
			CreatedAt: time.Now(),
		},
		{
			Name:      "very-long-component-name-with-namespace",
			Component: "very-long-component",
			Stack:     "production-us-east-1",
			Source:    "components/terraform/infrastructure/very-long-path/component",
			Path:      ".workdir/terraform/production-us-east-1-very-long-component",
			CreatedAt: time.Now(),
		},
	}

	// Should handle varying lengths without panic.
	printListTable(workdirs)
}

func TestPrintListTable_ZeroTime(t *testing.T) {
	initTestIO(t)
	workdirs := []WorkdirInfo{
		{
			Name:      "test",
			Component: "test",
			Stack:     "test",
			Source:    "test",
			Path:      "test",
			CreatedAt: time.Time{}, // Zero time.
		},
	}

	// Should handle zero time.
	printListTable(workdirs)
}

// Test JSON output edge cases.

func TestPrintListJSON_LargeList(t *testing.T) {
	initTestIO(t)
	workdirs := make([]WorkdirInfo, 100)
	for i := range workdirs {
		workdirs[i] = WorkdirInfo{
			Name:      fmt.Sprintf("workdir-%d", i),
			Component: fmt.Sprintf("component-%d", i),
			Stack:     "dev",
			Source:    "components/terraform/test",
			Path:      fmt.Sprintf(".workdir/terraform/dev-component-%d", i),
			CreatedAt: time.Now(),
		}
	}

	err := printListJSON(workdirs)
	require.NoError(t, err)
}

func TestPrintListJSON_SpecialCharacters(t *testing.T) {
	initTestIO(t)
	workdirs := []WorkdirInfo{
		{
			Name:      "test-with-special",
			Component: "test/component",
			Stack:     "dev:test",
			Source:    "path/with spaces/component",
			Path:      ".workdir/special\"chars",
		},
	}

	err := printListJSON(workdirs)
	require.NoError(t, err)
}

// Test YAML output edge cases.

func TestPrintListYAML_LargeList(t *testing.T) {
	initTestIO(t)
	workdirs := make([]WorkdirInfo, 50)
	for i := range workdirs {
		workdirs[i] = WorkdirInfo{
			Name:      fmt.Sprintf("workdir-%d", i),
			Component: fmt.Sprintf("component-%d", i),
			Stack:     "prod",
		}
	}

	err := printListYAML(workdirs)
	require.NoError(t, err)
}

func TestPrintListYAML_SpecialCharacters(t *testing.T) {
	initTestIO(t)
	workdirs := []WorkdirInfo{
		{
			Name:      "test-yaml",
			Component: "test: component",
			Stack:     "dev",
			Source:    "path with\nnewline",
		},
	}

	err := printListYAML(workdirs)
	require.NoError(t, err)
}
