package planfile

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePlanfileResults(t *testing.T) {
	t.Run("writes plan and lock files to correct paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		planPath := filepath.Join(tmpDir, PlanFilename)

		results := []FileResult{
			{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("plan data"))},
			{Name: LockFilename, Data: io.NopCloser(strings.NewReader("lock data"))},
		}

		err := WritePlanfileResults(results, planPath)
		require.NoError(t, err)

		planData, err := os.ReadFile(planPath)
		require.NoError(t, err)
		assert.Equal(t, "plan data", string(planData))

		lockData, err := os.ReadFile(filepath.Join(tmpDir, LockFilename))
		require.NoError(t, err)
		assert.Equal(t, "lock data", string(lockData))
	})

	t.Run("skips unknown filenames", func(t *testing.T) {
		tmpDir := t.TempDir()
		planPath := filepath.Join(tmpDir, PlanFilename)

		results := []FileResult{
			{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("plan data"))},
			{Name: "unknown.txt", Data: io.NopCloser(strings.NewReader("should be skipped"))},
		}

		err := WritePlanfileResults(results, planPath)
		require.NoError(t, err)

		// Plan file should exist.
		_, err = os.Stat(planPath)
		require.NoError(t, err)

		// Unknown file should not exist.
		_, err = os.Stat(filepath.Join(tmpDir, "unknown.txt"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		planPath := filepath.Join(tmpDir, "nested", "dir", PlanFilename)

		results := []FileResult{
			{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("plan data"))},
			{Name: LockFilename, Data: io.NopCloser(strings.NewReader("lock data"))},
		}

		err := WritePlanfileResults(results, planPath)
		require.NoError(t, err)

		planData, err := os.ReadFile(planPath)
		require.NoError(t, err)
		assert.Equal(t, "plan data", string(planData))

		lockData, err := os.ReadFile(filepath.Join(tmpDir, "nested", "dir", LockFilename))
		require.NoError(t, err)
		assert.Equal(t, "lock data", string(lockData))
	})

	t.Run("handles empty results", func(t *testing.T) {
		tmpDir := t.TempDir()
		planPath := filepath.Join(tmpDir, PlanFilename)

		err := WritePlanfileResults(nil, planPath)
		require.NoError(t, err)
	})
}
