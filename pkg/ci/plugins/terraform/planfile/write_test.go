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

func TestWritePlanfileResultsRestoresThePlanRecordBesideThePlan(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "atmos-key.planfile")
	results := []FileResult{
		{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("plan data"))},
		{Name: BindingFilename, Data: io.NopCloser(strings.NewReader(`{"Version":3}`))},
	}
	require.NoError(t, WritePlanfileResults(results, planPath))

	record, err := os.ReadFile(BindingPath(planPath))
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":3}`, string(record))
	assert.Equal(t, planPath+".atmos.json", BindingPath(planPath))
}

func TestWritePlanfileResultsForVerificationDoesNotRestoreARecord(t *testing.T) {
	tmpDir := t.TempDir()
	stored := filepath.Join(tmpDir, StoredPlanPrefix+PlanFilename)
	canonical := filepath.Join(tmpDir, "atmos-key.planfile")
	results := []FileResult{
		{Name: PlanFilename, Data: io.NopCloser(strings.NewReader("plan data"))},
		{Name: BindingFilename, Data: io.NopCloser(strings.NewReader("{}"))},
	}
	require.NoError(t, WritePlanfileResultsForVerification(results, stored, canonical))

	assert.FileExists(t, stored)
	assert.NoFileExists(t, BindingPath(stored), "a stored plan is only compared against; the fresh plan gets its own record")
	assert.NoFileExists(t, BindingPath(canonical))
}

func TestOpenBindingFindsOnlyAnExistingRecord(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "atmos-key.planfile")
	_, closer, found := OpenBinding(planPath)
	assert.False(t, found, "components without a record upload nothing under the record name")
	assert.Nil(t, closer)

	require.NoError(t, os.WriteFile(BindingPath(planPath), []byte("record"), 0o600))
	entry, closer, found := OpenBinding(planPath)
	require.True(t, found)
	defer closer.Close()
	assert.Equal(t, BindingFilename, entry.Name)
	data, err := io.ReadAll(entry.Data)
	require.NoError(t, err)
	assert.Equal(t, "record", string(data))
}
