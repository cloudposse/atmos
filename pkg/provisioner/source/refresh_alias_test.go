package source

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSourceRefreshCancellationWhileWaitingForPublication(t *testing.T) {
	src, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	refreshWrite(t, src, "main.tf", "# source")
	canonical, err := resolveExistingSymlinks(target)
	require.NoError(t, err)
	lock := filelock.New(provisioner.LockCoordPath(canonical+".source-refresh") + ".lock")
	require.NoError(t, lock.WithExclusive(t.Context(), func() error {
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		err := VendorSource(ctx, nil, &schema.VendorComponentSource{Uri: src}, target, WithRuntimePreservation(nil))
		require.ErrorIs(t, err, errUtils.ErrCacheLocked)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NoDirExists(t, target)
		return nil
	}))
	// Cancellation releases only the waiter's claim; a later publisher can recover.
	require.NoError(t, VendorSource(t.Context(), nil, &schema.VendorComponentSource{Uri: src}, target, WithRuntimePreservation(nil)))
	content, err := os.ReadFile(filepath.Join(target, "main.tf"))
	require.NoError(t, err)
	require.Equal(t, "# source", string(content))
}

func TestSourceRefreshSerializesAncestorAliasOwners(t *testing.T) {
	parent := t.TempDir()
	realParent, aliasParent := filepath.Join(parent, "real"), filepath.Join(parent, "alias")
	require.NoError(t, os.Mkdir(realParent, 0o755))
	if err := os.Symlink(realParent, aliasParent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	firstSource, secondSource := t.TempDir(), t.TempDir()
	refreshWrite(t, firstSource, "main.tf", "# winning source")
	refreshWrite(t, secondSource, "main.tf", "# losing source")
	firstSpec := &schema.VendorComponentSource{Uri: firstSource}
	secondSpec := &schema.VendorComponentSource{Uri: secondSource}
	firstTarget := &Target{Dir: filepath.Join(realParent, "work"), IsWorkdir: true, EnforceOwner: true, Component: "module", Instance: "first"}
	secondTarget := &Target{Dir: filepath.Join(aliasParent, "work"), IsWorkdir: true, EnforceOwner: true, Component: "module", Instance: "second"}

	staged, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	pauseBeforePublication := func(opts *vendorSourceOptions) {
		metadata := opts.metadata
		opts.metadata = func(stage string) error {
			if err := metadata(stage); err != nil {
				return err
			}
			close(staged)
			<-release
			return nil
		}
	}
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- VendorSource(t.Context(), nil, firstSpec, firstTarget.Dir, WithRuntimePreservation(nil), withProvenance(firstTarget, "dev", firstSpec, nil), pauseBeforePublication)
	}()
	select {
	case <-staged:
	case err := <-firstResult:
		t.Fatalf("first source failed before staging: %v", err)
	}
	// The winner still owns the publication lock. The aliased contender must
	// time out acquiring that same lock, rather than publish into the same tree.
	secondErr := VendorSource(t.Context(), nil, secondSpec, secondTarget.Dir, WithRuntimePreservation(nil), withProvenance(secondTarget, "dev", secondSpec, nil))
	unblock()
	require.NoError(t, <-firstResult)
	require.ErrorIs(t, secondErr, errUtils.ErrCacheLocked)
	metadata, err := workdir.ReadMetadata(firstTarget.Dir)
	require.NoError(t, err)
	require.Equal(t, "first", metadata.Component)
	content, err := os.ReadFile(filepath.Join(firstTarget.Dir, "main.tf"))
	require.NoError(t, err)
	require.Equal(t, "# winning source", string(content))
	refreshWrite(t, firstTarget.Dir, "terraform.tfstate", `{"version":4,"lineage":"winner","serial":1}`)
	before := refreshFileDigest(t, filepath.Join(firstTarget.Dir, "terraform.tfstate"))
	require.ErrorIs(t, VendorSource(t.Context(), nil, secondSpec, secondTarget.Dir, WithRuntimePreservation(nil), withProvenance(secondTarget, "dev", secondSpec, nil)), errUtils.ErrWorkdirProvision)
	require.Equal(t, before, refreshFileDigest(t, filepath.Join(firstTarget.Dir, "terraform.tfstate")))
	content, err = os.ReadFile(filepath.Join(firstTarget.Dir, "main.tf"))
	require.NoError(t, err)
	require.Equal(t, "# winning source", string(content))
}
