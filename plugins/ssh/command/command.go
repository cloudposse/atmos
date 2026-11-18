// Package command implements the "ssh" plugin command: key and agent management and the
// manual acceptance test.
//
// Commands run with HTTP only: they have no private state, no credentials, and no way to prompt.
// So they never call an authority. Every change at an authority goes through login or logout.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// ErrRefused means a command declined to act because it was not confirmed.
var ErrRefused = errors.New("refused")

// Deps are the outside dependencies of the commands.
type Deps struct {
	// Paths returns the data root; an empty root means the default location.
	Paths func(root string) (*paths.Paths, error)
	// OpenStore builds a key store.
	OpenStore agent.StoreOpener
	// Daemon starts and inspects the agent service.
	Daemon daemon.Deps
	// CheckSocket verifies that a socket answers.
	CheckSocket func(ctx context.Context, socket string, want ssh.PublicKey) error
	// Dial connects to an agent socket.
	Dial func(ctx context.Context, socket string) (net.Conn, error)
	// Now is the clock.
	Now func() time.Time
	// Sleep waits for d, or until ctx ends.
	Sleep func(ctx context.Context, d time.Duration) error
	// Hostname returns the machine's name, for key titles.
	Hostname func() (string, error)
}

// Service runs the commands.
type Service struct {
	deps Deps
}

// New returns the command service.
func New(deps Deps) *Service { return &Service{deps: deps} }

// call is one command invocation.
type call struct {
	svc   *Service
	ctx   context.Context
	out   io.Writer
	err   io.Writer
	flags flags
	paths *paths.Paths
}

type handler func(c *call, words []string) error

// route is one command path and its handler.
type route struct {
	path []string
	run  handler
}

var routes = []route{
	{[]string{"key", "list"}, (*call).keyList},
	{[]string{"key", "show"}, (*call).keyShow},
	{[]string{"key", "status"}, (*call).keyStatus},
	{[]string{"key", "delete"}, (*call).keyDelete},
	{[]string{"agent", "start"}, (*call).agentStart},
	{[]string{"agent", "stop"}, (*call).agentStop},
	{[]string{"agent", "status"}, (*call).agentStatus},
	{[]string{"agent", "log"}, (*call).agentLog},
	{[]string{"test"}, (*call).test},
}

// Run executes the command named by args and returns its exit code. settings is the "config" map
// of the command's package alias; its "root" entry relocates the plugin's data directory.
func (s *Service) Run(ctx context.Context, args []string, settings map[string]any, stdout, stderr io.Writer) int {
	f, err := parseFlags(args)
	if err != nil {
		return report(stderr, err)
	}
	if f.bools["help"] || len(f.words) == 0 || f.words[0] == "help" {
		fmt.Fprint(stdout, usage)
		return ExitOK
	}
	r, rest, ok := lookup(f.words)
	if !ok {
		return report(stderr, usagef("unknown command %q; run \"help\" for the list", strings.Join(f.words, " ")))
	}
	p, err := s.deps.Paths(root(f, settings))
	if err != nil {
		return report(stderr, err)
	}
	c := &call{svc: s, ctx: ctx, out: stdout, err: stderr, flags: f, paths: p}
	if err := r.run(c, rest); err != nil {
		return report(stderr, err)
	}
	return ExitOK
}

// root picks the data root: the --root flag, then the command settings.
func root(f flags, settings map[string]any) string {
	if v, ok := f.values["root"]; ok {
		return v
	}
	v, _ := settings["root"].(string)
	return v
}

func lookup(words []string) (route, []string, bool) {
	for _, r := range routes {
		if len(words) >= len(r.path) && equalWords(words[:len(r.path)], r.path) {
			return r, words[len(r.path):], true
		}
	}
	return route{}, nil, false
}

func equalWords(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// report prints err on w and returns the exit code it deserves.
func report(w io.Writer, err error) int {
	var u *usageError
	if errors.As(err, &u) {
		fmt.Fprintf(w, "Error: %s\n", u.msg)
		return ExitUsage
	}
	fmt.Fprintf(w, "Error: %v\n", err)
	return ExitFailure
}

func (c *call) printf(format string, args ...any) { fmt.Fprintf(c.out, format, args...) }

const usage = `Manage SSH keys and the agent that serves them.

Usage:
  key list                          Keys on this machine, across key stores.
  key show <name> [--format F]      Print a key. F is authorized-key (default),
                                    allowed-signers, github, or json.
  key status [<name>]               Each key's store, requirements, authority,
                                    expiry, and the last login result and error.
  key delete <name> --yes           Delete the local key and its agent entry. The
                                    authority is updated at the next login or logout.
  agent start | stop | status       Run, stop, or inspect the agent service.
  agent log [--lines N]             Print the tail of the audit log.
  test <identity> [--soak D] [--interval D]
                                    Sign a Git payload through the identity's socket
                                    and verify it. --soak repeats it every interval
                                    for D, one line per attempt.
  help                              Print this text.

Flags:
  --root <dir>    Use this data directory instead of the default. The "root" entry
                  of the command's config does the same.
  --store <kind>  Pick the key store when a name exists in more than one.

Keys are created by "atmos auth login". Commands never contact an authority.
`
