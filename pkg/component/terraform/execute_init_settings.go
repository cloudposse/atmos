package terraform

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/component/execution"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
)

// initFileFlags are the init options whose value can name a file Terraform reads during init.
var initFileFlags = []string{"-backend-config", "-var-file"}

// withProviderInitSettings adds the init settings that provider extra arguments contribute to smart
// init's inputs: the effective init arguments and environment, and the contents of files those
// arguments name. Without them, an edited init-only backend file would leave a stale initialization
// trusted. Values only contribute to the fingerprint digest; they are never recorded or logged.
// Settings that cannot be evaluated return nil inputs, so no recorded marker is trusted; the init
// phase itself then reports the problem.
func withProviderInitSettings(in *autoinit.Inputs, info *schema.ConfigAndStacksInfo, componentPath string) *autoinit.Inputs {
	config := info.ComponentExecution
	if in == nil || config.IsEmpty() {
		return in
	}
	applied, err := execution.ApplyExtraArguments(config.ExtraArguments, &execution.ArgumentsRequest{
		Command: subcommandInit, Args: []string{subcommandInit}, InsertAt: 1, WorkingDirectory: componentPath,
	})
	if err != nil {
		return nil
	}
	in.EnvLookup = overlayEnvironment(applied.Environment, in.EnvLookup)
	extra := maps.Clone(in.Extra)
	if extra == nil {
		extra = map[string]string{}
	}
	arguments := applied.Args[1:]
	extra["provider-init-arguments"] = strings.Join(arguments, "\x00")
	extra["provider-init-environment"] = strings.Join(applied.Environment, "\x00")
	for _, path := range initFileArguments(arguments) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(componentPath, path)
		}
		// A missing or unreadable file is recorded as such, so creating it changes the digest.
		digest, err := fileDigest(path)
		if err != nil {
			digest = "unreadable"
		}
		extra["provider-init-file:"+path] = digest
	}
	in.Extra = extra
	return in
}

// overlayEnvironment looks up KEY=value entries first and then base. A nil base reports a key
// absent, so the caller falls back to the process environment.
func overlayEnvironment(entries []string, base func(string) (string, bool)) func(string) (string, bool) {
	environment := map[string]string{}
	for _, entry := range entries {
		if key, value, ok := strings.Cut(entry, "="); ok {
			environment[key] = value
		}
	}
	return func(key string) (string, bool) {
		if value, ok := environment[key]; ok {
			return value, true
		}
		if base == nil {
			return "", false
		}
		return base(key)
	}
}

// initFileArguments returns the file paths named by -backend-config and -var-file options, in
// both the "-flag=value" and "-flag value" forms. A -backend-config value holding "=" sets one
// setting instead of naming a file; it is already part of the arguments.
func initFileArguments(arguments []string) []string {
	var paths []string
	for i := 0; i < len(arguments); i++ {
		name, value, inline := strings.Cut(strings.TrimPrefix(arguments[i], "-"), "=")
		name = "-" + name
		if !slices.Contains(initFileFlags, name) {
			continue
		}
		if !inline {
			if i+1 >= len(arguments) {
				break
			}
			i++
			value = arguments[i]
		}
		if name == "-backend-config" && strings.Contains(value, "=") {
			continue
		}
		paths = append(paths, value)
	}
	return paths
}
