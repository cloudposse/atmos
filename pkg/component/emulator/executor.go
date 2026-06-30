package emulator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// Seams for testability — overridden in tests.
var (
	setupComponentAuthForCLI = e.SetupComponentAuthForCLI
	processStacks            = e.ProcessStacks
	initCliConfig            = cfg.InitCliConfig
	// The newManager seam constructs the emulator manager so the
	// container-runtime-backed manager can be replaced with a fake in tests.
	newManager = func(runtimePref string, autoStart bool) emulatorManager {
		return emu.NewManager(runtimePref, autoStart)
	}
)

// emulatorManager is the subset of *emu.Manager the executor, resolver, and
// YAML function consume. Narrowing it to an interface lets tests inject a fake
// without a running container runtime. *emu.Manager satisfies it.
type emulatorManager interface {
	Up(ctx context.Context, spec *emu.Spec, stack, name string, env map[string]string) (emu.Endpoint, error)
	Down(ctx context.Context, stack, name string) error
	Reset(ctx context.Context, spec *emu.Spec, stack, name string) error
	Ps(ctx context.Context, stack string) ([]emu.Status, error)
	Logs(ctx context.Context, stack, name string, follow bool) error
	Exec(ctx context.Context, stack, name string, command []string) error
	Resolve(ctx context.Context, spec *emu.Spec, stack, name string) (emu.Endpoint, emu.Profile, error)
}

// resolved holds the merged, runtime-ready configuration for an emulator
// component instance.
type resolved struct {
	atmosConfig schema.AtmosConfiguration
	spec        emu.Spec
	env         map[string]string
	runtimePref string
	autoStart   bool
	stack       string
	component   string
	dryRun      bool
}

// prepare resolves the component section (templates, YAML functions, secrets,
// optional auth) and projects it onto an emulator Spec.
func prepare(info *schema.ConfigAndStacksInfo) (*resolved, error) {
	defer perf.Track(nil, "componentemulator.prepare")()

	info.ComponentType = cfg.EmulatorComponentType
	atmosConfig, err := initCliConfig(*info, true)
	if err != nil {
		return nil, err
	}

	var authManager auth.AuthManager
	if info.Identity != "" {
		authManager, err = setupComponentAuthForCLI(&atmosConfig, info)
		if err != nil {
			return nil, err
		}
	}

	processedInfo, err := processStacks(&atmosConfig, *info, true, true, true, nil, authManager)
	if err != nil {
		return nil, err
	}
	*info = processedInfo

	if isAbstractSection(info.ComponentSection) {
		return nil, fmt.Errorf("%w: component %q is abstract and cannot be operated directly", errUtils.ErrComponentExecutionFailed, info.ComponentFromArg)
	}

	spec, err := emu.FromComponentSection(info.ComponentSection)
	if err != nil {
		return nil, err
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	env := envListToMap(info.ComponentEnvList)
	return &resolved{
		atmosConfig: atmosConfig,
		spec:        spec,
		env:         env,
		runtimePref: strings.TrimSpace(atmosConfig.Container.Runtime.Provider),
		autoStart:   atmosConfig.Container.Runtime.AutoStart,
		stack:       info.Stack,
		component:   info.ComponentFromArg,
		dryRun:      info.DryRun,
	}, nil
}

func (r *resolved) manager() emulatorManager {
	return newManager(r.runtimePref, r.autoStart)
}

// ExecuteUp starts (or reuses) the emulator's long-running container.
func ExecuteUp(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "componentemulator.ExecuteUp")()

	return executeUp(ctx, info, false)
}

// executeUp starts (or reuses) the emulator's container. When ephemeralOverride
// is true (the `--ephemeral` CLI flag), the instance runs without persistence for
// this `up`, overriding the component's `ephemeral:` config.
func executeUp(ctx context.Context, info *schema.ConfigAndStacksInfo, ephemeralOverride bool) error {
	defer perf.Track(nil, "componentemulator.executeUp")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	if ephemeralOverride {
		ephemeral := true
		r.spec.Ephemeral = &ephemeral
	}
	if r.dryRun {
		ui.Infof("[dry-run] would start emulator %s", r.component)
		return nil
	}
	err = spinner.ExecWithSpinnerDynamic(
		fmt.Sprintf("Starting emulator %s", r.component),
		func() (string, error) {
			endpoint, upErr := r.manager().Up(ctx, &r.spec, r.stack, r.component, r.env)
			if upErr != nil {
				return "", upErr
			}
			if url := endpoint.URL("http"); url != "" {
				return fmt.Sprintf("emulator %s is up at %s", r.component, url), nil
			}
			return fmt.Sprintf("emulator %s is up", r.component), nil
		},
	)
	if err != nil {
		return fmt.Errorf("%w: up %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// ExecuteDown stops and removes the emulator's container.
func ExecuteDown(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "componentemulator.ExecuteDown")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	if r.dryRun {
		ui.Infof("[dry-run] would stop emulator %s", r.component)
		return nil
	}
	if err := spinner.ExecWithSpinner(
		fmt.Sprintf("Stopping emulator %s", r.component),
		fmt.Sprintf("emulator %s is down", r.component),
		func() error { return r.manager().Down(ctx, r.stack, r.component) },
	); err != nil {
		return fmt.Errorf("%w: down %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// confirmReset prompts before wiping persisted state; it is a seam overridden in
// tests so the non-force path can be exercised without a TTY.
var confirmReset = defaultConfirmReset

// ExecuteReset stops and removes the emulator's container and wipes its persisted
// state. Unless force is set, it prompts for confirmation first.
func ExecuteReset(ctx context.Context, info *schema.ConfigAndStacksInfo, force bool) error {
	defer perf.Track(nil, "componentemulator.ExecuteReset")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	if r.dryRun {
		ui.Infof("[dry-run] would reset emulator %s (stop, remove, and wipe persisted state)", r.component)
		return nil
	}
	if !force {
		confirmed, cErr := confirmReset(fmt.Sprintf("Reset emulator %s? This stops the container and deletes its persisted state.", r.component))
		if cErr != nil {
			return cErr
		}
		if !confirmed {
			ui.Warning("reset aborted")
			return nil
		}
	}
	if err := r.manager().Reset(ctx, &r.spec, r.stack, r.component); err != nil {
		return fmt.Errorf("%w: reset %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	ui.Successf("emulator %s reset", r.component)
	return nil
}

// defaultConfirmReset shows a left-aligned Yes/No confirmation dialog.
func defaultConfirmReset(message string) (bool, error) {
	confirm := false
	prompt := uiutils.NewAtmosConfirm().
		Title(message).
		Affirmative("Yes!").
		Negative("No.").
		Value(&confirm).
		WithTheme(uiutils.NewAtmosHuhTheme())
	if err := prompt.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, fmt.Errorf("%w", errUtils.ErrUserAborted)
		}
		return false, err
	}
	return confirm, nil
}

// ExecutePs lists running emulators in the component's stack.
func ExecutePs(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "componentemulator.ExecutePs")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	statuses, err := r.manager().Ps(ctx, r.stack)
	if err != nil {
		return fmt.Errorf("%w: ps: %w", errUtils.ErrComponentExecutionFailed, err)
	}
	if len(statuses) == 0 {
		ui.Infof("no emulators running in stack %s", r.stack)
		return nil
	}
	for _, status := range statuses {
		ui.Writef("%s\t%s\t%s\t%s\n", status.Name, status.Image, status.Status, status.ID)
	}
	return nil
}

// ExecuteList lists emulators in a clean, theme-aware table with a status dot.
// Unlike ExecutePs it does not require a component: it builds the manager from
// the container-runtime config alone and lists every emulator discovered by
// label (scoped to info.Stack when `--stack` is set, otherwise all stacks).
func ExecuteList(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "componentemulator.ExecuteList")()

	info.ComponentType = cfg.EmulatorComponentType
	atmosConfig, err := initCliConfig(*info, true)
	if err != nil {
		return err
	}

	manager := newManager(strings.TrimSpace(atmosConfig.Container.Runtime.Provider), false)
	statuses, err := manager.Ps(ctx, info.Stack)
	if err != nil {
		return fmt.Errorf("%w: list: %w", errUtils.ErrComponentExecutionFailed, err)
	}

	renderEmulatorList(statuses, info.Stack)
	return nil
}

// renderEmulatorList prints the emulator statuses. In a TTY it renders the shared
// styled list table with a colored status dot; otherwise it emits a plain,
// tab-separated row per emulator so the output stays pipeable.
func renderEmulatorList(statuses []emu.Status, stack string) {
	if len(statuses) == 0 {
		if stack != "" {
			ui.Infof("No emulators running in stack %s.", stack)
		} else {
			ui.Info("No emulators running.")
		}
		return
	}

	if !terminal.New().IsTTY(terminal.Stdout) {
		for _, s := range statuses {
			ui.Writef("%s\t%s\t%s\t%s\t%s\n", s.Name, s.Stack, shortImage(s.Image), s.Status, shortID(s.ID))
		}
		return
	}

	styles := theme.GetCurrentStyles()
	header := []string{"", "NAME", "STACK", "IMAGE", "CONTAINER ID"}
	rows := make([][]string, 0, len(statuses))
	for _, s := range statuses {
		rows = append(rows, []string{
			statusDot(s.Status, styles),
			s.Name,
			s.Stack,
			shortImage(s.Image),
			shortID(s.ID),
		})
	}
	ui.Write(format.CreateStyledTable(header, rows))
}

// statusDot renders a colored ● indicating whether the emulator container is
// running (green) or not (muted).
func statusDot(status string, styles *theme.StyleSet) string {
	const dot = "●"
	s := strings.ToLower(status)
	// Check negative states first so "unhealthy" is not matched by the "healthy" substring below.
	if strings.Contains(s, "unhealthy") || strings.Contains(s, "exited") || strings.Contains(s, "dead") {
		return styles.Muted.Render(dot)
	}
	if strings.Contains(s, "up") || strings.Contains(s, "running") || strings.Contains(s, "healthy") {
		return styles.Success.Render(dot)
	}
	return styles.Muted.Render(dot)
}

// shortImage drops the `@sha256:…` digest and the `docker.io/` registry prefix so
// the image column stays narrow and readable (e.g. `floci/floci`).
func shortImage(image string) string {
	if idx := strings.Index(image, "@"); idx >= 0 {
		image = image[:idx]
	}
	image = strings.TrimPrefix(image, "docker.io/")
	return image
}

// shortID truncates a container ID to the conventional 12-character short form.
func shortID(id string) string {
	const shortIDLen = 12
	if len(id) > shortIDLen {
		return id[:shortIDLen]
	}
	return id
}

// ExecuteLogs streams the emulator container's logs.
func ExecuteLogs(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "componentemulator.ExecuteLogs")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	if err := r.manager().Logs(ctx, r.stack, r.component, false); err != nil {
		return fmt.Errorf("%w: logs %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// ExecuteExec runs a command in the emulator's container. Args after `--` form
// the command; defaults to a shell.
func ExecuteExec(ctx context.Context, info *schema.ConfigAndStacksInfo, command []string) error {
	defer perf.Track(nil, "componentemulator.ExecuteExec")()

	r, err := prepare(info)
	if err != nil {
		return err
	}
	if r.dryRun {
		ui.Infof("[dry-run] would exec in emulator %s: %v", r.component, command)
		return nil
	}
	if err := r.manager().Exec(ctx, r.stack, r.component, command); err != nil {
		return fmt.Errorf("%w: exec %q: %w", errUtils.ErrComponentExecutionFailed, r.component, err)
	}
	return nil
}

// isAbstractSection reports whether a component section is an abstract base.
func isAbstractSection(section map[string]any) bool {
	metadata, ok := section[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return false
	}
	componentType, _ := metadata["type"].(string)
	return componentType == "abstract"
}

// envListToMap converts a "KEY=VALUE" env slice into a map.
func envListToMap(list []string) map[string]string {
	out := make(map[string]string, len(list))
	for _, kv := range list {
		if i := strings.IndexByte(kv, '='); i > 0 {
			out[kv[:i]] = kv[i+1:]
		}
	}
	return out
}
