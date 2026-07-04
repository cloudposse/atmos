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

func TestWritePlanfileResultsForVerification(t *testing.T) {
	t.Run("writes plan to stored path and lock to canonical dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		storedPlanPath := filepath.Join(tmpDir, StoredPlanPrefix+PlanFilename)
		canonicalPlanPath := filepath.Join(tmpDir, PlanFilename)

		results := []FileResult{
			{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("stored plan data"))},
			{Name: LockFilename, Data: io.NopCloser(strings.NewReader("lock data"))},
		}

		err := WritePlanfileResultsForVerification(results, storedPlanPath, canonicalPlanPath)
		require.NoError(t, err)

		// Plan file should be written to stored path.
		planData, err := os.ReadFile(storedPlanPath)
		require.NoError(t, err)
		assert.Equal(t, "stored plan data", string(planData))

		// Lock file should be written to canonical dir (not stored path dir).
		lockData, err := os.ReadFile(filepath.Join(tmpDir, LockFilename))
		require.NoError(t, err)
		assert.Equal(t, "lock data", string(lockData))

		// Canonical plan path should NOT exist (it's reserved for fresh plan).
		_, err = os.Stat(canonicalPlanPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("writes plan to stored path in different dir from canonical", func(t *testing.T) {
		tmpDir := t.TempDir()
		storedDir := filepath.Join(tmpDir, "component")
		canonicalDir := filepath.Join(tmpDir, "component")

		storedPlanPath := filepath.Join(storedDir, StoredPlanPrefix+PlanFilename)
		canonicalPlanPath := filepath.Join(canonicalDir, PlanFilename)

		results := []FileResult{
			{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("plan data"))},
			{Name: LockFilename, Data: io.NopCloser(strings.NewReader("lock data"))},
		}

		err := WritePlanfileResultsForVerification(results, storedPlanPath, canonicalPlanPath)
		require.NoError(t, err)

		// Plan written to stored path.
		planData, err := os.ReadFile(storedPlanPath)
		require.NoError(t, err)
		assert.Equal(t, "plan data", string(planData))

		// Lock written to canonical dir.
		lockData, err := os.ReadFile(filepath.Join(canonicalDir, LockFilename))
		require.NoError(t, err)
		assert.Equal(t, "lock data", string(lockData))
	})

	t.Run("skips unknown filenames", func(t *testing.T) {
		tmpDir := t.TempDir()
		storedPlanPath := filepath.Join(tmpDir, StoredPlanPrefix+PlanFilename)
		canonicalPlanPath := filepath.Join(tmpDir, PlanFilename)

		results := []FileResult{
			{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("plan data"))},
			{Name: "unknown.txt", Data: io.NopCloser(strings.NewReader("should be skipped"))},
		}

		err := WritePlanfileResultsForVerification(results, storedPlanPath, canonicalPlanPath)
		require.NoError(t, err)

		_, err = os.Stat(storedPlanPath)
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(tmpDir, "unknown.txt"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("handles empty results", func(t *testing.T) {
		tmpDir := t.TempDir()
		storedPlanPath := filepath.Join(tmpDir, StoredPlanPrefix+PlanFilename)
		canonicalPlanPath := filepath.Join(tmpDir, PlanFilename)

		err := WritePlanfileResultsForVerification(nil, storedPlanPath, canonicalPlanPath)
		require.NoError(t, err)
	})
}
