// Package scaffold generates plugin projects using Atmos's existing scaffold engine.
package scaffold

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// Options selects the implemented starter and its generation behavior.
type Options struct {
	DisplayName string
	Description string
	Author      string
	Repository  string
	Name        string
	Module      string
	Runtime     string
	Capability  string
	SDKVersion  string
	SDKPath     string
	Force       bool
	Defaults    bool
}

// Create runs the shared scaffold loader, field validation, rendering, conflict
// handling, and provenance recording. It does not invoke another Cobra command.
//
//nolint:gocritic // Immutable options snapshot prevents caller mutations during generation.
func Create(ctx context.Context, directory string, options Options) error {
	defer perf.Track(nil, "pluginscaffold.Create")()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateOptions(&options); err != nil {
		return err
	}
	configurations, err := templates.GetAvailableConfigurations()
	if err != nil {
		return err
	}
	template, found := configurations["plugin-oidc"]
	if !found {
		return errUtils.ErrPluginInvalidConfig
	}
	ioContext, err := iolib.NewContext()
	if err != nil {
		return errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	ui := generatorUI.NewInitUI(ioContext, terminal.New(terminal.WithIO(iolib.NewTerminalWriter(ioContext))))
	values := map[string]any{"plugin_name": options.Name, "module": options.Module, "sdk_version": options.SDKVersion, "platform": runtime.GOOS + "/" + runtime.GOARCH}
	for key, value := range map[string]string{"display_name": options.DisplayName, "description": options.Description, "author": options.Author, "repository": options.Repository} {
		if value != "" {
			values[key] = value
		}
	}
	if options.SDKPath != "" {
		absolute, err := filepath.Abs(options.SDKPath)
		if err != nil {
			return errors.Join(errUtils.ErrPluginInvalidConfig, err)
		}
		values["sdk_path"] = absolute
	}
	return ui.ExecuteWithBaseRef(&template, directory, options.Force, false, options.Defaults, "", values)
}

func validateOptions(options *Options) error {
	if options.Runtime != "go" || options.Capability != "auth.provider" {
		return errUtils.ErrPluginInvalidConfig
	}
	if err := module.CheckPath(options.Module); err != nil {
		return errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if options.SDKPath == "" && !semver.IsValid(options.SDKVersion) {
		return errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithHint("Set --sdk-version to a released Atmos SDK version, or use --sdk-path for a local development checkout.").Err()
	}
	return nil
}
