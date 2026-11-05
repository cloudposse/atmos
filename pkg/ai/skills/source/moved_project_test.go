package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMovedProjectReportsSafeRecovery(t *testing.T) {
	e, _ := fixture(t)
	run(t, e, Options{})
	original := e.Project
	moved := filepath.Join(realTemp(t), "moved")
	require.NoError(t, os.Rename(original, moved))
	e.Project, e.Config.BasePath = moved, moved
	statePath := filepath.Join(e.stateDir(scopeProject), "installations.json")
	before, err := os.ReadFile(statePath)
	require.NoError(t, err)
	_, err = e.Run(context.Background(), Options{Check: true})
	require.ErrorIs(t, err, ErrInvalid)
	require.ErrorContains(t, err, "project skill state")
	require.ErrorContains(t, err, original)
	require.ErrorContains(t, err, moved)
	require.ErrorContains(t, err, "uninstall its project skills before moving it again")
	after, err := os.ReadFile(statePath)
	require.NoError(t, err)
	require.Equal(t, before, after)
	// Following the recovery guidance preserves ownership until uninstall succeeds.
	require.NoError(t, os.Rename(moved, original))
	e.Project, e.Config.BasePath = original, original
	run(t, e, Options{Check: true})
	run(t, e, Options{Uninstall: true, Scope: scopeProject})
	require.NoError(t, os.Rename(original, moved))
	e.Project, e.Config.BasePath = moved, moved
	run(t, e, Options{})
	run(t, e, Options{Check: true})
}

func TestUserRecordsRetainForeignProjectOwnership(t *testing.T) {
	e, _ := fixture(t)
	run(t, e, Options{Scope: scopeUser})
	e.Project = realTemp(t)
	state, err := e.loadState(scopeUser)
	require.NoError(t, err)
	require.NotEmpty(t, state.Records)
	require.NotEqual(t, e.Project, state.Records[0].Project)
}
