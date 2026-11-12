package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// UnitName is the systemd user unit of the agent service.
const UnitName = "atmos-ssh-agent.service"

const unitMode = os.FileMode(0o644)

// systemctlCandidates are the places systemctl may live. The plugin has no PATH to search.
var systemctlCandidates = []string{
	"/usr/bin/systemctl",
	"/bin/systemctl",
	"/usr/local/bin/systemctl",
	"/run/current-system/sw/bin/systemctl",
}

// systemd drives a user unit with systemctl --user.
type systemd struct {
	opts Options
	unit string
}

func newSystemd(opts Options) (Manager, error) {
	dir := opts.DefinitionDir
	if dir == "" {
		home, err := paths.HomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".config", "systemd", "user")
	}
	return &systemd{opts: opts, unit: filepath.Join(dir, UnitName)}, nil
}

// env is the environment systemctl --user needs to find the user's manager. The plugin's own
// environment is empty, so it is built from the user id.
func (s *systemd) env() []string {
	runtimeDir := "/run/user/" + strconv.Itoa(s.opts.UID)
	return []string{
		"XDG_RUNTIME_DIR=" + runtimeDir,
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + runtimeDir + "/bus",
	}
}

func (s *systemd) run(ctx context.Context, args ...string) (Output, error) {
	exe, ok := s.opts.FindExecutable(systemctlCandidates)
	if !ok {
		return Output{}, fmt.Errorf("%w: looked in %s", ErrNoSystemctl, strings.Join(systemctlCandidates, ", "))
	}
	return s.opts.Runner.Run(ctx, Command{Name: exe, Args: append([]string{"--user"}, args...), Env: s.env()})
}

// must runs a systemctl command that has to succeed.
func (s *systemd) must(ctx context.Context, args ...string) error {
	out, err := s.run(ctx, args...)
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return failure("systemctl --user "+args[0], out)
	}
	return nil
}

func (s *systemd) installed() bool {
	_, err := os.Stat(s.unit)
	return err == nil
}

// Install writes the unit, enables it and restarts it so a new executable takes effect.
func (s *systemd) Install(ctx context.Context, spec Spec) error {
	if err := spec.validate(); err != nil {
		return err
	}
	if err := s.opts.Paths.EnsureDirs(); err != nil {
		return err
	}
	if err := writeFileAtomic(s.unit, renderUnit(spec, s.opts.Paths.ServiceLogFile()), unitMode); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", UnitName}, {"restart", UnitName}} {
		if err := s.must(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// Start starts an installed unit.
func (s *systemd) Start(ctx context.Context) error {
	if !s.installed() {
		return fmt.Errorf("%w: %s", ErrNotInstalled, s.unit)
	}
	return s.must(ctx, "start", UnitName)
}

// Stop stops the unit. A unit that was never installed is already stopped.
func (s *systemd) Stop(ctx context.Context) error {
	if !s.installed() {
		return nil
	}
	return s.must(ctx, "stop", UnitName)
}

// Uninstall disables and stops the unit, then deletes it.
func (s *systemd) Uninstall(ctx context.Context) error {
	if !s.installed() {
		return nil
	}
	if err := s.must(ctx, "disable", "--now", UnitName); err != nil {
		return err
	}
	if err := os.Remove(s.unit); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", s.unit, err)
	}
	return s.must(ctx, "daemon-reload")
}

// Status reports the unit's state.
func (s *systemd) Status(ctx context.Context) (Status, error) {
	st := Status{DefinitionPath: s.unit, LogFile: s.opts.Paths.ServiceLogFile(), Installed: s.installed()}
	if !st.Installed {
		return st, nil
	}
	if data, err := os.ReadFile(s.unit); err == nil {
		st.Executable = programFromUnit(data)
	}
	out, err := s.run(ctx, "show", UnitName, "--property=LoadState,ActiveState,MainPID")
	if err != nil {
		return st, err
	}
	if out.ExitCode != 0 {
		return st, failure("systemctl --user show", out)
	}
	props := parseProperties(out.Stdout)
	st.Loaded = props["LoadState"] == "loaded"
	st.PID, _ = strconv.Atoi(props["MainPID"])
	st.Running = props["ActiveState"] == "active" && st.PID > 0
	if !st.Running {
		st.PID = 0
	}
	fillVersions(&st, s.opts.Paths)
	return st, nil
}

// parseProperties reads systemctl show's Key=Value lines.
func parseProperties(s string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[k] = v
		}
	}
	return props
}

// programFromUnit returns the executable of the first ExecStart line of a unit this package wrote.
func programFromUnit(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		rest, ok := strings.CutPrefix(line, "ExecStart=")
		if !ok {
			continue
		}
		return firstQuoted(rest)
	}
	return ""
}

// firstQuoted decodes the first double-quoted word of an ExecStart value written by quoteArg.
func firstQuoted(s string) string {
	if !strings.HasPrefix(s, `"`) {
		return strings.Fields(s)[0]
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case c == '%' && i+1 < len(s) && s[i+1] == '%', c == '$' && i+1 < len(s) && s[i+1] == '$':
			b.WriteByte(c)
			i++
		case c == '"':
			return b.String()
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// quoteArg quotes one argument for a systemd ExecStart line: double quotes, with backslash and
// quote escaped and the specifier and variable characters (% and $) doubled.
func quoteArg(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`)
	return `"` + r.Replace(s) + `"`
}

// renderUnit produces the systemd user unit.
func renderUnit(spec Spec, logFile string) []byte {
	words := make([]string, 0, 1+len(spec.Args))
	for _, w := range append([]string{spec.Executable}, spec.Args...) {
		words = append(words, quoteArg(w))
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Atmos SSH agent\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=" + strings.Join(words, " ") + "\n")
	b.WriteString("Restart=always\n")
	b.WriteString("RestartSec=2\n")
	b.WriteString("StandardOutput=append:" + logFile + "\n")
	b.WriteString("StandardError=append:" + logFile + "\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=default.target\n")
	return []byte(b.String())
}
