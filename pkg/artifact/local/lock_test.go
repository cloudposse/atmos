package local

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// Environment variables that turn the test binary into a lock helper process. The test binary
// doubles as a cross-platform subprocess: no Unix-only helper programs are involved.
const (
	helperModeEnv     = "_ATMOS_LOCAL_LOCK_HELPER"
	helperRootEnv     = "_ATMOS_LOCAL_LOCK_ROOT"
	helperResourceEnv = "_ATMOS_LOCAL_LOCK_RESOURCE"

	helperExitAcquired = 0
	helperExitBusy     = 3
	helperExitError    = 4
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperModeEnv); mode != "" {
		os.Exit(runLockHelper(mode))
	}
	os.Exit(m.Run())
}

// runLockHelper is the body of the helper process.
//
//	try:  attempt the lock for a short time; exit 0 when acquired, 3 when busy.
//	hold: acquire the lock, announce it on stdout, and hold it until stdin closes or the process is killed.
func runLockHelper(mode string) int {
	repo, err := newRepository("helper", artifact.RepositoryKindLocalDir, helperConfig(os.Getenv(helperRootEnv)), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return helperExitError
	}
	timeout := 400 * time.Millisecond
	if mode == "hold" {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	handle, err := repo.Lock(ctx, artifact.LockRequest{Resource: os.Getenv(helperResourceEnv), Owner: "helper-process"})
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return helperExitBusy
	case err != nil:
		fmt.Fprintln(os.Stderr, err)
		return helperExitError
	}
	if mode == "hold" {
		fmt.Println("locked")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if err := handle.Release(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return helperExitError
	}
	return helperExitAcquired
}

func helperCommand(t *testing.T, mode, root, resource string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), helperModeEnv+"="+mode, helperRootEnv+"="+root, helperResourceEnv+"="+resource)
	return cmd
}

func exitCode(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	return exitErr.ExitCode()
}

func lockRequest(resource string) artifact.LockRequest {
	return artifact.LockRequest{Resource: resource, Owner: "test-owner", OperationID: "op-lock", Required: artifact.LockScopeHost}
}

func TestLock_HandleContract(t *testing.T) {
	repo := newRepo(t, dirSpec())
	handle, err := repo.Lock(context.Background(), lockRequest("team/app:1.0.0"))
	require.NoError(t, err)
	assert.Equal(t, "team/app:1.0.0", handle.Resource())
	assert.Equal(t, "test-owner", handle.Owner())
	assert.Equal(t, artifact.LockScopeHost, handle.Scope())
	assert.NotEmpty(t, handle.Token())
	_, expires := handle.ExpiresAt()
	assert.False(t, expires, "local locks never expire")
	require.ErrorIs(t, handle.Renew(context.Background(), time.Minute), errUtils.ErrArtifactUnsupportedCapability)
	assert.FileExists(t, lockFile(repo, "team/app:1.0.0"), "the lock file lives in the managed namespace and is never removed")

	require.NoError(t, handle.Release(context.Background()))
	require.ErrorIs(t, handle.Release(context.Background()), errUtils.ErrArtifactConflict, "a handle cannot be released twice")
	assert.FileExists(t, lockFile(repo, "team/app:1.0.0"))

	other, err := repo.Lock(context.Background(), artifact.LockRequest{Resource: "x"})
	require.NoError(t, err)
	assert.NotEmpty(t, other.Owner(), "an empty owner defaults to the repository owner")
	require.NoError(t, other.Release(context.Background()))
}

func TestLock_RejectsUnsupportedGuarantees(t *testing.T) {
	repo := newRepo(t, dirSpec())
	request := lockRequest("r")
	request.Lease = time.Minute
	_, err := repo.Lock(context.Background(), request)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a lease cannot be fenced")

	request = lockRequest("r")
	request.Required = artifact.LockScopeRepository
	_, err = repo.Lock(context.Background(), request)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a host file lock is never reported as repository scope")
	capErr, ok := artifact.AsCapabilityError(err)
	require.True(t, ok)
	assert.Equal(t, "lock/repository", capErr.Capability)

	_, err = repo.Lock(context.Background(), artifact.LockRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	caps := repo.Capabilities()
	assert.True(t, caps.HasLockScope(artifact.LockScopeHost))
	assert.False(t, caps.HasLockScope(artifact.LockScopeRepository))
}

func TestLock_ReleaseChecksTheOwnershipToken(t *testing.T) {
	repo := newRepo(t, dirSpec())
	handle, err := repo.Lock(context.Background(), lockRequest("r"))
	require.NoError(t, err)
	real, ok := handle.(*fileLock)
	require.True(t, ok)

	forged := &fileLock{path: real.path, resource: real.resource, token: "not-the-token", release: make(chan struct{}), done: make(chan error, 1)}
	require.ErrorIs(t, forged.Release(context.Background()), errUtils.ErrArtifactConflict, "a wrong token never releases another operation's lock")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = repo.Lock(ctx, lockRequest("r"))
	require.ErrorIs(t, err, context.DeadlineExceeded, "the real owner still holds the lock")
	require.NoError(t, handle.Release(context.Background()))
}

func TestLock_CompetingHandlesAreMutuallyExclusive(t *testing.T) {
	// Separate repository objects over the same root model separate writers in one process.
	root := dirSpec().withRoot(filepath.Join(t.TempDir(), "shared"))
	writers := make([]*Repository, 4)
	for index := range writers {
		writers[index] = newRepo(t, root)
	}
	var (
		inside, maxInside, total atomic.Int32
		wg                       sync.WaitGroup
	)
	for _, writer := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				handle, err := writer.Lock(context.Background(), lockRequest("team/app:1.0.0"))
				if !assert.NoError(t, err) {
					return
				}
				now := inside.Add(1)
				for {
					seen := maxInside.Load()
					if now <= seen || maxInside.CompareAndSwap(seen, now) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				total.Add(1)
				inside.Add(-1)
				assert.NoError(t, handle.Release(context.Background()))
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, maxInside.Load(), "never two owners inside the critical section")
	assert.EqualValues(t, 20, total.Load())
}

func TestLock_CanceledAcquisitionLeaksNothing(t *testing.T) {
	repo := newRepo(t, dirSpec())
	holder, err := repo.Lock(context.Background(), lockRequest("r"))
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			handle, err := repo.Lock(ctx, lockRequest("r"))
			assert.Nil(t, handle)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
		}()
	}
	wg.Wait()
	require.NoError(t, holder.Release(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	next, err := repo.Lock(ctx, lockRequest("r"))
	require.NoError(t, err, "the canceled waiters took no ownership")
	require.NoError(t, next.Release(context.Background()))

	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	_, err = repo.Lock(canceled, lockRequest("r2"))
	if err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
	again, err := repo.Lock(context.Background(), lockRequest("r2"))
	require.NoError(t, err, "an already-canceled attempt leaves the resource free")
	require.NoError(t, again.Release(context.Background()))
}

// Two writers to one reference: the OS lock excludes them across the comparison and commit, so
// exactly one creates the reference and every other sees the conflict.
func TestTwoWritersToOneReference(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			repo := newRepo(t, spec)
			const writers = 6
			prepared := make([]*artifact.PreparedArtifact, writers)
			for index := range writers {
				prepared[index] = prepare(t, repo, &prepareSpec{opID: fmt.Sprintf("op-%d", index)})
			}
			outcomes := make([]error, writers)
			var wg sync.WaitGroup
			for index := range writers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, outcomes[index] = repo.Publish(context.Background(), prepared[index], artifact.PublishOptions{})
				}()
			}
			wg.Wait()
			created := 0
			for _, err := range outcomes {
				if err == nil {
					created++
				} else {
					require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
				}
			}
			assert.Equal(t, 1, created)
			assertIntact(t, repo, defaultRef())
			assert.Empty(t, listFiles(t, filepath.Join(repo.root, filepath.FromSlash(layout.ManagedRoot), "operations")), "no journal outlives its publication")
		})
	}
}

// A publication without a caller-held lock takes the reference lock itself, so it waits for an
// existing holder; with a caller-held lock it proceeds without deadlocking on itself.
func TestPublishHonorsTheReferenceLock(t *testing.T) {
	repo := newRepo(t, dirSpec())
	holder, err := repo.Lock(context.Background(), lockRequest(testName+":"+testVersion))
	require.NoError(t, err)

	prepared := prepare(t, repo, &prepareSpec{})
	done := make(chan error, 1)
	go func() {
		_, publishErr := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
		done <- publishErr
	}()
	select {
	case err := <-done:
		t.Fatalf("the publication did not wait for the held lock (returned %v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	require.NoError(t, holder.Release(context.Background()))
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the publication never resumed after the lock was released")
	}

	second := newRepo(t, dirSpec())
	held, err := second.Lock(context.Background(), lockRequest("team/other:1.0.0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Release(context.Background()) })
	_, err = second.Publish(context.Background(), prepare(t, second, &prepareSpec{name: "team/other"}), artifact.PublishOptions{Lock: held})
	require.NoError(t, err, "a service-held lock is honored, not re-acquired")
}

func TestPublishCancellationWhileWaitingLeaksNothing(t *testing.T) {
	repo := newRepo(t, dirSpec())
	holder, err := repo.Lock(context.Background(), lockRequest(testName+":"+testVersion))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = repo.Publish(ctx, prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, holder.Release(context.Background()))
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "nothing was written")
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
}

func TestLock_ExcludesOtherProcesses(t *testing.T) {
	repo := newRepo(t, dirSpec())
	resource := "team/app:1.0.0"
	handle, err := repo.Lock(context.Background(), lockRequest(resource))
	require.NoError(t, err)

	assert.Equal(t, helperExitBusy, exitCode(t, helperCommand(t, "try", repo.root, resource)), "another process cannot take a held lock")
	other := resource + "-other"
	assert.Equal(t, helperExitAcquired, exitCode(t, helperCommand(t, "try", repo.root, other)), "locks on other resources are independent")

	require.NoError(t, handle.Release(context.Background()))
	assert.Equal(t, helperExitAcquired, exitCode(t, helperCommand(t, "try", repo.root, resource)), "the lock is free after release")
}

func TestLock_ReleasesWhenItsOwnerProcessExits(t *testing.T) {
	repo := newRepo(t, dirSpec())
	resource := "team/app:1.0.0"
	cmd := helperCommand(t, "hold", repo.root, resource)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	exited := false
	defer func() {
		if !exited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "locked", strings.TrimSpace(line))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = repo.Lock(ctx, lockRequest(resource))
	require.ErrorIs(t, err, context.DeadlineExceeded, "the helper process holds the lock")

	// Kill the owner without letting it release: the OS must free the lock.
	_ = stdin.Close()
	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	exited = true

	acquire, cancelAcquire := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelAcquire()
	handle, err := repo.Lock(acquire, lockRequest(resource))
	require.NoError(t, err, "the lock is released when its owner exits")
	require.NoError(t, handle.Release(context.Background()))
}

func lockFile(repo *Repository, resource string) string {
	return filepath.Join(repo.root, filepath.FromSlash(layout.LockPath(resource)))
}
