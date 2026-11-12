package daemon_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/service"
)

const version = "1.2.3"

func deps(m *testfakes.Manager) (daemon.Deps, *int) {
	installs := 0
	return daemon.Deps{
		Service: func(*paths.Paths) (service.Manager, error) { return m, nil },
		InstallExecutable: func(p *paths.Paths, v string) (string, error) {
			installs++
			return service.StablePath(p, v)
		},
		Version: version,
	}, &installs
}

func current() service.Status {
	return service.Status{Installed: true, Loaded: true, Running: true, PID: 7, InstalledVersion: version, DaemonVersion: version}
}

func TestArgsAlwaysNameTheRoot(t *testing.T) {
	p := testfakes.Paths(t)
	assert.Equal(t, []string{"agent-serve", "--root", p.Root()}, daemon.Args(p))
}

func TestEnsureLeavesACurrentDaemonAlone(t *testing.T) {
	m := &testfakes.Manager{Version: version, St: current()}
	d, installs := deps(m)

	res, err := daemon.Ensure(context.Background(), d, testfakes.Paths(t))

	require.NoError(t, err)
	assert.False(t, res.Installed)
	assert.Equal(t, 7, res.Status.PID)
	assert.Zero(t, *installs)
	assert.Zero(t, m.InstallCount())
	assert.Zero(t, m.Starts)
}

func TestEnsureInstallsAndStartsWhenTheDaemonIsMissingStoppedOrOld(t *testing.T) {
	tests := []struct {
		name string
		st   service.Status
	}{
		{"not installed", service.Status{}},
		{"installed but stopped", service.Status{Installed: true, InstalledVersion: version}},
		{"daemon is a different version", func() service.Status { s := current(); s.VersionMismatch = true; return s }()},
		{"installed copy is older", func() service.Status { s := current(); s.InstalledVersion = "1.0.0"; return s }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &testfakes.Manager{Version: version, St: tt.st}
			d, installs := deps(m)
			p := testfakes.Paths(t)

			res, err := daemon.Ensure(context.Background(), d, p)

			require.NoError(t, err)
			assert.True(t, res.Installed)
			assert.Equal(t, 1, *installs)
			require.Equal(t, 1, m.InstallCount())
			assert.Equal(t, daemon.Args(p), m.Installs[0].Args)
			assert.Equal(t, 1, m.Starts)
		})
	}
}

func TestEnsureTreatsAStatusErrorAsNeedingAnInstall(t *testing.T) {
	m := &testfakes.Manager{Version: version, StatusErr: errors.New("launchctl failed")}
	d, _ := deps(m)

	res, err := daemon.Ensure(context.Background(), d, testfakes.Paths(t))

	require.NoError(t, err)
	assert.True(t, res.Installed)
}

func TestEnsureReportsEachFailingStep(t *testing.T) {
	boom := testfakes.ErrBoom
	tests := []struct {
		name string
		edit func(d *daemon.Deps, m *testfakes.Manager)
		want string
	}{
		{"no service manager", func(d *daemon.Deps, _ *testfakes.Manager) {
			d.Service = func(*paths.Paths) (service.Manager, error) { return nil, service.ErrUnsupportedPlatform }
		}, "agent service"},
		{"copy fails", func(d *daemon.Deps, _ *testfakes.Manager) {
			d.InstallExecutable = func(*paths.Paths, string) (string, error) { return "", boom }
		}, "install the agent executable"},
		{"install fails", func(_ *daemon.Deps, m *testfakes.Manager) { m.InstallErr = boom }, "install the agent service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &testfakes.Manager{Version: version}
			d, _ := deps(m)
			tt.edit(&d, m)

			_, err := daemon.Ensure(context.Background(), d, testfakes.Paths(t))

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestEnsureStopsEarlyWhereThereIsNoServiceManager(t *testing.T) {
	m := &testfakes.Manager{Version: version, StatusErr: service.ErrUnsupportedPlatform}
	d, installs := deps(m)

	_, err := daemon.Ensure(context.Background(), d, testfakes.Paths(t))

	require.ErrorIs(t, err, service.ErrUnsupportedPlatform)
	assert.Zero(t, *installs, "the executable is not copied for a service that cannot run it")
}
