package plugin

import (
	"bytes"
	"context"
	"os"
	execpkg "os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/ui"
)

// managedDirName is the sub-directory of the toolchain install path where Atmos
// installs Helm plugins. Helm is pointed at this directory via HELM_PLUGINS so
// plugin installs never touch the user's global Helm configuration.
const managedDirName = "helm-plugins"

// managedDirPerm is the permission for the managed HELM_PLUGINS directory.
const managedDirPerm = 0o755

// InstalledPlugin describes a plugin reported by `helm plugin list`.
type InstalledPlugin struct {
	Name    string
	Version string
}

// Runner abstracts subprocess execution so the installer can be unit-tested
// without a real helm binary.
type Runner interface {
	// Run executes name with args. extraEnv entries (KEY=VALUE) are appended to
	// the current environment. It returns combined stdout and stderr separately.
	Run(ctx context.Context, name string, args, extraEnv []string) (stdout, stderr string, err error)
}

// execRunner is the production Runner backed by os/exec.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args, extraEnv []string) (string, string, error) {
	defer perf.Track(nil, "plugin.execRunner.Run")()

	cmd := execpkg.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Installer installs and inspects Helm plugins in an Atmos-managed directory.
type Installer struct {
	helmBin string
	dir     string
	runner  Runner
}

// Option configures an Installer.
type Option func(*Installer)

// WithRunner overrides the subprocess runner (used in tests).
func WithRunner(r Runner) Option {
	defer perf.Track(nil, "plugin.WithRunner")()

	return func(i *Installer) { i.runner = r }
}

// WithDir overrides the managed plugins directory (used in tests).
func WithDir(dir string) Option {
	defer perf.Track(nil, "plugin.WithDir")()

	return func(i *Installer) { i.dir = dir }
}

// ManagedDir returns the default managed HELM_PLUGINS directory, derived from
// the toolchain install path.
//
// The path is resolved to absolute: helmfile runs helm from the component's
// working directory, so a HELM_PLUGINS value relative to the Atmos base
// directory (e.g. a configured `.tools`) would not resolve there and helm would
// fail to find installed plugins like helm-diff. This mirrors how toolchain
// binary PATH entries are absolutized (see pkg/dependencies/environment.go).
func ManagedDir() string {
	defer perf.Track(nil, "plugin.ManagedDir")()

	dir := filepath.Join(toolchain.GetInstallPath(), managedDirName)
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// NewInstaller creates an Installer for the given helm binary path.
func NewInstaller(helmBin string, opts ...Option) *Installer {
	defer perf.Track(nil, "plugin.NewInstaller")()

	i := &Installer{
		helmBin: helmBin,
		dir:     ManagedDir(),
		runner:  execRunner{},
	}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// EnsurePlugins makes sure every spec is installed at the requested version in
// the managed directory and returns that directory (suitable for HELM_PLUGINS).
//
// Already-satisfied plugins are skipped. A plugin present at a different pinned
// version is reinstalled to honor the declared version.
func (i *Installer) EnsurePlugins(ctx context.Context, specs []Spec) (string, error) {
	defer perf.Track(nil, "plugin.Installer.EnsurePlugins")()

	if len(specs) == 0 {
		return i.dir, nil
	}

	if i.helmBin == "" {
		return "", errUtils.Build(errUtils.ErrHelmBinaryNotFound).
			WithExplanation("Cannot install helm plugins without a helm binary").
			WithHint("Declare `helm` in the component `dependencies.tools` so Atmos installs it, or ensure helm is on PATH").
			Err()
	}

	if err := os.MkdirAll(i.dir, managedDirPerm); err != nil {
		return "", errUtils.Build(errUtils.ErrHelmPluginInstall).
			WithCause(err).
			WithExplanationf("Failed to create managed helm plugins directory %q", i.dir).
			Err()
	}

	installed, err := i.ListInstalled(ctx)
	if err != nil {
		return "", err
	}

	for _, spec := range specs {
		if err := i.ensureOne(ctx, spec, installed); err != nil {
			return "", err
		}
	}

	return i.dir, nil
}

// ensureOne installs or upgrades a single plugin as needed.
func (i *Installer) ensureOne(ctx context.Context, spec Spec, installed map[string]string) error {
	defer perf.Track(nil, "plugin.Installer.ensureOne")()

	name, curVersion, present := matchInstalled(spec, installed)
	switch {
	case present && (spec.IsLatest() || versionsEqual(curVersion, spec.Version)):
		log.Debug("Helm plugin already installed", "plugin", spec.Name, "version", curVersion)
		return nil
	case present:
		// Pinned version differs: reinstall to honor the declared version.
		ui.Info("Updating helm plugin `" + spec.Name + "` to " + spec.Version)
		if err := i.uninstall(ctx, name); err != nil {
			return err
		}
	default:
		ui.Info("Installing helm plugin `" + spec.Name + "`")
	}

	return i.install(ctx, spec)
}

// install runs `helm plugin install <url> [--version <tag>]`.
func (i *Installer) install(ctx context.Context, spec Spec) error {
	defer perf.Track(nil, "plugin.Installer.install")()

	args := []string{"plugin", "install", spec.URL}
	if !spec.IsLatest() {
		args = append(args, "--version", spec.Version)
	}

	_, stderr, err := i.runner.Run(ctx, i.helmBin, args, i.env())
	if err != nil {
		return errUtils.Build(errUtils.ErrHelmPluginInstall).
			WithCause(err).
			WithExplanationf("Failed to install helm plugin %q from %s", spec.Name, spec.URL).
			WithHintf("helm reported: %s", strings.TrimSpace(stderr)).
			Err()
	}
	return nil
}

// uninstall runs `helm plugin uninstall <name>`.
func (i *Installer) uninstall(ctx context.Context, name string) error {
	defer perf.Track(nil, "plugin.Installer.uninstall")()

	_, stderr, err := i.runner.Run(ctx, i.helmBin, []string{"plugin", "uninstall", name}, i.env())
	if err != nil {
		return errUtils.Build(errUtils.ErrHelmPluginInstall).
			WithCause(err).
			WithExplanationf("Failed to uninstall helm plugin %q before reinstalling", name).
			WithHintf("helm reported: %s", strings.TrimSpace(stderr)).
			Err()
	}
	return nil
}

// ListInstalled returns the plugins currently installed in the managed
// directory, keyed by plugin name.
func (i *Installer) ListInstalled(ctx context.Context) (map[string]string, error) {
	defer perf.Track(nil, "plugin.Installer.ListInstalled")()

	stdout, stderr, err := i.runner.Run(ctx, i.helmBin, []string{"plugin", "list"}, i.env())
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrHelmPluginInstall).
			WithCause(err).
			WithExplanation("Failed to list installed helm plugins").
			WithHintf("helm reported: %s", strings.TrimSpace(stderr)).
			Err()
	}
	return parsePluginList(stdout), nil
}

// env returns the environment overrides for helm subprocess invocations,
// pointing helm at the managed plugins directory.
func (i *Installer) env() []string {
	return []string{"HELM_PLUGINS=" + i.dir}
}

// matchInstalled checks whether any of a spec's candidate names is present in
// the installed set. Returns the matched name, its version, and whether found.
func matchInstalled(spec Spec, installed map[string]string) (string, string, bool) {
	for _, name := range spec.candidateNames() {
		if v, ok := installed[name]; ok {
			return name, v, true
		}
	}
	return "", "", false
}

// versionsEqual compares two version strings ignoring a leading "v".
func versionsEqual(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// parsePluginList parses the tabular output of `helm plugin list` into a
// name->version map. The expected format is a header row followed by rows of
// "NAME<whitespace>VERSION<whitespace>DESCRIPTION".
func parsePluginList(out string) map[string]string {
	result := map[string]string{}
	for idx, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Skip the header row.
		if idx == 0 && strings.EqualFold(fields[0], "NAME") {
			continue
		}
		result[fields[0]] = fields[1]
	}
	return result
}
