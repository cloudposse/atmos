package azure

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/gofrs/flock"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// The msal-extensions lock file convention (Python msal-extensions CrossPlatLock, used by the
// Azure CLI for ~/.azure/msal_token_cache.json):
//
//   - Acquire: wait up to 5 seconds, polling every 0.25 seconds, until the lock file can be
//     created exclusively; then lock it (portalocker, an exclusive flock on POSIX) and write
//     "<pid> <argv0>" into it.
//   - Release: unlock, then delete the lock file.
//
// The lock (flock) is what excludes concurrent writers; the file's existence is the signal az
// polls on. A lock file left behind therefore costs every later az command the full 5-second
// wait. Atmos follows the same convention: it claims the file before locking it and deletes it
// on release, so az never waits on a lock Atmos no longer holds.
var (
	// The msalLockfileWait value bounds the wait for another process's lock file, as in msal-extensions.
	msalLockfileWait = 5 * time.Second
	// The msalLockfilePollInterval value is the polling interval of that wait, as in msal-extensions.
	msalLockfilePollInterval = 250 * time.Millisecond
)

// claimMSALLockfile waits, like msal-extensions, until lockPath can be created exclusively, so
// Atmos takes its turn after a live az process releases and deletes the file. A lock file that
// exists but that nobody holds (left by a crashed process or an older Atmos build) is taken over
// at once instead of being waited on. Claiming is best effort: on timeout, cancellation or an
// unexpected error the caller still takes the lock itself, which is what excludes writers. The
// returned claim records whether this call created the lock file (see msalLockfileClaim.abandon).
func claimMSALLockfile(ctx context.Context, lockPath string) msalLockfileClaim {
	deadline := time.Now().Add(msalLockfileWait)
	for {
		claim, err := createMSALLockfile(lockPath)
		switch {
		case claim.created():
			return claim
		case !errors.Is(err, fs.ErrExist):
			// For example a read-only directory: the lock below reports what is wrong.
			log.Debug("Could not create the MSAL cache lock file", "path", lockPath, "error", err)
			return msalLockfileClaim{}
		case !msalLockfileHeld(lockPath):
			log.Debug("Taking over an MSAL cache lock file nobody holds", "path", lockPath)
			return msalLockfileClaim{}
		case time.Now().After(deadline):
			log.Debug("Timed out waiting for the MSAL cache lock file; waiting on the lock instead", "path", lockPath)
			return msalLockfileClaim{}
		}
		select {
		case <-ctx.Done():
			return msalLockfileClaim{}
		case <-time.After(msalLockfilePollInterval):
		}
	}
}

// msalLockfileClaim is the outcome of claimMSALLockfile. It identifies the lock file this process
// created, if any, so the claim can be withdrawn when the lock is not acquired afterwards.
type msalLockfileClaim struct {
	// file is the lock file this process created, or nil when it took over or never created one.
	file os.FileInfo
}

// created reports whether this process created the lock file.
func (c msalLockfileClaim) created() bool {
	return c.file != nil
}

// abandon removes the lock file when the lock could not be acquired after this process created
// it, so az does not wait on a claim nobody will release. The file is removed only while lockPath
// still names the file this process created and nobody holds its lock: a lock file another
// process created since, or the same file another process took over and locked before this one
// could, is left alone. As on release, the probe lock is released before the file is deleted (the
// msal-extensions order, which also works on Windows, where a locked file may not be deletable).
func (c msalLockfileClaim) abandon(lockPath string) {
	if !c.created() {
		return
	}
	current, err := os.Stat(lockPath)
	if err != nil || !os.SameFile(c.file, current) {
		return
	}
	probe := flock.New(lockPath)
	locked, err := probe.TryLock()
	if err != nil || !locked {
		log.Debug("Keeping the MSAL cache lock file another process holds", "path", lockPath)
		return
	}
	_ = probe.Unlock()
	releaseMSALLockfile(lockPath)
}

// createMSALLockfile creates lockPath exclusively and records this process in it, as
// msal-extensions does. It returns an empty claim with an error satisfying fs.ErrExist when the
// file already exists.
func createMSALLockfile(lockPath string) (msalLockfileClaim, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FilePermissions)
	if err != nil {
		return msalLockfileClaim{}, err
	}
	_, writeErr := file.WriteString(msalLockfileOwner())
	info, statErr := file.Stat()
	if err := errors.Join(writeErr, statErr, file.Close()); err != nil {
		log.Debug("Could not record the owner in the MSAL cache lock file", "path", lockPath, "error", err)
	}
	if statErr != nil {
		// Without the file's identity it cannot be withdrawn safely later; it is still claimed.
		info, _ = os.Stat(lockPath)
	}
	return msalLockfileClaim{file: info}, nil
}

// msalLockfileHeld reports whether another process holds the lock on lockPath. A lock that
// cannot be probed counts as held, so Atmos keeps waiting rather than racing a live holder.
func msalLockfileHeld(lockPath string) bool {
	probe := flock.New(lockPath)
	locked, err := probe.TryLock()
	if err != nil {
		return true
	}
	if locked {
		_ = probe.Unlock()
		return false
	}
	return true
}

// releaseMSALLockfile deletes lockPath, as msal-extensions does when it releases the lock, so the
// next az or Atmos process does not wait for it. A file that is already gone (another process
// raced this one) or that cannot be deleted (a read-only directory) is not an error.
func releaseMSALLockfile(lockPath string) {
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Debug("Could not remove the MSAL cache lock file", "path", lockPath, "error", err)
	}
}

// msalLockfileOwner is the "<pid> <argv0>" record msal-extensions writes into its lock file.
func msalLockfileOwner() string {
	name := "atmos"
	if len(os.Args) > 0 {
		name = os.Args[0]
	}
	return strconv.Itoa(os.Getpid()) + " " + name
}
