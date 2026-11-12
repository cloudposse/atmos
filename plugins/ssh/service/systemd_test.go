package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
)

const fakeSystemctl = "/usr/bin/systemctl"

func newTestSystemd(t *testing.T, runner Runner) (*systemd, string) {
	t.Helper()
	p, defs := testPaths(t)
	m, err := newForOS("linux", Options{
		Paths: p, Runner: runner, DefinitionDir: defs, UID: testUID,
		FindExecutable: func([]string) (string, bool) { return fakeSystemctl, true },
	})
	require.NoError(t, err)
	return m.(*systemd), filepath.Join(defs, UnitName)
}

func TestRenderUnit(t *testing.T) {
	spec := Spec{
		Executable: "/home/me/.local/share/atmos/ssh/bin/atmos-ssh-1.2.3",
		Args:       []string{"agent-serve", "--config", `/p/with space/"q"/50%/$HOME.json`},
	}
	got := string(renderUnit(spec, "/home/me/agent.log"))

	want := `[Unit]
Description=Atmos SSH agent

[Service]
Type=simple
ExecStart="/home/me/.local/share/atmos/ssh/bin/atmos-ssh-1.2.3" "agent-serve" "--config" "/p/with space/\"q\"/50%%/$$HOME.json"
Restart=always
RestartSec=2
StandardOutput=append:/home/me/agent.log
StandardError=append:/home/me/agent.log

[Install]
WantedBy=default.target
`
	assert.Equal(t, want, got)
	assert.Equal(t, spec.Executable, programFromUnit([]byte(got)))
}

func TestProgramFromUnitDecodesEscapes(t *testing.T) {
	spec := Spec{Executable: `/odd "dir"/100%/$x/atmos-ssh-1`}
	assert.Equal(t, spec.Executable, programFromUnit(renderUnit(spec, "/l")))
	assert.Equal(t, "", programFromUnit([]byte("[Unit]\n")))
}

func TestSystemdInstall(t *testing.T) {
	runner := &fakeRunner{}
	s, unit := newTestSystemd(t, runner)
	spec := Spec{Executable: "/x/atmos-ssh-1.0.0", Args: []string{"agent-serve"}}

	require.NoError(t, s.Install(context.Background(), spec))

	assert.Equal(t, []string{
		fakeSystemctl + " --user daemon-reload",
		fakeSystemctl + " --user enable " + UnitName,
		fakeSystemctl + " --user restart " + UnitName,
	}, runner.lines())
	data, err := os.ReadFile(unit)
	require.NoError(t, err)
	assert.Equal(t, string(renderUnit(spec, s.opts.Paths.ServiceLogFile())), string(data))
	for _, c := range runner.calls {
		assert.ElementsMatch(t, []string{
			"XDG_RUNTIME_DIR=/run/user/501",
			"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/501/bus",
		}, c.Env)
	}
}

func TestSystemdInstallStopsOnFailure(t *testing.T) {
	runner := &fakeRunner{reply: func(line string, _ int) Output {
		if strings.HasSuffix(line, "enable "+UnitName) {
			return Output{ExitCode: 1, Stderr: "Failed to connect to bus"}
		}
		return Output{}
	}}
	s, _ := newTestSystemd(t, runner)
	err := s.Install(context.Background(), Spec{Executable: "/x/atmos-ssh-1"})
	require.ErrorIs(t, err, ErrCommandFailed)
	assert.Contains(t, err.Error(), "Failed to connect to bus")
	assert.Len(t, runner.lines(), 2, "restart is not attempted")
}

func TestSystemdStartStop(t *testing.T) {
	runner := &fakeRunner{}
	s, unit := newTestSystemd(t, runner)
	ctx := context.Background()

	require.ErrorIs(t, s.Start(ctx), ErrNotInstalled)
	require.NoError(t, s.Stop(ctx), "stopping what is not installed is a no-op")
	assert.Empty(t, runner.lines())

	require.NoError(t, os.MkdirAll(filepath.Dir(unit), 0o755))
	require.NoError(t, os.WriteFile(unit, []byte("x"), 0o644))
	require.NoError(t, s.Start(ctx))
	require.NoError(t, s.Stop(ctx))
	assert.Equal(t, []string{
		fakeSystemctl + " --user start " + UnitName,
		fakeSystemctl + " --user stop " + UnitName,
	}, runner.lines())
}

func TestSystemdUninstall(t *testing.T) {
	runner := &fakeRunner{}
	s, unit := newTestSystemd(t, runner)
	require.NoError(t, os.MkdirAll(filepath.Dir(unit), 0o755))
	require.NoError(t, os.WriteFile(unit, []byte("x"), 0o644))

	require.NoError(t, s.Uninstall(context.Background()))
	assert.Equal(t, []string{
		fakeSystemctl + " --user disable --now " + UnitName,
		fakeSystemctl + " --user daemon-reload",
	}, runner.lines())
	_, err := os.Stat(unit)
	require.ErrorIs(t, err, os.ErrNotExist)

	runner.calls = nil
	require.NoError(t, s.Uninstall(context.Background()))
	assert.Empty(t, runner.lines())
}

func TestSystemdStatusVersions(t *testing.T) {
	show := func(active string, pid string) *fakeRunner {
		return &fakeRunner{reply: func(string, int) Output {
			return Output{Stdout: "LoadState=loaded\nActiveState=" + active + "\nMainPID=" + pid + "\n"}
		}}
	}
	tests := []struct {
		name         string
		runner       *fakeRunner
		stateVer     string
		wantRunning  bool
		wantDaemon   string
		wantMismatch bool
	}{
		{"running old daemon", show("active", "4321"), "1.0.0", true, "1.0.0", true},
		{"running current daemon", show("active", "4321"), "2.0.0", true, "2.0.0", false},
		{"inactive ignores state file", show("inactive", "0"), "1.0.0", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, unit := newTestSystemd(t, tt.runner)
			exe, err := StablePath(s.opts.Paths, "2.0.0")
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Dir(unit), 0o755))
			require.NoError(t, os.WriteFile(unit, renderUnit(Spec{Executable: exe}, "/l"), 0o644))
			require.NoError(t, agent.WriteState(s.opts.Paths.StateFile(), agent.State{PID: 4321, Version: tt.stateVer, Started: time.Now()}))

			got, err := s.Status(context.Background())
			require.NoError(t, err)
			assert.True(t, got.Installed)
			assert.True(t, got.Loaded)
			assert.Equal(t, tt.wantRunning, got.Running)
			assert.Equal(t, exe, got.Executable)
			assert.Equal(t, "2.0.0", got.InstalledVersion)
			assert.Equal(t, tt.wantDaemon, got.DaemonVersion)
			assert.Equal(t, tt.wantMismatch, got.VersionMismatch)
			assert.Equal(t, []string{fakeSystemctl + " --user show " + UnitName + " --property=LoadState,ActiveState,MainPID"}, tt.runner.lines())
		})
	}
}

func TestSystemdStatusNotInstalled(t *testing.T) {
	runner := &fakeRunner{}
	s, _ := newTestSystemd(t, runner)
	got, err := s.Status(context.Background())
	require.NoError(t, err)
	assert.False(t, got.Installed)
	assert.Empty(t, runner.lines())
}

func TestSystemctlLookup(t *testing.T) {
	p, defs := testPaths(t)
	var asked []string
	m, err := newForOS("linux", Options{
		Paths: p, Runner: &fakeRunner{}, DefinitionDir: defs, UID: testUID,
		FindExecutable: func(c []string) (string, bool) { asked = c; return "", false },
	})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(defs, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defs, UnitName), []byte("x"), 0o644))

	require.ErrorIs(t, m.Start(context.Background()), ErrNoSystemctl)
	assert.Contains(t, asked, "/usr/bin/systemctl")
	assert.Contains(t, asked, "/bin/systemctl")
}

func TestFindExisting(t *testing.T) {
	dir := t.TempDir()
	notExec := filepath.Join(dir, "plain")
	exe := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(notExec, []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(exe, []byte("x"), 0o755))

	got, ok := findExisting([]string{filepath.Join(dir, "missing"), dir, notExec, exe})
	if os.PathSeparator == '\\' {
		t.Skip("POSIX execute bits only")
	}
	require.True(t, ok)
	assert.Equal(t, exe, got)
	_, ok = findExisting([]string{filepath.Join(dir, "missing")})
	assert.False(t, ok)
}
