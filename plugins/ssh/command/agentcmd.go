package command

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/service"
)

const (
	// defaultLogLines is how many audit log lines "agent log" prints.
	defaultLogLines = 20
	// maxLogRead bounds how much of the audit log is read to find its last lines.
	maxLogRead = 1 << 20
)

// agentStart installs and starts the agent service, or restarts it when it is out of date.
func (c *call) agentStart(words []string) error {
	if err := c.flags.only(); err != nil {
		return err
	}
	if err := exactly(words, 0, "agent start"); err != nil {
		return err
	}
	res, err := daemon.Ensure(c.ctx, c.svc.deps.Daemon, c.paths)
	if err != nil {
		return err
	}
	if res.Installed {
		c.printf("Installed and started the agent service (version %s).\n", c.svc.deps.Daemon.Version)
		return nil
	}
	c.printf("The agent is already running (pid %d, version %s).\n", res.Status.PID, res.Status.DaemonVersion)
	return nil
}

func (c *call) manager() (service.Manager, error) {
	return c.svc.deps.Daemon.Service(c.paths)
}

// agentStop stops the agent service and keeps it stopped.
func (c *call) agentStop(words []string) error {
	if err := c.flags.only(); err != nil {
		return err
	}
	if err := exactly(words, 0, "agent stop"); err != nil {
		return err
	}
	mgr, err := c.manager()
	if err != nil {
		return err
	}
	if err := mgr.Stop(c.ctx); err != nil {
		return err
	}
	c.printf("Stopped the agent service. Using an identity starts it again.\n")
	return nil
}

// agentStatus reports whether the daemon runs, which version, and which sockets it serves.
func (c *call) agentStatus(words []string) error {
	if err := c.flags.only(); err != nil {
		return err
	}
	if err := exactly(words, 0, "agent status"); err != nil {
		return err
	}
	mgr, err := c.manager()
	if err != nil {
		return err
	}
	st, err := mgr.Status(c.ctx)
	if err != nil {
		return err
	}
	c.printStatus(st)
	return c.printSockets()
}

func (c *call) printStatus(st service.Status) {
	plugin := c.svc.deps.Daemon.Version
	field := func(label string, value any) { c.printf("%-19s %v\n", label+":", value) }
	field("Service installed", st.Installed)
	field("Running", st.Running)
	if st.Running {
		field("PID", st.PID)
	}
	field("Plugin version", plugin)
	field("Installed version", orDash(st.InstalledVersion))
	field("Daemon version", orDash(st.DaemonVersion))
	if st.Executable != "" {
		field("Executable", st.Executable)
	}
	if st.DefinitionPath != "" {
		field("Definition", st.DefinitionPath)
	}
	if st.LogFile != "" {
		field("Service log", st.LogFile)
	}
	field("Audit log", c.paths.AuditLogFile())
	switch {
	case st.VersionMismatch:
		c.printf("\nThe running daemon (%s) is not the installed version (%s). Run \"agent start\" to replace it.\n", orDash(st.DaemonVersion), orDash(st.InstalledVersion))
	case st.Installed && st.InstalledVersion != plugin:
		c.printf("\nThe installed daemon (%s) is not this plugin's version (%s). Run \"agent start\" to update it.\n", orDash(st.InstalledVersion), plugin)
	}
}

// printSockets lists the sockets the configuration asks for and whether each one answers.
func (c *call) printSockets() error {
	cfg, err := agent.LoadConfig(c.paths.AgentConfigFile())
	if errors.Is(err, agent.ErrConfigNotFound) {
		c.printf("\nNo identities are configured.\n")
		return nil
	}
	if err != nil {
		return err
	}
	c.printf("\nSockets:\n")
	for _, id := range cfg.Identities {
		socket, err := c.paths.SocketPath(id.Name)
		if err != nil {
			return err
		}
		state := "serving"
		if err := c.svc.deps.CheckSocket(c.ctx, socket, nil); err != nil {
			state = "not serving (" + err.Error() + ")"
		}
		c.printf("  %s  %s  allow=%s  %s\n", id.Name, socket, allowText(id.Allow), state)
	}
	return nil
}

func allowText(allow []agent.Allow) string {
	out := make([]string, len(allow))
	for i, a := range allow {
		out[i] = string(a)
	}
	return strings.Join(out, ",")
}

// agentLog prints the last lines of the audit log.
func (c *call) agentLog(words []string) error {
	if err := c.flags.only("lines"); err != nil {
		return err
	}
	if err := exactly(words, 0, "agent log [--lines N]"); err != nil {
		return err
	}
	n, err := c.flags.count("lines", defaultLogLines)
	if err != nil {
		return err
	}
	lines, err := tailLines(c.paths.AuditLogFile(), n)
	if errors.Is(err, os.ErrNotExist) {
		c.printf("No audit log yet: %s\n", c.paths.AuditLogFile())
		return nil
	}
	if err != nil {
		return err
	}
	for _, l := range lines {
		c.printf("%s\n", l)
	}
	return nil
}

// tailLines returns the last n lines of the file at path, reading at most maxLogRead bytes.
func tailLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset, truncated := int64(0), false
	if info.Size() > maxLogRead {
		offset, truncated = info.Size()-maxLogRead, true
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if truncated {
		// The first line is probably cut in half.
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}
