package exec

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
