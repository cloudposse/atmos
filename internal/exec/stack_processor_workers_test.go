package exec

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessYAMLConfigFilesWaitsForWorkersAfterError(t *testing.T) {
	directory := t.TempDir()
	paths := []string{filepath.Join(directory, "slow.yaml"), filepath.Join(directory, "bad.yaml")}
	for _, path := range paths {
		require.NoError(t, os.WriteFile(path, []byte("vars: {stage: dev}\n"), 0o600))
	}
	ready, release, expected := blockStackWorkerRead(t)
	returned := make(chan struct{})
	var result error
	go func() {
		defer close(returned)
		_, _, _, _, result = ProcessYAMLConfigFiles(&schema.AtmosConfiguration{}, directory, "", "", "", "", paths, false, false, false)
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-returned:
		case <-time.After(5 * time.Second):
			t.Error("stack loading did not finish after releasing its worker")
		}
	})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("both stack workers did not reach the read barrier")
	}
	select {
	case <-returned:
		t.Error("stack loading returned while another worker was still reading")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case <-returned:
		require.ErrorIs(t, result, expected)
	case <-time.After(5 * time.Second):
		t.Fatal("stack loading did not return its original error after all workers finished")
	}
}

// blockStackWorkerRead holds one worker while another reports an admission error.
func blockStackWorkerRead(t *testing.T) (<-chan struct{}, func(), error) {
	t.Helper()
	t.Cleanup(guard.SaveHooks())
	started, ready, released := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var startOnce, readyOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	t.Cleanup(release)
	expected := errors.New("stack source rejected")
	guard.SetEnabled(func(kind string) (bool, error) { return kind == "source", nil })
	guard.SetEvaluator(func(_ context.Context, _ string, raw json.RawMessage) error {
		var input guard.SourceInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return err
		}
		switch filepath.Base(input.Path) {
		case "slow.yaml":
			startOnce.Do(func() { close(started) })
			<-released
		case "bad.yaml":
			<-started
			readyOnce.Do(func() { close(ready) })
			return expected
		}
		return nil
	})
	return ready, release, expected
}

func TestProcessYAMLConfigFilesWaitsForWorkersOnError(t *testing.T) {
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	firstFailed := make(chan struct{})
	finished := make(chan error, 1)
	done := make(chan struct{})
	// Keep one worker inside policy discovery while another returns an error.
	failPolicyEnabled(t, func(_ string, call int) bool {
		if call == 1 {
			<-secondStarted
			close(firstFailed)
		} else {
			close(secondStarted)
			<-releaseSecond
		}
		return true
	})
	root := t.TempDir()
	go func() {
		defer close(done)
		_, _, _, _, err := ProcessYAMLConfigFiles(&schema.AtmosConfiguration{}, root,
			"", "", "", "", []string{filepath.Join(root, "first.yaml"), filepath.Join(root, "second.yaml")},
			false, false, false)
		finished <- err
	}()
	// Always release workers before test cleanup restores the policy lookup.
	var release sync.Once
	unblock := func() { release.Do(func() { close(releaseSecond) }) }
	defer func() {
		unblock()
		<-done
	}()
	select {
	case <-firstFailed:
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not reach policy discovery")
	}
	select {
	case err := <-finished:
		t.Fatalf("returned while a stack worker was still running: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
	case <-time.After(5 * time.Second):
		t.Fatal("stack processing did not finish after releasing its worker")
	}
}
