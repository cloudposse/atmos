// Package wiring connects the SSH plugin's logic to the real operating system: the data
// directory, key stores, service manager, clock, and the Atmos API. Everything here is thin;
// the behavior lives in the packages it assembles.
package wiring

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/github"
	"github.com/cloudposse/atmos/plugins/ssh/authority/self"
	"github.com/cloudposse/atmos/plugins/ssh/command"
	"github.com/cloudposse/atmos/plugins/ssh/daemon"
	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/rpc"
	"github.com/cloudposse/atmos/plugins/ssh/service"
	"github.com/cloudposse/atmos/sdk/api"
	"github.com/cloudposse/atmos/sdk/plugin"
)

var registerOnce sync.Once

// RegisterAuthorities adds the built-in authorities to the default registry. Nothing registers
// itself, so a new authority is added here.
func RegisterAuthorities() {
	registerOnce.Do(func() {
		for _, f := range []authority.Factory{github.Factory, self.Factory} {
			if err := authority.Register(f); err != nil && !errors.Is(err, authority.ErrDuplicateAuthority) {
				panic(err)
			}
		}
	})
}

// Services builds the plugin's providers, identity, and command against the real system.
func Services(client *api.Client, version string) plugin.Services {
	RegisterAuthorities()
	d := daemonDeps(version)
	idDeps := identity.Deps{
		Paths: resolvePaths, OpenStore: keystore.New, Authorities: authority.Default, Daemon: d,
		CheckSocket: agent.CheckSocket, HTTPClient: client.HTTPClient(), State: client,
		Now: time.Now, Hostname: os.Hostname, HomeDir: paths.HomeDir, FindSSHKeygen: FindSSHKeygen, Sleep: Sleep,
	}
	cmdDeps := command.Deps{
		Paths: resolvePaths, OpenStore: keystore.New, Daemon: d, CheckSocket: agent.CheckSocket,
		Dial: dial, Now: time.Now, Sleep: Sleep, Hostname: os.Hostname,
	}
	return plugin.Services{
		Provider: &rpc.Provider{Now: time.Now},
		Identity: rpc.NewIdentity(identity.New(idDeps)),
		Commands: rpc.NewCommands(command.New(cmdDeps)),
	}
}

// ServeAgent runs the agent daemon until ctx ends. An empty root means the default data directory.
func ServeAgent(ctx context.Context, root, version string, logf func(string, ...any)) error {
	p, err := resolvePaths(root)
	if err != nil {
		return err
	}
	return agent.Serve(ctx, agent.Options{Paths: p, Version: version, OpenStore: keystore.New, Logf: logf})
}

func daemonDeps(version string) daemon.Deps {
	return daemon.Deps{
		Service:           func(p *paths.Paths) (service.Manager, error) { return service.New(service.Options{Paths: p}) },
		InstallExecutable: service.InstallExecutable,
		Version:           version,
	}
}

func resolvePaths(root string) (*paths.Paths, error) {
	if root == "" {
		return paths.Default()
	}
	return paths.New(root)
}

func dial(ctx context.Context, socket string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", socket)
}

// Sleep waits for d, or until ctx ends.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FindSSHKeygen returns the absolute path of ssh-keygen. The plugin has no PATH, so the usual
// install locations are checked in order.
func FindSSHKeygen() (string, error) { return findSSHKeygen(runtime.GOOS, fileExists) }

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func findSSHKeygen(goos string, exists func(string) bool) (string, error) {
	for _, c := range sshKeygenCandidates(goos) {
		if exists(c) {
			return c, nil
		}
	}
	return "", identity.ErrNoSSHKeygen
}

func sshKeygenCandidates(goos string) []string {
	if goos == "windows" {
		return []string{
			`C:\Windows\System32\OpenSSH\ssh-keygen.exe`,
			`C:\Program Files\Git\usr\bin\ssh-keygen.exe`,
		}
	}
	return []string{"/usr/bin/ssh-keygen", "/usr/local/bin/ssh-keygen", "/opt/homebrew/bin/ssh-keygen", "/bin/ssh-keygen"}
}
