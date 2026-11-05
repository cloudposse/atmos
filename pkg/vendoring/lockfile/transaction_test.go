package lockfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func transactionRecord(t *testing.T, config *schema.AtmosConfiguration, name string) (*PreparedRecord, string) {
	t.Helper()
	source := filepath.Join(config.BasePath, "source-"+name)
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte(name), 0o644))
	target := filepath.Join(config.BasePath, "target-"+name)
	record, err := PrepareRecord(context.Background(), config, RecordTarget{Name: name, Kind: "local", TempDir: source, Path: target, DeclaredSource: source}, RecordOptions{})
	require.NoError(t, err)
	return record, target
}

func TestPreparedRecordMaterializeTransaction(t *testing.T) {
	for _, mode := range []string{"success", "copy failure", "cancel during copy", "corrupt receipt"} {
		t.Run(mode, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			record, target := transactionRecord(t, config, "example")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			copyErr := errors.New("copy failed")
			if mode == "corrupt receipt" {
				require.NoError(t, os.WriteFile(Path(config), []byte("artifacts: [invalid"), 0o644))
			}
			copied := false
			err := record.Materialize(ctx, config, func() error {
				copied = true
				if mode == "copy failure" {
					return copyErr
				}
				require.NoError(t, os.MkdirAll(target, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("example"), 0o644))
				if mode == "cancel during copy" {
					cancel()
				}
				return nil
			})
			switch mode {
			case "copy failure":
				require.ErrorIs(t, err, copyErr)
				require.NoFileExists(t, Path(config))
			case "corrupt receipt":
				require.Error(t, err)
				assert.False(t, copied)
				require.NoDirExists(t, target)
			default:
				require.NoError(t, err)
				lock, err := Load(config)
				require.NoError(t, err)
				require.Len(t, lock.Artifacts, 1)
				check, err := IsMaterialized(config, MaterializationParams{ID: record.id, Declared: record.artifact.Source.Declared, Target: target})
				require.NoError(t, err)
				assert.True(t, check.Materialized, check.Reason)
			}
		})
	}
}

func TestMutationCancellationAndInvalidPathsLeaveTargetsUntouched(t *testing.T) {
	t.Run("canceled before lock creation", func(t *testing.T) {
		config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := WithMutation(ctx, config, func() error { t.Fatal("canceled transaction invoked callback"); return nil })
		require.ErrorIs(t, err, context.Canceled)
		require.NoDirExists(t, filepath.Join(config.BasePath, ".atmos"))
	})
	for _, blocker := range []string{"project lock directory", "receipt parent"} {
		t.Run(blocker, func(t *testing.T) {
			base := t.TempDir()
			config := &schema.AtmosConfiguration{BasePath: base}
			if blocker == "project lock directory" {
				require.NoError(t, os.WriteFile(filepath.Join(base, ".atmos"), []byte("file"), 0o644))
			} else {
				config.Vendor.LockFile = filepath.Join(base, "blocked", "vendor.lock.yaml")
				require.NoError(t, os.WriteFile(filepath.Join(base, "blocked"), []byte("file"), 0o644))
			}
			err := WithMutation(context.Background(), config, func() error { t.Fatal("invalid lock location invoked callback"); return nil })
			require.Error(t, err)
		})
	}
}

func TestCanonicalMutationPathResolvesSymlinkPrefix(t *testing.T) {
	real := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(real, alias))
	actual, err := canonicalPath(filepath.Join(alias, "not-yet-created", "vendor.lock.yaml"))
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(expected, "not-yet-created", "vendor.lock.yaml"), actual)
}

// The test binary supplies a portable separate process; no shell or platform binaries are used.
func TestMutationProcessHelper(t *testing.T) {
	if os.Getenv("ATMOS_TEST_MUTATION_HELPER") != "1" {
		return
	}
	base := os.Getenv("ATMOS_TEST_MUTATION_BASE")
	config := &schema.AtmosConfiguration{BasePath: base}
	if os.Getenv("ATMOS_TEST_MUTATION_MODE") == "clean" {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		_, err := CleanSelectedContext(ctx, config, nil, CleanOptions{Force: true})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		return
	}
	name := os.Getenv("ATMOS_TEST_MUTATION_NAME")
	record, target := transactionRecord(t, config, name)
	require.NoError(t, record.Materialize(context.Background(), config, func() error {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(target, "main.tf"), []byte(name), 0o644)
	}))
}

func mutationProcess(t *testing.T, base, name, mode string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestMutationProcessHelper$", "-test.timeout=20s")
	command.Env = append(os.Environ(), "ATMOS_TEST_MUTATION_HELPER=1", "ATMOS_TEST_MUTATION_BASE="+base, "ATMOS_TEST_MUTATION_NAME="+name, "ATMOS_TEST_MUTATION_MODE="+mode)
	return command
}

func TestSeparateProcessMaterializationKeepsEveryReceipt(t *testing.T) {
	base := t.TempDir()
	var workers sync.WaitGroup
	failures := make(chan string, 4)
	for i := range 4 {
		command := mutationProcess(t, base, fmt.Sprintf("component-%d", i), "record")
		workers.Add(1)
		go func() {
			defer workers.Done()
			output, err := command.CombinedOutput()
			if err != nil {
				failures <- fmt.Sprintf("%v: %s", err, output)
			}
		}()
	}
	workers.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	config := &schema.AtmosConfiguration{BasePath: base}
	lock, err := Load(config)
	require.NoError(t, err)
	require.Len(t, lock.Artifacts, 4, "serialized read-modify-write must retain every process's receipt")
	report, err := Verify(config, lock)
	require.NoError(t, err)
	assert.Empty(t, report)
}

func TestSeparateProcessCleanWaitsForMaterialization(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	record, target := transactionRecord(t, config, "component")
	require.NoError(t, record.Materialize(context.Background(), config, func() error {
		require.NoError(t, os.MkdirAll(target, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("component"), 0o644))
		output, err := mutationProcess(t, config.BasePath, "", "clean").CombinedOutput()
		require.NoError(t, err, string(output))
		require.FileExists(t, filepath.Join(target, "main.tf"))
		return nil
	}))
	report, err := Clean(config, "", true, false)
	require.NoError(t, err)
	assert.NotEmpty(t, report.Removed)
	require.NoFileExists(t, filepath.Join(target, "main.tf"))
	lock, err := Load(config)
	require.NoError(t, err)
	require.Len(t, lock.Artifacts, 1, "clean must preserve the completed materialization receipt")
	for _, artifact := range lock.Artifacts {
		assert.Equal(t, "component", artifact.Name)
		require.Len(t, artifact.Files, 1)
		assert.Equal(t, "main.tf", artifact.Files[0].Path)
	}
}

func TestCanonicalPathRejectsSymlinkLoop(t *testing.T) {
	base := t.TempDir()
	first, second := filepath.Join(base, "first"), filepath.Join(base, "second")
	require.NoError(t, os.Symlink(second, first))
	require.NoError(t, os.Symlink(first, second))
	_, err := canonicalPath(filepath.Join(first, "vendor.lock.yaml"))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "links") || strings.Contains(err.Error(), "symlink"), err)
}
