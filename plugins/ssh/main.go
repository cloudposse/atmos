// Command plugin is the cloudposse/ssh Atmos plugin. One executable plays two roles:
//
//   - The plugin server, started by Atmos: the ssh/self provider, the ssh/key identity, and the
//     ssh command.
//   - The agent daemon, started by launchd or systemd with the first argument "agent-serve". It
//     never speaks the plugin handshake, and Atmos never starts it.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/wiring"
	"github.com/cloudposse/atmos/sdk/api"
	"github.com/cloudposse/atmos/sdk/plugin"
)

// entry holds the two roles of the executable, so a test can tell which one ran.
type entry struct {
	agent  func(ctx context.Context, root string) error
	plugin func()
	stderr io.Writer
}

func main() {
	os.Exit(dispatch(os.Args[1:], realEntry()))
}

func realEntry() entry {
	version := pluginVersion()
	return entry{
		agent: func(ctx context.Context, root string) error {
			return wiring.ServeAgent(ctx, root, version, func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, format+"\n", args...)
			})
		},
		plugin: func() {
			plugin.Main(plugin.Config{Description: description(), Build: func(client *api.Client) plugin.Services {
				return wiring.Services(client, version)
			}})
		},
		stderr: os.Stderr,
	}
}

// dispatch runs the daemon when the first argument is "agent-serve", and otherwise the plugin
// server. The daemon branch comes first and never reaches plugin.Main: go-plugin exits when the
// magic cookie is missing, and the daemon has none.
func dispatch(args []string, e entry) int {
	if len(args) == 0 || args[0] != daemon.Arg {
		e.plugin()
		return 0
	}
	root, err := parseAgentArgs(args[1:])
	if err != nil {
		fmt.Fprintf(e.stderr, "agent-serve: %v\n", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := e.agent(ctx, root); err != nil {
		fmt.Fprintf(e.stderr, "agent-serve: %v\n", err)
		return 1
	}
	return 0
}

// parseAgentArgs reads the daemon's flags: --root <dir> or --root=<dir>.
func parseAgentArgs(args []string) (string, error) {
	root := ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == daemon.RootFlag && i+1 < len(args):
			i++
			root = args[i]
		case len(a) > len(daemon.RootFlag)+1 && a[:len(daemon.RootFlag)+1] == daemon.RootFlag+"=":
			root = a[len(daemon.RootFlag)+1:]
		default:
			return "", fmt.Errorf("unknown argument %q (usage: agent-serve [--root <dir>])", a)
		}
	}
	return root, nil
}
