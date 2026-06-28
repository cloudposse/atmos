package emulator

//go:generate go run go.uber.org/mock/mockgen@latest -destination=mock_runtime_test.go -package=emulator github.com/cloudposse/atmos/pkg/container Runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultProtocol = "tcp"

// Manager operates emulator containers over the container component lifecycle,
// using ComponentType "emulator" so discovery, labels, and naming are shared with
// the container kind. It does not process stacks; callers pass a resolved Spec.
type Manager struct {
	runtime     container.Runtime // injected for tests; detected per call when nil.
	runtimePref string
	autoStart   bool
}

// NewManager returns a Manager that detects the container runtime using the given
// preference ("docker"|"podman"|"") and Podman auto-start setting.
func NewManager(runtimePref string, autoStart bool) *Manager {
	defer perf.Track(nil, "emulator.NewManager")()

	return &Manager{runtimePref: runtimePref, autoStart: autoStart}
}

// newManagerWithRuntime injects an explicit runtime (used in tests).
func newManagerWithRuntime(runtime container.Runtime) *Manager {
	return &Manager{runtime: runtime}
}

func (m *Manager) runtimeFor(ctx context.Context) (container.Runtime, error) {
	if m.runtime != nil {
		return m.runtime, nil
	}
	return container.DetectRuntimeWithPreferenceAndRecovery(ctx, m.runtimePref, m.autoStart)
}

// Up reconciles the emulator's long-lived container and returns its live endpoint.
// Host ports are auto-assigned (0) unless pinned, so concurrent emulators do not
// collide; the live ports are read back from the runtime.
func (m *Manager) Up(ctx context.Context, spec *Spec, stack, name string, env map[string]string) (Endpoint, error) {
	defer perf.Track(nil, "emulator.Manager.Up")()

	runtime, err := m.runtimeFor(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	// Detect a rootless runtime so drivers that need rootless accommodations
	// (e.g. k3s) can swap in their rootless command. Rootful is the default.
	rootless := container.RuntimeIsRootless(ctx, runtime)
	namedConfig, err := m.namedConfig(spec, stack, name, env, rootless)
	if err != nil {
		return Endpoint{}, err
	}
	// Join the per-stack shared network so emulators can resolve each other by
	// component name (e.g. a GitOps controller in k3s reaching the Gitea emulator).
	m.attachSharedNetwork(ctx, runtime, namedConfig, stack, name)
	if _, err := container.UpWithRuntime(ctx, runtime, namedConfig); err != nil {
		return Endpoint{}, err
	}
	// Gate readiness on the container health check (component or driver default) so
	// callers — and any dependent Terraform — never hit a not-yet-ready emulator.
	if err := m.waitHealthyIfNeeded(ctx, runtime, spec, stack, name); err != nil {
		return Endpoint{}, err
	}
	// A file-backed Vault/OpenBao server boots sealed and uninitialized; initialize,
	// unseal, and enable KV v2 (or re-unseal from the persisted bootstrap) before the
	// endpoint is considered ready.
	if err := m.bootstrapVaultIfNeeded(ctx, runtime, spec, stack, name); err != nil {
		return Endpoint{}, err
	}
	// A fresh Gitea server boots installed-but-empty; create the admin user and the
	// deployment repository so the GitOps loop has something to push to and clone.
	if err := m.bootstrapGitIfNeeded(ctx, runtime, spec, stack, name); err != nil {
		return Endpoint{}, err
	}
	return m.endpoint(ctx, runtime, spec, stack, name)
}

// waitHealthyIfNeeded blocks until the emulator container reports healthy when a
// health check applies (the component's `container.healthcheck` or the driver
// default). It is a no-op when none applies — e.g. vault, whose readiness is its
// bootstrap — because a container without a health check never reports a health
// state and would otherwise time out.
func (m *Manager) waitHealthyIfNeeded(ctx context.Context, runtime container.Runtime, spec *Spec, stack, name string) error {
	defer perf.Track(nil, "emulator.Manager.waitHealthyIfNeeded")()

	hc, err := spec.EffectiveHealthCheck()
	if err != nil {
		return err
	}
	if hc == nil || hc.Disable {
		return nil
	}
	inst := container.Instance{Stack: stack, ComponentType: cfg.EmulatorComponentType, Component: name}
	return container.WaitHealthy(ctx, runtime, inst, container.DefaultHealthyTimeout)
}

// bootstrapVaultIfNeeded runs the Vault/OpenBao file-backend bootstrap when the
// spec's target is vault; it is a no-op for every other target.
func (m *Manager) bootstrapVaultIfNeeded(ctx context.Context, runtime container.Runtime, spec *Spec, stack, name string) error {
	target, err := spec.Target()
	if err != nil {
		return err
	}
	if target != TargetVault {
		return nil
	}
	info, found, err := container.FindInstance(ctx, runtime, stack, cfg.EmulatorComponentType, name)
	if err != nil {
		return err
	}
	if !found || !container.IsContainerRunning(info.Status) {
		return fmt.Errorf("%w: %s/emulator/%s did not start", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	return bootstrapVault(ctx, runtime, info.ID)
}

// bootstrapGitIfNeeded creates the Gitea admin user and deployment repository when
// the spec's target is git; it is a no-op for every other target. The repository
// is created over the live HTTP endpoint, so the host port is read back from the
// running container the same way endpoint() does.
func (m *Manager) bootstrapGitIfNeeded(ctx context.Context, runtime container.Runtime, spec *Spec, stack, name string) error {
	target, err := spec.Target()
	if err != nil {
		return err
	}
	if target != TargetGit {
		return nil
	}
	info, found, err := container.FindInstance(ctx, runtime, stack, cfg.EmulatorComponentType, name)
	if err != nil {
		return err
	}
	if !found || !container.IsContainerRunning(info.Status) {
		return fmt.Errorf("%w: %s/emulator/%s did not start", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	endpoint, err := m.endpoint(ctx, runtime, spec, stack, name)
	if err != nil {
		return err
	}
	baseURL := endpoint.URL("http")
	if baseURL == "" {
		return fmt.Errorf("%w: %s/emulator/%s has no published HTTP port", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	return bootstrapGitea(ctx, runtime, info.ID, baseURL)
}

func (m *Manager) namedConfig(spec *Spec, stack, name string, env map[string]string, rootless bool) (*container.NamedConfig, error) {
	image, err := spec.Image()
	if err != nil {
		return nil, err
	}
	ports, err := spec.ContainerPorts()
	if err != nil {
		return nil, err
	}
	privileged, err := spec.Privileged()
	if err != nil {
		return nil, err
	}
	defaultEnv, err := spec.DefaultEnv()
	if err != nil {
		return nil, err
	}
	command, err := spec.DefaultCommand()
	if err != nil {
		return nil, err
	}
	runArgs, command, err := m.resolveRootlessRun(spec, command, rootless)
	if err != nil {
		return nil, err
	}
	// Assemble the user's explicit mounts plus the auto-injected persistence bind
	// mount (host XDG cache dir -> driver data dir) so emulator state survives
	// `down`/`up` unless the instance is ephemeral.
	mounts, err := resolveMounts(spec, stack, name)
	if err != nil {
		return nil, err
	}
	// Restart policy and health check come from `container.restart`/`.healthcheck`
	// when set, otherwise the driver default — reusing the container kind's mappers
	// so both translate identically.
	healthCheck, err := spec.EffectiveHealthCheck()
	if err != nil {
		return nil, err
	}
	restart, err := spec.EffectiveRestart()
	if err != nil {
		return nil, err
	}
	return &container.NamedConfig{
		Stack:            stack,
		ComponentType:    cfg.EmulatorComponentType,
		Component:        name,
		Image:            image,
		Command:          command,
		RunArgs:          runArgs,
		Ports:            portBindings(ports),
		Env:              mergeEnv(defaultEnv, env),
		Mounts:           mounts,
		Privileged:       privileged,
		Host:             spec.HostRuntime(),
		Restart:          restart,
		HealthCheck:      healthCheck,
		PullPolicy:       container.PullMissing,
		RuntimeName:      m.runtimePref,
		RuntimeAutoStart: m.autoStart,
	}, nil
}

// emulatorNetworkName is the per-stack user network emulators join so containers
// in the same stack resolve each other by component name (container DNS). Derived
// from the stack alone and sanitized to a valid network name.
func emulatorNetworkName(stack string) string {
	return "atmos-emulator-" + sanitizeNetworkToken(stack)
}

// sanitizeNetworkToken reduces a stack name to characters valid in a docker/podman
// network name ([a-zA-Z0-9_.-]); any other rune becomes '-'.
func sanitizeNetworkToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

// attachSharedNetwork best-effort joins the emulator container to the per-stack
// shared network with its component name as a network alias, so peers resolve it
// by name. It is a no-op when the runtime cannot create networks (e.g. a test
// mock) or network creation fails — single-emulator use still works over the
// default bridge, only cross-container name resolution is lost.
func (m *Manager) attachSharedNetwork(ctx context.Context, runtime container.Runtime, namedConfig *container.NamedConfig, stack, name string) {
	defer perf.Track(nil, "emulator.Manager.attachSharedNetwork")()

	ensurer, ok := runtime.(container.NetworkEnsurer)
	if !ok {
		return
	}
	network := emulatorNetworkName(stack)
	if err := ensurer.EnsureNetwork(ctx, network); err != nil {
		log.Debug("emulator shared network unavailable; containers will not resolve each other by name",
			"network", network, "error", err)
		return
	}
	namedConfig.RunArgs = append(namedConfig.RunArgs, "--network", network, "--network-alias", name)
}

// resolveRootlessRun applies the driver's rootless override under a rootless
// runtime: it swaps in the rootless run-args and command (e.g. k3s's
// cgroup-nesting entrypoint). Rootful keeps the defaults, returning the command
// unchanged and no run-args.
func (m *Manager) resolveRootlessRun(spec *Spec, command []string, rootless bool) (runArgs, cmd []string, err error) {
	defer perf.Track(nil, "emulator.Manager.resolveRootlessRun")()

	if !rootless {
		return nil, command, nil
	}
	rc, err := spec.RootlessOverride()
	if err != nil {
		return nil, nil, err
	}
	if rc.Applies {
		return rc.RunArgs, rc.Command, nil
	}
	return nil, command, nil
}

// mergeEnv layers the component/profile env over the driver defaults so an explicit
// value wins, while driver-required vars (e.g. K3S_TOKEN) remain present.
func mergeEnv(defaults, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(defaults)+len(overrides))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return merged
}

// portBindings converts the spec's container ports into runtime port bindings,
// defaulting the protocol and leaving the host port 0 (auto-assigned) unless pinned.
func portBindings(ports []schema.ContainerPort) []container.PortBinding {
	bindings := make([]container.PortBinding, 0, len(ports))
	for _, port := range ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = defaultProtocol
		}
		bindings = append(bindings, container.PortBinding{
			HostPort:      port.Host, // 0 -> auto-assigned.
			ContainerPort: port.Container,
			Protocol:      protocol,
		})
	}
	return bindings
}

// Resolve discovers the running emulator and builds its connection profile. It is
// the seam consumed by auth identities and the !emulator YAML function.
func (m *Manager) Resolve(ctx context.Context, spec *Spec, stack, name string) (Endpoint, Profile, error) {
	defer perf.Track(nil, "emulator.Manager.Resolve")()

	runtime, err := m.runtimeFor(ctx)
	if err != nil {
		return Endpoint{}, Profile{}, err
	}
	endpoint, err := m.endpoint(ctx, runtime, spec, stack, name)
	if err != nil {
		return Endpoint{}, Profile{}, err
	}
	driver, err := spec.ResolvedDriver()
	if err != nil {
		return Endpoint{}, Profile{}, err
	}
	profile := driver.Profile(&endpoint)

	// Kubernetes profiles are harvested from the running container (the kubeconfig IS
	// the credential) rather than built from the endpoint, so the driver's Profile leaves
	// Kubeconfig empty. Populate it here so every caller consumes Profile.Kubeconfig
	// uniformly without branching on the target.
	if driver.Target() == TargetKubernetes && len(profile.Kubeconfig) == 0 {
		kubeconfig, kErr := m.Kubeconfig(ctx, stack, name)
		if kErr != nil {
			return Endpoint{}, Profile{}, kErr
		}
		profile.Kubeconfig = kubeconfig
	}

	// The Vault/OpenBao root token is dynamic (the file-backed server generates it at
	// init), so it is harvested from the running container rather than built from the
	// endpoint. Add it here so every caller consumes a complete profile.
	if driver.Target() == TargetVault {
		token, vErr := m.VaultToken(ctx, stack, name)
		if vErr != nil {
			return Endpoint{}, Profile{}, vErr
		}
		if profile.Env == nil {
			profile.Env = map[string]string{}
		}
		profile.Env["VAULT_TOKEN"] = token
		profile.Env["BAO_TOKEN"] = token
	}

	return endpoint, profile, nil
}

// VaultToken harvests the dynamic root token from a running Vault/OpenBao emulator
// (the file-backed server records it in a bootstrap file under its data dir).
func (m *Manager) VaultToken(ctx context.Context, stack, name string) (string, error) {
	defer perf.Track(nil, "emulator.Manager.VaultToken")()

	runtime, info, err := m.find(ctx, stack, name)
	if err != nil {
		return "", err
	}
	return vaultRootToken(ctx, runtime, info.ID)
}

// endpoint discovers the running container by label and reads its live host ports.
func (m *Manager) endpoint(ctx context.Context, runtime container.Runtime, spec *Spec, stack, name string) (Endpoint, error) {
	info, found, err := container.FindInstance(ctx, runtime, stack, cfg.EmulatorComponentType, name)
	if err != nil {
		return Endpoint{}, err
	}
	if !found || !container.IsContainerRunning(info.Status) {
		return Endpoint{}, fmt.Errorf("%w: %s/emulator/%s (start it with `atmos emulator up %s -s %s`)",
			errUtils.ErrEmulatorNotRunning, stack, name, name, stack)
	}
	target, err := spec.Target()
	if err != nil {
		return Endpoint{}, err
	}
	ports := make(map[int]int, len(info.Ports))
	for _, binding := range info.Ports {
		if binding.HostPort != 0 {
			ports[binding.ContainerPort] = binding.HostPort
		}
	}
	return Endpoint{
		Target:   target,
		Host:     "localhost",
		Ports:    ports,
		Region:   spec.Region,
		Project:  spec.Project,
		Services: spec.Services,
	}, nil
}

// Down stops and removes the emulator's container.
func (m *Manager) Down(ctx context.Context, stack, name string) error {
	defer perf.Track(nil, "emulator.Manager.Down")()

	runtime, err := m.runtimeFor(ctx)
	if err != nil {
		return err
	}
	return container.Down(ctx, runtime, stack, cfg.EmulatorComponentType, name)
}

// Reset stops and removes the emulator's container, then wipes its persisted
// state directory under the XDG cache. The next `up` starts a fresh instance.
// The data directory is derived from stack+name alone, so reset works without
// resolving the driver or a running container.
func (m *Manager) Reset(ctx context.Context, spec *Spec, stack, name string) error {
	defer perf.Track(nil, "emulator.Manager.Reset")()

	// While the container is still running, delete its persisted state from inside
	// (as the container's user, typically root). On a rootful runtime (e.g. Docker in
	// CI) the container writes root-owned files into the bind-mounted host directory
	// that the host process cannot remove; deleting them in-container first lets the
	// host wipe the now-empty directory below. Best effort — rootless runtimes map
	// container-root to the host user, where os.RemoveAll already suffices.
	m.wipePersistedStateInContainer(ctx, spec, stack, name)

	if err := m.Down(ctx, stack, name); err != nil {
		return err
	}
	dataDir := LookupInstanceDataDir(stack, name)
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("%w: removing persisted state %q: %w", errUtils.ErrEmulatorResetFailed, dataDir, err)
	}
	return nil
}

// wipePersistedStateInContainer deletes the contents of the emulator's in-container
// data directory from inside the running container, so root-owned files written by a
// rootful runtime are removed before the host wipes the (now-empty) bind-mount
// directory. It is best effort: any failure (no persistence, container not running,
// or an image without a shell) leaves the host-side os.RemoveAll in Reset as the
// authority.
func (m *Manager) wipePersistedStateInContainer(ctx context.Context, spec *Spec, stack, name string) {
	if spec == nil || !spec.PersistEnabled() {
		return
	}
	dataDir, err := spec.DataDir()
	if err != nil || dataDir == "" {
		return
	}
	runtime, err := m.runtimeFor(ctx)
	if err != nil {
		return
	}
	info, found, err := container.FindInstance(ctx, runtime, stack, cfg.EmulatorComponentType, name)
	if err != nil || !found || !container.IsContainerRunning(info.Status) {
		return
	}
	// Remove the data directory's contents (regular and dotfile entries) while
	// keeping the mount point itself. Unquoted so the shell expands the globs.
	script := fmt.Sprintf("rm -rf %s/* %s/.[!.]* %s/..?* 2>/dev/null || true", dataDir, dataDir, dataDir)
	_ = runtime.Exec(ctx, info.ID, []string{"sh", "-c", script}, &container.ExecOptions{
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
}

// Logs streams the emulator container's logs to the default data/UI channels.
func (m *Manager) Logs(ctx context.Context, stack, name string, follow bool) error {
	defer perf.Track(nil, "emulator.Manager.Logs")()

	runtime, info, err := m.find(ctx, stack, name)
	if err != nil {
		return err
	}
	return runtime.Logs(ctx, info.ID, follow, "all", nil, nil)
}

// Exec runs a command in the emulator container (interactive; defaults to a shell).
func (m *Manager) Exec(ctx context.Context, stack, name string, command []string) error {
	defer perf.Track(nil, "emulator.Manager.Exec")()

	runtime, info, err := m.find(ctx, stack, name)
	if err != nil {
		return err
	}
	if len(command) == 0 {
		command = []string{"/bin/sh"}
	}
	return runtime.Exec(ctx, info.ID, command, &container.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	})
}

// Status is one row of `atmos emulator ps`.
type Status struct {
	Name   string
	Image  string
	Status string
	ID     string
}

// Ps lists emulator containers in a stack (by canonical labels).
func (m *Manager) Ps(ctx context.Context, stack string) ([]Status, error) {
	defer perf.Track(nil, "emulator.Manager.Ps")()

	runtime, err := m.runtimeFor(ctx)
	if err != nil {
		return nil, err
	}
	filter := map[string]string{
		"label": fmt.Sprintf("%s=%s", container.LabelComponentType, cfg.EmulatorComponentType),
	}
	infos, err := runtime.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	statuses := make([]Status, 0, len(infos))
	for i := range infos {
		if infos[i].Labels[container.LabelStack] != stack {
			continue
		}
		statuses = append(statuses, Status{
			Name:   infos[i].Labels[container.LabelComponent],
			Image:  infos[i].Image,
			Status: infos[i].Status,
			ID:     infos[i].ID,
		})
	}
	return statuses, nil
}

// find resolves the runtime and the running container for an emulator instance.
func (m *Manager) find(ctx context.Context, stack, name string) (container.Runtime, *container.Info, error) {
	runtime, err := m.runtimeFor(ctx)
	if err != nil {
		return nil, nil, err
	}
	info, found, err := container.FindInstance(ctx, runtime, stack, cfg.EmulatorComponentType, name)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, fmt.Errorf("%w: %s/emulator/%s", errUtils.ErrEmulatorNotRunning, stack, name)
	}
	return runtime, info, nil
}
