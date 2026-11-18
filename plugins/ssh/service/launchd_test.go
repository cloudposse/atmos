package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
)

const (
	testUID = 501
	target  = "gui/501/com.cloudposse.atmos.ssh-agent"
	domain  = "gui/501"
)

func newTestLaunchd(t *testing.T, runner Runner) (*launchd, string) {
	t.Helper()
	p, defs := testPaths(t)
	m, err := newForOS("darwin", Options{Paths: p, Runner: runner, DefinitionDir: defs, UID: testUID})
	require.NoError(t, err)
	l := m.(*launchd)
	l.pause = noPause
	return l, defs
}

// launchctlState scripts a launchctl whose loaded state flips on bootstrap and bootout.
type launchctlState struct {
	loaded bool
	pid    int
}

func (s *launchctlState) reply(line string, _ int) Output {
	switch {
	case strings.HasPrefix(line, LaunchctlPath+" print "):
		if !s.loaded {
			return Output{ExitCode: 113, Stderr: "Could not find service"}
		}
		out := "com.cloudposse.atmos.ssh-agent = {\n\tactive count = 1\n\tstate = running\n"
		if s.pid > 0 {
			out += "\tpid = " + itoa(s.pid) + "\n"
		}
		return Output{Stdout: out + "}\n"}
	case strings.HasPrefix(line, LaunchctlPath+" bootstrap "):
		s.loaded = true
	case strings.HasPrefix(line, LaunchctlPath+" bootout "):
		s.loaded = false
	}
	return Output{}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestRenderPlist(t *testing.T) {
	spec := Spec{Executable: "/Users/me/.local/share/atmos/ssh/bin/atmos-ssh-1.2.3", Args: []string{"agent-serve", "--config", "/a&b/<c>.json"}}
	got := string(renderPlist(spec, "/Users/me/log file.log"))

	for _, want := range []string{
		"<key>Label</key>\n\t<string>com.cloudposse.atmos.ssh-agent</string>",
		"<string>/Users/me/.local/share/atmos/ssh/bin/atmos-ssh-1.2.3</string>",
		"<string>agent-serve</string>",
		"<string>--config</string>",
		"<string>/a&amp;b/&lt;c&gt;.json</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>KeepAlive</key>\n\t<true/>",
		"<key>StandardOutPath</key>\n\t<string>/Users/me/log file.log</string>",
		"<key>StandardErrorPath</key>\n\t<string>/Users/me/log file.log</string>",
	} {
		assert.Contains(t, got, want)
	}
	assert.Equal(t, spec.Executable, programFromPlist([]byte(got)))
}

func TestProgramFromPlistUnescapes(t *testing.T) {
	spec := Spec{Executable: "/x/a&b/atmos-ssh-1"}
	assert.Equal(t, "/x/a&b/atmos-ssh-1", programFromPlist(renderPlist(spec, "/l")))
	assert.Equal(t, "", programFromPlist([]byte("<plist/>")))
}

func TestLaunchdInstallFresh(t *testing.T) {
	st := &launchctlState{}
	runner := &fakeRunner{reply: st.reply}
	l, defs := newTestLaunchd(t, runner)
	spec := Spec{Executable: filepath.Join(t.TempDir(), "atmos-ssh-1.0.0"), Args: []string{"agent-serve"}}

	require.NoError(t, l.Install(context.Background(), spec))

	plist := filepath.Join(defs, Label+".plist")
	assert.Equal(t, []string{
		LaunchctlPath + " print " + target,
		LaunchctlPath + " bootstrap " + domain + " " + plist,
	}, runner.lines())
	data, err := os.ReadFile(plist)
	require.NoError(t, err)
	assert.Equal(t, string(renderPlist(spec, l.opts.Paths.ServiceLogFile())), string(data))
	for _, c := range runner.calls {
		assert.Empty(t, c.Env, "launchctl runs with an empty environment")
	}
}

func TestLaunchdInstallReplacesLoadedService(t *testing.T) {
	st := &launchctlState{loaded: true}
	runner := &fakeRunner{reply: st.reply}
	l, defs := newTestLaunchd(t, runner)

	require.NoError(t, l.Install(context.Background(), Spec{Executable: "/x/atmos-ssh-2.0.0"}))

	plist := filepath.Join(defs, Label+".plist")
	assert.Equal(t, []string{
		LaunchctlPath + " print " + target,
		LaunchctlPath + " bootout " + target,
		LaunchctlPath + " print " + target,
		LaunchctlPath + " bootstrap " + domain + " " + plist,
	}, runner.lines())
}

func TestLaunchdInstallReportsBootstrapFailure(t *testing.T) {
	runner := &fakeRunner{reply: func(line string, _ int) Output {
		if strings.Contains(line, " bootstrap ") {
			return Output{ExitCode: 5, Stderr: "Bootstrap failed: 5: Input/output error"}
		}
		return Output{ExitCode: 113}
	}}
	l, _ := newTestLaunchd(t, runner)
	err := l.Install(context.Background(), Spec{Executable: "/x/atmos-ssh-2.0.0"})
	require.ErrorIs(t, err, ErrCommandFailed)
	assert.Contains(t, err.Error(), "Input/output error")
}

func TestLaunchdInstallRejectsRelativeExecutable(t *testing.T) {
	runner := &fakeRunner{}
	l, defs := newTestLaunchd(t, runner)
	require.ErrorIs(t, l.Install(context.Background(), Spec{Executable: "atmos-ssh"}), ErrInvalidSpec)
	assert.Empty(t, runner.lines())
	_, err := os.Stat(defs)
	require.ErrorIs(t, err, os.ErrNotExist, "nothing is written")
}

func TestLaunchdStartStop(t *testing.T) {
	st := &launchctlState{}
	runner := &fakeRunner{reply: st.reply}
	l, defs := newTestLaunchd(t, runner)
	ctx := context.Background()

	require.ErrorIs(t, l.Start(ctx), ErrNotInstalled)
	assert.Empty(t, runner.lines())

	require.NoError(t, os.MkdirAll(defs, 0o755))
	plist := filepath.Join(defs, Label+".plist")
	require.NoError(t, os.WriteFile(plist, renderPlist(Spec{Executable: "/x/atmos-ssh-1"}, "/l"), 0o644))

	require.NoError(t, l.Start(ctx))
	assert.Equal(t, []string{
		LaunchctlPath + " print " + target,
		LaunchctlPath + " bootstrap " + domain + " " + plist,
	}, runner.lines())

	runner.calls = nil
	require.NoError(t, l.Start(ctx), "already loaded")
	assert.Equal(t, []string{LaunchctlPath + " print " + target}, runner.lines())

	runner.calls = nil
	require.NoError(t, l.Stop(ctx))
	assert.Equal(t, []string{
		LaunchctlPath + " print " + target,
		LaunchctlPath + " bootout " + target,
		LaunchctlPath + " print " + target,
	}, runner.lines())

	runner.calls = nil
	require.NoError(t, l.Stop(ctx), "already stopped")
	assert.Equal(t, []string{LaunchctlPath + " print " + target}, runner.lines())
}

func TestLaunchdUninstall(t *testing.T) {
	st := &launchctlState{loaded: true}
	runner := &fakeRunner{reply: st.reply}
	l, defs := newTestLaunchd(t, runner)
	require.NoError(t, os.MkdirAll(defs, 0o755))
	plist := filepath.Join(defs, Label+".plist")
	require.NoError(t, os.WriteFile(plist, []byte("x"), 0o644))

	require.NoError(t, l.Uninstall(context.Background()))
	_, err := os.Stat(plist)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, runner.lines(), LaunchctlPath+" bootout "+target)

	require.NoError(t, l.Uninstall(context.Background()), "uninstalling twice is fine")
}

func TestLaunchdStatusVersions(t *testing.T) {
	tests := []struct {
		name         string
		installed    string
		statePID     int
		stateVer     string
		wantRunning  bool
		wantDaemon   string
		wantMismatch bool
	}{
		{"same version", "2.0.0", 777, "2.0.0", true, "2.0.0", false},
		{"older daemon", "2.0.0", 777, "1.0.0", true, "1.0.0", true},
		{"stale state from another pid", "2.0.0", 111, "1.0.0", true, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &launchctlState{loaded: true, pid: 7}
			runner := &fakeRunner{reply: func(line string, n int) Output {
				out := st.reply(line, n)
				out.Stdout = strings.Replace(out.Stdout, "pid = 7", "pid = 777", 1)
				return out
			}}
			l, defs := newTestLaunchd(t, runner)
			p := l.opts.Paths
			require.NoError(t, os.MkdirAll(defs, 0o755))
			exe, err := StablePath(p, tt.installed)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(defs, Label+".plist"), renderPlist(Spec{Executable: exe}, "/l"), 0o644))
			require.NoError(t, agent.WriteState(p.StateFile(), agent.State{PID: tt.statePID, Version: tt.stateVer, Started: time.Now()}))

			got, err := l.Status(context.Background())
			require.NoError(t, err)
			assert.True(t, got.Installed)
			assert.True(t, got.Loaded)
			assert.Equal(t, tt.wantRunning, got.Running)
			assert.Equal(t, 777, got.PID)
			assert.Equal(t, exe, got.Executable)
			assert.Equal(t, tt.installed, got.InstalledVersion)
			assert.Equal(t, tt.wantDaemon, got.DaemonVersion)
			assert.Equal(t, tt.wantMismatch, got.VersionMismatch)
		})
	}
}

func TestLaunchdStatusNotInstalled(t *testing.T) {
	runner := &fakeRunner{reply: (&launchctlState{}).reply}
	l, _ := newTestLaunchd(t, runner)
	got, err := l.Status(context.Background())
	require.NoError(t, err)
	assert.False(t, got.Installed)
	assert.False(t, got.Loaded)
	assert.False(t, got.Running)
	assert.False(t, got.VersionMismatch)
}
