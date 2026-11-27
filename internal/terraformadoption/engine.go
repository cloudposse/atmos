// Package terraformadoption identifies the native CLI used by an existing Terraform-family root.
package terraformadoption

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

const (
	terraform  = "terraform"
	tofu       = "tofu"
	choiceHint = "choose with --command=terraform or --command=tofu"
)

// Engine carries a portable command for the stack and the executable resolved for this trial.
type Engine struct {
	Command string
	Path    string
	Reason  string
	Pin     string
}

// Selection holds caller preferences for choosing an executable.
type Selection struct {
	Explicit, Configured string
	Interactive          bool
}

// Select inspects only the selected directory and project-scoped evidence.
// Availability is a fallback. Never silently substitute a different installed engine.
func Select(ctx context.Context, dir, project string, opts Selection) (Engine, error) {
	defer perf.Track(nil, "terraformadoption.Select")()

	tf, ot, err := sourceFiles(dir)
	if err != nil {
		return Engine{}, err
	}
	dirs := evidenceDirectories(dir, project)
	pins, err := projectPins(dirs)
	if err != nil {
		return Engine{}, err
	}
	cmd, reason := commandPreference(opts.Explicit, opts.Configured)
	if cmd != "" {
		if !tf && cmd == terraform {
			return Engine{}, fmt.Errorf("%w: Terraform cannot read a root containing only OpenTofu configuration", errUtils.ErrInvalidComponent)
		}
		return resolve(ctx, dir, cmd, reason, pins[cmd])
	}
	return inferEngine(ctx, dir, engineEvidence{dirs: dirs, pins: pins, tf: tf, ot: ot, interactive: opts.Interactive})
}

type engineEvidence struct {
	dirs                []string
	pins                map[string]string
	tf, ot, interactive bool
}

func commandPreference(explicit, configured string) (string, string) {
	if explicit != "" {
		return explicit, "--command"
	}
	if configured == "" {
		env := viper.New()
		env.AutomaticEnv()
		configured = env.GetString("ATMOS_COMPONENTS_TERRAFORM_COMMAND")
	}
	return configured, "configured command"
}

func inferEngine(ctx context.Context, dir string, observed engineEvidence) (Engine, error) {
	dirs, pins, tf, ot, interactive := observed.dirs, observed.pins, observed.tf, observed.ot, observed.interactive
	hints, err := engineHints(dirs, dir, pins, ot && !tf)
	if err != nil {
		return Engine{}, err
	}
	if len(hints) == 1 {
		for command, reason := range hints {
			return resolve(ctx, dir, command, reason, pins[command])
		}
	}
	if !tf {
		// Conflicting declarations cannot make a Terraform-only binary read .tofu files.
		return Engine{}, fmt.Errorf("%w: conflicting engine evidence for an OpenTofu-only root; select --command=tofu", errUtils.ErrInvalidToolSpec)
	}
	return selectAvailable(ctx, dir, pins, hints, selectionMode{dual: ot, interactive: interactive})
}

type selectionMode struct{ dual, interactive bool }

func selectAvailable(ctx context.Context, dir string, pins map[string]string, hints evidence, mode selectionMode) (Engine, error) {
	var available []Engine
	var unavailable []error
	for _, command := range []string{terraform, tofu} {
		engine, err := resolve(ctx, dir, command, "available executable", pins[command])
		if err == nil {
			available = append(available, engine)
		} else {
			unavailable = append(unavailable, err)
		}
	}
	if len(hints) == 0 && !mode.dual && len(available) == 1 {
		return available[0], nil
	}
	if len(available) == 0 {
		return Engine{}, fmt.Errorf("no usable engine for %s; %s: %w", dir, choiceHint, errors.Join(unavailable...))
	}
	reason := "ambiguous engine"
	if len(hints) > 1 {
		reason = fmt.Sprintf("conflicting engine evidence (%s: %s, %s: %s)", terraform, hints[terraform], tofu, hints[tofu])
	}
	if !mode.interactive {
		return Engine{}, fmt.Errorf("%w: %s for %s; %s", errUtils.ErrInvalidToolSpec, reason, dir, choiceHint)
	}
	return promptEngine(ctx, dir, available)
}

func promptEngine(ctx context.Context, dir string, available []Engine) (Engine, error) {
	var labels []string
	for _, engine := range available {
		labels = append(labels, engine.Command+"  "+versionLine(ctx, dir, engine.Path)+"  "+engine.Path)
	}
	selected, err := flags.PromptForValue("command", "Which CLI do you use for "+dir+"?", labels)
	if err != nil {
		return Engine{}, err
	}
	i := slices.Index(labels, selected)
	if i < 0 {
		return Engine{}, fmt.Errorf("%w: no engine selected; %s", errUtils.ErrInvalidToolSpec, choiceHint)
	}
	engine := available[i]
	engine.Reason = "interactive selection"
	return engine, nil
}

var releasePin = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)

func resolve(ctx context.Context, dir, command, reason, pin string) (Engine, error) {
	var err error
	name := command
	if !filepath.IsAbs(name) && filepath.Base(name) != name {
		name = filepath.Join(dir, name)
	}
	if pin == "system" {
		pin = ""
	}
	if pin != "" {
		name, err = pinnedBinary(command, pin, name)
		if err != nil {
			return Engine{}, err
		}
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return Engine{}, fmt.Errorf("%s selects %q but it is unavailable: %w", reason, command, err)
	}
	if pin != "" {
		line := versionLine(ctx, dir, path)
		product := "Terraform"
		if command == tofu {
			product = "OpenTofu"
		}
		if line != product+" v"+strings.TrimPrefix(pin, "v") {
			return Engine{}, fmt.Errorf("%w: %s requires %s %s; %s reports %q", errUtils.ErrInvalidToolSpec, reason, command, pin, path, line)
		}
	}
	return Engine{Command: command, Path: path, Reason: reason, Pin: pin}, nil
}

const nativeVersionTimeout = 15 * time.Second

func versionLine(ctx context.Context, dir, path string) string {
	ctx, cancel := context.WithTimeout(ctx, nativeVersionTimeout)
	defer cancel()
	probe := exec.CommandContext(ctx, path, "version")
	probe.Dir = dir
	out, err := probe.Output()
	if err != nil {
		return "version unavailable"
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line)
}

func sourceFiles(dir string) (bool, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, false, err
	}
	var tf, ot bool
	for _, entry := range entries {
		name := entry.Name()
		if ignoredSource(entry) {
			continue
		}
		switch {
		case name == "terragrunt.hcl" || name == "terragrunt.stack.hcl":
			return false, false, fmt.Errorf("%w: %s is a Terragrunt deployment; use the Terragrunt integration", errUtils.ErrInvalidToolSpec, dir)
		case configSuffix(name, ".tf"):
			tf = true
		case configSuffix(name, ".tofu"):
			ot = true
		}
	}
	if !tf && !ot {
		return false, false, fmt.Errorf("%w: directory contains no Terraform or OpenTofu configuration", errUtils.ErrInvalidComponent)
	}
	return tf, ot, nil
}

func engineHints(dirs []string, dir string, pins map[string]string, tofuOnly bool) (evidence, error) {
	hints, err := deploymentHints(dirs, dir)
	if err != nil {
		return nil, err
	}
	if tofuOnly {
		hints.add(tofu, "OpenTofu-only root")
	}
	// A pin for each engine describes a dual-engine toolchain. A deployment-specific
	// declaration can select one of them. A single contrary pin requires a decision.
	if len(pins) == 1 || len(hints) == 0 {
		for command := range pins {
			hints.add(command, ".tool-versions")
		}
	}
	return hints, nil
}

func pinnedBinary(command, pin, name string) (string, error) {
	// Only exact release pins can be preserved through both trial and saved runs.
	if !releasePin.MatchString(pin) {
		return "", fmt.Errorf("%w: %s has unsupported engine version %q; select an exact release in .tool-versions", errUtils.ErrInvalidToolSpec, command, pin)
	}
	owner, repo := "hashicorp", "terraform"
	if command == tofu {
		owner, repo = "opentofu", "opentofu"
	}
	if installed, err := toolchain.NewInstaller().FindBinaryPath(owner, repo, strings.TrimPrefix(pin, "v"), command); err == nil {
		name = installed
	}
	return name, nil
}

func configSuffix(name, ext string) bool {
	return strings.HasSuffix(name, ext) || strings.HasSuffix(name, ext+".json")
}

func ignoredSource(entry os.DirEntry) bool {
	return entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "~")
}
