package source

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStateProbeStreamsTopLevelMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, contents string
		want           bool
	}{
		{"header first", `{"version":4,"lineage":"state","serial":0,"resources":[]}`, true},
		{"header last", `{"resources":[{"values":{"nested":[1,true,null,{"version":9}]}}],"serial":2,"lineage":"state","version":4}`, true},
		{"nested impostor", `{"resources":[{"version":4,"lineage":"nested","serial":2}]}`, false},
		{"invalid serial", `{"version":4,"lineage":"state","serial":"2"}`, false},
		{"ordinary source", `{"configuration":{"items":[{},[]]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, err := inspectStateMetadata(strings.NewReader(tc.contents))
			require.NoError(t, err)
			require.Equal(t, tc.want, state)
		})
	}
	readFailure := errors.New("state reader unavailable")
	state, err := inspectStateMetadata(io.MultiReader(strings.NewReader(`{"version":4,"lineage":"state","serial":2,`), iotest.ErrReader(readFailure)))
	require.NoError(t, err, "recognized header must stop before reading resources")
	require.True(t, state)
	_, err = inspectStateMetadata(io.MultiReader(strings.NewReader(`{"resources":[`), iotest.ErrReader(readFailure)))
	require.ErrorIs(t, err, readFailure)
}

func writeLargeRefreshFile(t *testing.T, path string, state bool) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	if state {
		_, err = io.WriteString(file, `{"version":4,"lineage":"large-runtime","serial":9,"resources":[`)
		require.NoError(t, err)
	}
	// Whitespace makes the state valid while exceeding the former 64 MiB limit.
	block := strings.Repeat(" ", 1024*1024)
	for range 65 {
		_, err = io.WriteString(file, block)
		require.NoError(t, err)
	}
	if state {
		_, err = io.WriteString(file, `]}`)
		require.NoError(t, err)
	}
	require.NoError(t, file.Close())
}

func refreshFileDigest(t *testing.T, path string) []byte {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	require.NoError(t, err)
	return hash.Sum(nil)
}

func TestSourceRefreshProtectsLargeCustomState(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "replace"}[replace], func(t *testing.T) {
			source, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
			refreshWrite(t, source, "custom.data", "source fixture")
			refreshWrite(t, source, "main.tf", "# initial")
			spec := &schema.VendorComponentSource{Uri: source}
			require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
			state := filepath.Join(target, "custom.data")
			writeLargeRefreshFile(t, state, true)
			before := refreshFileDigest(t, state)
			inode, err := os.Stat(state)
			require.NoError(t, err)
			require.Greater(t, inode.Size(), int64(64*1024*1024))
			if !replace {
				require.NoError(t, os.Remove(filepath.Join(source, "custom.data")))
			}
			err = VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil))
			if replace {
				require.ErrorContains(t, err, "collides with runtime artifact")
			} else {
				require.NoError(t, err)
			}
			after, err := os.Stat(state)
			require.NoError(t, err)
			require.True(t, os.SameFile(inode, after))
			require.Equal(t, before, refreshFileDigest(t, state))
		})
	}
}

func TestSourceRefreshReplacesLargeNonStateSource(t *testing.T) {
	source, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	refreshWrite(t, source, "custom.data", "source fixture")
	spec := &schema.VendorComponentSource{Uri: source}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	writeLargeRefreshFile(t, filepath.Join(target, "custom.data"), false)
	refreshWrite(t, source, "custom.data", "updated source")
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	content, err := os.ReadFile(filepath.Join(target, "custom.data"))
	require.NoError(t, err)
	require.Equal(t, "updated source", string(content))
}

func TestSourceRefreshRefusesUnreadableOwnedArtifact(t *testing.T) {
	source, target := t.TempDir(), filepath.Join(t.TempDir(), "target")
	refreshWrite(t, source, "custom.data", "source fixture")
	spec := &schema.VendorComponentSource{Uri: source}
	require.NoError(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)))
	path := filepath.Join(target, "custom.data")
	refreshWrite(t, target, "custom.data", `{"version":4,"lineage":"state","serial":1}`)
	before := refreshFileDigest(t, path)
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if file, err := os.Open(path); err == nil {
		_ = file.Close()
		t.Skip("host permissions allow reading mode-000 files")
	}
	require.NoError(t, os.Remove(filepath.Join(source, "custom.data")))
	require.ErrorContains(t, VendorSource(t.Context(), nil, spec, target, WithRuntimePreservation(nil)), "inspect runtime artifact")
	require.NoError(t, os.Chmod(path, 0o600))
	require.Equal(t, before, refreshFileDigest(t, path))
}

func TestStateProbePropagatesHeaderReadErrors(t *testing.T) {
	failure := errors.New("state file became unreadable")
	for _, prefix := range []string{`{"vers`, `{"version":`} {
		t.Run(prefix, func(t *testing.T) {
			reader := io.MultiReader(strings.NewReader(prefix), iotest.ErrReader(failure))
			state, err := inspectStateMetadata(reader)
			require.False(t, state)
			require.ErrorIs(t, err, failure, "an I/O failure must not classify an owned artifact as ordinary source")
		})
	}
}
