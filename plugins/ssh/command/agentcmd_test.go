package command_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/command"
	"github.com/cloudposse/atmos/plugins/ssh/service"
)

func runningStatus() service.Status {
	return service.Status{
		Installed: true, Loaded: true, Running: true, PID: 321, Executable: "/data/bin/atmos-ssh-0.1.0",
		InstalledVersion: version, DaemonVersion: version, DefinitionPath: "/agents/x.plist", LogFile: "/data/agent.log",
	}
}

func TestAgentStatusOfACurrentDaemon(t *testing.T) {
	r := newRig(t)
	r.mgr.St = runningStatus()
	r.configure(agent.Identity{Name: "ops", Store: "file", Key: "ops", Allow: []agent.Allow{agent.AllowSigning, agent.AllowAuthentication}})

	code, out, errOut := r.run("agent", "status")

	require.Equal(t, command.ExitOK, code, errOut)
	for _, want := range []string{"Service installed:  true", "Running:            true", "PID:                321", "Plugin version:     0.1.0", "Installed version:  0.1.0", "Daemon version:     0.1.0", "Audit log:"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "Run \"agent start\"")
	socket, err := r.paths.SocketPath("ops")
	require.NoError(t, err)
	assert.Contains(t, out, "ops  "+socket+"  allow=signing,authentication  serving")
}

func TestAgentStatusReportsAVersionMismatch(t *testing.T) {
	r := newRig(t)
	st := runningStatus()
	st.DaemonVersion, st.VersionMismatch = "0.0.9", true
	r.mgr.St = st

	code, out, _ := r.run("agent", "status")

	assert.Equal(t, command.ExitOK, code)
	assert.Contains(t, out, "Daemon version:     0.0.9")
	assert.Contains(t, out, "The running daemon (0.0.9) is not the installed version (0.1.0). Run \"agent start\" to replace it.")
}

func TestAgentStatusReportsAnInstalledCopyThatIsNotThisPluginsVersion(t *testing.T) {
	r := newRig(t)
	st := runningStatus()
	st.InstalledVersion, st.DaemonVersion = "0.0.9", "0.0.9"
	r.mgr.St = st

	_, out, _ := r.run("agent", "status")

	assert.Contains(t, out, "The installed daemon (0.0.9) is not this plugin's version (0.1.0)")
}

func TestAgentStatusOfNothingInstalled(t *testing.T) {
	r := newRig(t)

	code, out, _ := r.run("agent", "status")

	assert.Equal(t, command.ExitOK, code)
	assert.Contains(t, out, "Service installed:  false")
	assert.Contains(t, out, "Running:            false")
	assert.Contains(t, out, "No identities are configured.")
}

func TestAgentStatusNamesSocketsThatDoNotAnswer(t *testing.T) {
	r := newRig(t)
	r.mgr.St = runningStatus()
	r.configure(agent.Identity{Name: "ops", Store: "file", Key: "ops", Allow: []agent.Allow{agent.AllowSigning}})
	r.socketErr = errors.New("connection refused")

	_, out, _ := r.run("agent", "status")

	assert.Contains(t, out, "not serving (connection refused)")
}

func TestAgentStatusFailsWhenTheServiceCannotBeQueried(t *testing.T) {
	r := newRig(t)
	r.mgr.StatusErr = errors.New("launchctl exploded")

	code, _, errOut := r.run("agent", "status")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, errOut, "launchctl exploded")
}

func TestAgentStartInstallsAndStartsTheService(t *testing.T) {
	r := newRig(t)

	code, out, errOut := r.run("agent", "start")

	require.Equal(t, command.ExitOK, code, errOut)
	assert.Contains(t, out, "Installed and started the agent service (version 0.1.0)")
	require.Equal(t, 1, r.mgr.InstallCount())
	assert.Equal(t, []string{"agent-serve", "--root", r.paths.Root()}, r.mgr.Installs[0].Args)
	assert.Equal(t, 1, r.mgr.Starts)
}

func TestAgentStartLeavesARunningCurrentDaemonAlone(t *testing.T) {
	r := newRig(t)
	r.mgr.St = runningStatus()

	code, out, _ := r.run("agent", "start")

	assert.Equal(t, command.ExitOK, code)
	assert.Contains(t, out, "already running (pid 321, version 0.1.0)")
	assert.Zero(t, r.mgr.InstallCount())
}

func TestAgentStartReplacesAMismatchedDaemon(t *testing.T) {
	r := newRig(t)
	st := runningStatus()
	st.VersionMismatch = true
	r.mgr.St = st

	code, _, _ := r.run("agent", "start")

	assert.Equal(t, command.ExitOK, code)
	assert.Equal(t, 1, r.mgr.InstallCount())
}

func TestAgentStartReportsAnInstallFailure(t *testing.T) {
	r := newRig(t)
	r.mgr.InstallErr = service.ErrCommandFailed

	code, _, errOut := r.run("agent", "start")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, errOut, "install the agent service")
}

func TestAgentStop(t *testing.T) {
	r := newRig(t)
	r.mgr.St = runningStatus()

	code, out, _ := r.run("agent", "stop")

	assert.Equal(t, command.ExitOK, code)
	assert.Equal(t, 1, r.mgr.Stops)
	assert.Contains(t, out, "Stopped the agent service")
}

func TestAgentLogPrintsTheTailOfTheAuditLog(t *testing.T) {
	r := newRig(t)
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "{\"n\":%d}\n", i)
	}
	r.writeFile(r.paths.AuditLogFile(), b.String())

	_, def, _ := r.run("agent", "log")
	_, two, _ := r.run("agent", "log", "--lines", "2")

	assert.Equal(t, 20, strings.Count(def, "\n"))
	assert.True(t, strings.HasPrefix(def, "{\"n\":11}\n"))
	assert.Equal(t, "{\"n\":29}\n{\"n\":30}\n", two)
}

func TestAgentLogOfALargeFileReadsOnlyTheEnd(t *testing.T) {
	r := newRig(t)
	line := strings.Repeat("x", 99) + "\n"
	r.writeFile(r.paths.AuditLogFile(), strings.Repeat(line, 20000)+"last\n")

	_, out, _ := r.run("agent", "log", "--lines", "2")

	assert.Equal(t, strings.Repeat("x", 99)+"\nlast\n", out)
}

func TestAgentLogWithoutALogOrWithAnEmptyOne(t *testing.T) {
	r := newRig(t)

	code, out, _ := r.run("agent", "log")
	assert.Equal(t, command.ExitOK, code)
	assert.Contains(t, out, "No audit log yet")

	r.writeFile(r.paths.AuditLogFile(), "")
	code, out, _ = r.run("agent", "log")
	assert.Equal(t, command.ExitOK, code)
	assert.Empty(t, out)

	require.NoError(t, os.Remove(r.paths.AuditLogFile()))
}
