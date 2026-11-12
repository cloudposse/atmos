package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

const (
	// Label is the launchd label of the agent service.
	Label = "com.cloudposse.atmos.ssh-agent"
	// LaunchctlPath is where launchctl lives. The plugin has no PATH, so it is used as is.
	LaunchctlPath = "/bin/launchctl"

	plistMode = os.FileMode(0o644)
	// unloadWait bounds how long Install waits for launchd to finish a bootout.
	unloadAttempts = 30
	unloadPause    = 100 * time.Millisecond
)

var (
	pidRE     = regexp.MustCompile(`(?m)^\s*pid = (\d+)\s*$`)
	programRE = regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>\s*<string>([^<]*)</string>`)
)

// launchd drives a LaunchAgent with launchctl.
type launchd struct {
	opts  Options
	plist string
	pause func(context.Context, time.Duration) error
}

func newLaunchd(opts Options) (Manager, error) {
	dir := opts.DefinitionDir
	if dir == "" {
		home, err := paths.HomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, "Library", "LaunchAgents")
	}
	return &launchd{opts: opts, plist: filepath.Join(dir, Label+".plist"), pause: sleepContext}, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (l *launchd) domain() string { return "gui/" + strconv.Itoa(l.opts.UID) }
func (l *launchd) target() string { return l.domain() + "/" + Label }

func (l *launchd) run(ctx context.Context, args ...string) (Output, error) {
	return l.opts.Runner.Run(ctx, Command{Name: LaunchctlPath, Args: args, Env: []string{}})
}

// loaded reports whether launchd knows the service, and returns the print output.
func (l *launchd) loaded(ctx context.Context) (bool, Output, error) {
	out, err := l.run(ctx, "print", l.target())
	if err != nil {
		return false, out, err
	}
	return out.ExitCode == 0, out, nil
}

func (l *launchd) installed() bool {
	_, err := os.Stat(l.plist)
	return err == nil
}

// Install writes the plist and (re)loads the service, which starts it.
func (l *launchd) Install(ctx context.Context, spec Spec) error {
	if err := spec.validate(); err != nil {
		return err
	}
	if err := l.opts.Paths.EnsureDirs(); err != nil {
		return err
	}
	data := renderPlist(spec, l.opts.Paths.ServiceLogFile())
	if err := writeFileAtomic(l.plist, data, plistMode); err != nil {
		return err
	}
	if err := l.unload(ctx); err != nil {
		return err
	}
	return l.bootstrap(ctx)
}

func (l *launchd) bootstrap(ctx context.Context) error {
	out, err := l.run(ctx, "bootstrap", l.domain(), l.plist)
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return failure("launchctl bootstrap", out)
	}
	return nil
}

// unload boots the service out when it is loaded and waits until launchd has let go of it.
func (l *launchd) unload(ctx context.Context) error {
	ok, _, err := l.loaded(ctx)
	if err != nil || !ok {
		return err
	}
	out, err := l.run(ctx, "bootout", l.target())
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return failure("launchctl bootout", out)
	}
	for range unloadAttempts {
		ok, _, err := l.loaded(ctx)
		if err != nil || !ok {
			return err
		}
		if err := l.pause(ctx, unloadPause); err != nil {
			return err
		}
	}
	return fmt.Errorf("%w: launchd still has %s loaded after bootout", ErrCommandFailed, Label)
}

// Start loads the service if it is not loaded.
func (l *launchd) Start(ctx context.Context) error {
	if !l.installed() {
		return fmt.Errorf("%w: %s", ErrNotInstalled, l.plist)
	}
	ok, _, err := l.loaded(ctx)
	if err != nil || ok {
		return err
	}
	return l.bootstrap(ctx)
}

// Stop boots the service out. With KeepAlive set, that is the only way to keep it down.
func (l *launchd) Stop(ctx context.Context) error { return l.unload(ctx) }

// Uninstall boots the service out and deletes the plist.
func (l *launchd) Uninstall(ctx context.Context) error {
	if err := l.unload(ctx); err != nil {
		return err
	}
	if err := os.Remove(l.plist); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", l.plist, err)
	}
	return nil
}

// Status reports the service's state.
func (l *launchd) Status(ctx context.Context) (Status, error) {
	st := Status{DefinitionPath: l.plist, LogFile: l.opts.Paths.ServiceLogFile(), Installed: l.installed()}
	if st.Installed {
		if data, err := os.ReadFile(l.plist); err == nil {
			st.Executable = programFromPlist(data)
		}
	}
	ok, out, err := l.loaded(ctx)
	if err != nil {
		return st, err
	}
	st.Loaded = ok
	if ok {
		if m := pidRE.FindStringSubmatch(out.Stdout); m != nil {
			st.PID, _ = strconv.Atoi(m[1])
		}
		st.Running = st.PID > 0
	}
	fillVersions(&st, l.opts.Paths)
	return st, nil
}

// programFromPlist returns the first ProgramArguments entry of a plist this package wrote.
func programFromPlist(data []byte) string {
	m := programRE.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return html.UnescapeString(string(m[1]))
}

// renderPlist produces the LaunchAgent property list.
func renderPlist(spec Spec, logFile string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, arg := range append([]string{spec.Executable}, spec.Args...) {
		b.WriteString("\t\t<string>" + xmlEscape(arg) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>` + xmlEscape(logFile) + `</string>
	<key>StandardErrorPath</key>
	<string>` + xmlEscape(logFile) + `</string>
</dict>
</plist>
`)
	return b.Bytes()
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
