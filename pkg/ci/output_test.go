package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopOutputWriter_WriteOutput(t *testing.T) {
	writer := &NoopOutputWriter{}
	err := writer.WriteOutput("key", "value")
	assert.NoError(t, err)
}

func TestNoopOutputWriter_WriteSummary(t *testing.T) {
	writer := &NoopOutputWriter{}
	err := writer.WriteSummary("summary content")
	assert.NoError(t, err)
}

func TestNewFileOutputWriter(t *testing.T) {
	writer := NewFileOutputWriter("/tmp/output", "/tmp/summary")
	assert.Equal(t, "/tmp/output", writer.OutputPath)
	assert.Equal(t, "/tmp/summary", writer.SummaryPath)
}

func TestFileOutputWriter_WriteOutput_EmptyPath(t *testing.T) {
	writer := &FileOutputWriter{OutputPath: ""}
	err := writer.WriteOutput("key", "value")
	assert.NoError(t, err)
}

func TestFileOutputWriter_WriteOutput_SingleLine(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	writer := &FileOutputWriter{OutputPath: outputPath}
	err := writer.WriteOutput("mykey", "myvalue")
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "mykey=myvalue\n", string(content))
}

func TestFileOutputWriter_WriteOutput_Multiline(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	writer := &FileOutputWriter{OutputPath: outputPath}
	err := writer.WriteOutput("mykey", "line1\nline2\nline3")
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	// Should use heredoc format.
	assert.Contains(t, string(content), "mykey<<EOF")
	assert.Contains(t, string(content), "line1\nline2\nline3")
	assert.Contains(t, string(content), "EOF\n")
}

func TestFileOutputWriter_WriteOutput_MultilineWithEOFInContent(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	writer := &FileOutputWriter{OutputPath: outputPath}
	// Content contains "EOF", so delimiter should be changed.
	err := writer.WriteOutput("mykey", "line1\nEOF\nline2")
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	// Should use modified delimiter (EOF_).
	assert.Contains(t, string(content), "mykey<<EOF_")
	assert.Contains(t, string(content), "line1\nEOF\nline2")
	assert.Contains(t, string(content), "EOF_\n")
}

func TestFileOutputWriter_WriteOutput_MultipleWrites(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	writer := &FileOutputWriter{OutputPath: outputPath}

	err := writer.WriteOutput("key1", "value1")
	require.NoError(t, err)

	err = writer.WriteOutput("key2", "value2")
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "key1=value1")
	assert.Contains(t, string(content), "key2=value2")
}

func TestFileOutputWriter_WriteSummary_EmptyPath(t *testing.T) {
	writer := &FileOutputWriter{SummaryPath: ""}
	err := writer.WriteSummary("summary content")
	assert.NoError(t, err)
}

func TestFileOutputWriter_WriteSummary(t *testing.T) {
	tmpDir := t.TempDir()
	summaryPath := filepath.Join(tmpDir, "summary.md")

	writer := &FileOutputWriter{SummaryPath: summaryPath}
	err := writer.WriteSummary("# Summary\n\nThis is a test.")
	require.NoError(t, err)

	content, err := os.ReadFile(summaryPath)
	require.NoError(t, err)
	assert.Equal(t, "# Summary\n\nThis is a test.", string(content))
}

func TestFileOutputWriter_WriteSummary_Append(t *testing.T) {
	tmpDir := t.TempDir()
	summaryPath := filepath.Join(tmpDir, "summary.md")

	writer := &FileOutputWriter{SummaryPath: summaryPath}

	err := writer.WriteSummary("Part 1\n")
	require.NoError(t, err)

	err = writer.WriteSummary("Part 2\n")
	require.NoError(t, err)

	content, err := os.ReadFile(summaryPath)
	require.NoError(t, err)
	assert.Equal(t, "Part 1\nPart 2\n", string(content))
}

func TestNewOutputHelpers(t *testing.T) {
	writer := &NoopOutputWriter{}
	helpers := NewOutputHelpers(writer)
	assert.NotNil(t, helpers)
	assert.Equal(t, writer, helpers.Writer)
}

func TestOutputHelpers_WritePlanOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")
	writer := &FileOutputWriter{OutputPath: outputPath}
	helpers := NewOutputHelpers(writer)

	opts := PlanOutputOptions{
		HasChanges:        true,
		HasAdditions:      true,
		AdditionsCount:    5,
		ChangesCount:      3,
		HasDestructions:   true,
		DestructionsCount: 2,
		ExitCode:          2,
		ArtifactKey:       "stack/component/sha.tfplan",
	}

	err := helpers.WritePlanOutputs(opts)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "has_changes=true")
	assert.Contains(t, contentStr, "has_additions=true")
	assert.Contains(t, contentStr, "has_additions_count=5")
	assert.Contains(t, contentStr, "has_changes_count=3")
	assert.Contains(t, contentStr, "has_destructions=true")
	assert.Contains(t, contentStr, "has_destructions_count=2")
	assert.Contains(t, contentStr, "plan_exit_code=2")
	assert.Contains(t, contentStr, "artifact_key=stack/component/sha.tfplan")
}

func TestOutputHelpers_WritePlanOutputs_NoArtifactKey(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")
	writer := &FileOutputWriter{OutputPath: outputPath}
	helpers := NewOutputHelpers(writer)

	opts := PlanOutputOptions{
		HasChanges:   false,
		HasAdditions: false,
		ExitCode:     0,
		// No ArtifactKey.
	}

	err := helpers.WritePlanOutputs(opts)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "has_changes=false")
	assert.NotContains(t, contentStr, "artifact_key")
}

func TestOutputHelpers_WriteApplyOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")
	writer := &FileOutputWriter{OutputPath: outputPath}
	helpers := NewOutputHelpers(writer)

	opts := ApplyOutputOptions{
		ExitCode: 0,
		Success:  true,
		Outputs: map[string]string{
			"vpc_id":    "vpc-12345",
			"subnet_id": "subnet-67890",
		},
	}

	err := helpers.WriteApplyOutputs(opts)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "apply_exit_code=0")
	assert.Contains(t, contentStr, "success=true")
	assert.Contains(t, contentStr, "output_vpc_id=vpc-12345")
	assert.Contains(t, contentStr, "output_subnet_id=subnet-67890")
}

func TestOutputHelpers_WriteApplyOutputs_NoOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")
	writer := &FileOutputWriter{OutputPath: outputPath}
	helpers := NewOutputHelpers(writer)

	opts := ApplyOutputOptions{
		ExitCode: 1,
		Success:  false,
		Outputs:  nil,
	}

	err := helpers.WriteApplyOutputs(opts)
	require.NoError(t, err)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "apply_exit_code=1")
	assert.Contains(t, contentStr, "success=false")
	assert.NotContains(t, contentStr, "output_")
}

func TestPlanOutputOptions_Struct(t *testing.T) {
	opts := PlanOutputOptions{
		HasChanges:        true,
		HasAdditions:      true,
		AdditionsCount:    10,
		ChangesCount:      5,
		HasDestructions:   false,
		DestructionsCount: 0,
		ExitCode:          2,
		ArtifactKey:       "key.tfplan",
	}

	assert.True(t, opts.HasChanges)
	assert.True(t, opts.HasAdditions)
	assert.Equal(t, 10, opts.AdditionsCount)
	assert.Equal(t, 5, opts.ChangesCount)
	assert.False(t, opts.HasDestructions)
	assert.Equal(t, 0, opts.DestructionsCount)
	assert.Equal(t, 2, opts.ExitCode)
	assert.Equal(t, "key.tfplan", opts.ArtifactKey)
}

func TestApplyOutputOptions_Struct(t *testing.T) {
	opts := ApplyOutputOptions{
		ExitCode: 0,
		Success:  true,
		Outputs: map[string]string{
			"key": "value",
		},
	}

	assert.Equal(t, 0, opts.ExitCode)
	assert.True(t, opts.Success)
	assert.Equal(t, "value", opts.Outputs["key"])
}
