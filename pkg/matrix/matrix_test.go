package matrix

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestMarshal(t *testing.T) {
	t.Run("empty entries", func(t *testing.T) {
		result, err := Marshal([]Entry{})
		require.NoError(t, err)
		assert.JSONEq(t, `{"include":[]}`, string(result))
	})

	t.Run("single entry", func(t *testing.T) {
		entries := []Entry{
			{
				Stack:         "ue1-dev",
				Component:     "vpc",
				ComponentPath: filepath.Join("components", "terraform", "vpc"),
				ComponentType: "terraform",
			},
		}
		result, err := Marshal(entries)
		require.NoError(t, err)

		var output Output
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)
		require.Len(t, output.Include, 1)
		assert.Equal(t, "ue1-dev", output.Include[0].Stack)
		assert.Equal(t, "vpc", output.Include[0].Component)
		assert.Equal(t, filepath.Join("components", "terraform", "vpc"), output.Include[0].ComponentPath)
		assert.Equal(t, "terraform", output.Include[0].ComponentType)
	})

	t.Run("multiple entries", func(t *testing.T) {
		entries := []Entry{
			{Stack: "ue1-dev", Component: "vpc", ComponentPath: "components/terraform/vpc", ComponentType: "terraform"},
			{Stack: "ue1-staging", Component: "eks", ComponentPath: "components/terraform/eks", ComponentType: "terraform"},
		}
		result, err := Marshal(entries)
		require.NoError(t, err)

		var output Output
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)
		require.Len(t, output.Include, 2)
		assert.Equal(t, "ue1-dev", output.Include[0].Stack)
		assert.Equal(t, "eks", output.Include[1].Component)
	})
}

func TestWriteOutput_File(t *testing.T) {
	t.Run("writes matrix and count to file", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "github_output")
		entries := []Entry{
			{
				Stack:         "ue1-dev",
				Component:     "vpc",
				ComponentPath: "components/terraform/vpc",
				ComponentType: "terraform",
			},
		}
		err := WriteOutput(entries, outputFile)
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		require.Len(t, lines, 2)
		assert.True(t, strings.HasPrefix(lines[0], "matrix="))
		assert.Equal(t, "affected_count=1", lines[1])

		// Verify JSON is valid.
		matrixJSON := strings.TrimPrefix(lines[0], "matrix=")
		var output Output
		err = json.Unmarshal([]byte(matrixJSON), &output)
		require.NoError(t, err)
		require.Len(t, output.Include, 1)
		assert.Equal(t, "vpc", output.Include[0].Component)
	})

	t.Run("empty entries writes empty include", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "github_output")
		err := WriteOutput([]Entry{}, outputFile)
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), `"include":[]`)
		assert.Contains(t, string(content), "affected_count=0")
	})

	t.Run("file open error", func(t *testing.T) {
		err := WriteOutput([]Entry{}, filepath.Join(t.TempDir(), "nonexistent", "file"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open output file")
	})
}

func TestWriteOutput_Stdout(t *testing.T) {
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	err = WriteOutput([]Entry{
		{
			Stack:         "ue1-dev",
			Component:     "vpc",
			ComponentPath: "components/terraform/vpc",
			ComponentType: "terraform",
		},
	}, "")
	assert.NoError(t, err)
}

func TestMarshal_NilEntries(t *testing.T) {
	result, err := Marshal(nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"include":null}`, string(result))
}
