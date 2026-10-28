package bootstrap

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"google.golang.org/protobuf/types/known/structpb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/runtime/native"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Output selects command data and diagnostic streams.
type Output struct {
	Stdout, Stderr io.Writer
	// OnStart identifies the selected command before invoking the plugin.
	OnStart func(alias, name, description string)
}

// Run executes the selected command export of an installed package.
func Run(ctx context.Context, atmos *schema.AtmosConfiguration, alias string, arguments []string, output Output) error {
	defer perf.Track(atmos, "pluginbootstrap.Run")()
	activation, err := commandSelection(ctx, atmos, alias)
	if err != nil {
		return err
	}
	settings := atmos.Plugins.Packages[alias].Config
	if path, ok := activation.Metadata["schema"].(string); ok {
		compiled, err := extension.CompileSchema(activation.Directory, path)
		if err != nil {
			return err
		}
		if settings == nil {
			settings = map[string]any{}
		}
		if err := compiled.Validate(settings); err != nil {
			return errUtils.ErrPluginInvalidConfig
		}
	}
	values, err := structpb.NewStruct(settings)
	if err != nil {
		return errUtils.ErrPluginInvalidConfig
	}
	timeout, err := pluginconfig.PackageTimeout(atmos.Plugins.Packages[alias])
	if err != nil {
		return err
	}
	handle, err := native.Start(ctx, activation.Executable, native.WithDirectory(activation.Directory), native.WithDescription(activation.Description), native.WithCallTimeout(timeout))
	if err != nil {
		return err
	}
	defer handle.Close()
	if output.OnStart != nil {
		description, _ := activation.Metadata["description"].(string)
		output.OnStart(alias, activation.Name, description)
	}
	// Command execution receives HTTP but no auth provider's private credential namespace.
	return handle.Invoke(ctx, bridge.Invocation{Namespace: activation.Package + ":command:" + activation.Name, HTTP: api.HTTPService{}}, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		return handle.Command(ctx, &pluginv1.CommandRequest{Invocation: invocation, Name: activation.Name, Arguments: arguments, Settings: values}, output.Stdout, output.Stderr)
	})
}

func commandSelection(ctx context.Context, atmos *schema.AtmosConfiguration, alias string) (extension.Activation, error) {
	state, err := selectedPackages(ctx, atmos, alias)
	if err != nil {
		return extension.Activation{}, err
	}
	entry, exists := state.Packages[alias]
	if !exists {
		return extension.Activation{}, fmt.Errorf("%w: %s", errUtils.ErrPluginNotInstalled, alias)
	}
	announceLocal(atmos, state)
	definition, err := loadLocalManifest(alias, &entry)
	if err != nil {
		return extension.Activation{}, err
	}
	registry, err := Descriptors()
	if err != nil {
		return extension.Activation{}, err
	}
	bindings, err := registry.Validate(entry.Path, definition.Spec.Capabilities)
	if err != nil {
		return extension.Activation{}, repairLocal(alias, &entry, err)
	}
	expected, err := DescribePackage(entry.Path, definition, bindings)
	if err != nil {
		return extension.Activation{}, repairLocal(alias, &entry, err)
	}
	executable := selectedExecutable(&entry, definition)
	if err := validateExecutable(entry.Path, executable); err != nil {
		return extension.Activation{}, repairLocal(alias, &entry, err)
	}
	binding, err := selectCommand(bindings, atmos.Plugins.Packages[alias].Command)
	if err != nil {
		return extension.Activation{}, err
	}
	return extension.Activation{Binding: binding, Package: definition.Metadata.Name, Executable: filepath.Join(entry.Path, executable), Description: expected}, nil
}

func selectCommand(bindings []extension.Binding, selected string) (extension.Binding, error) {
	var candidates []extension.Binding
	for _, binding := range bindings {
		if binding.Capability == "command" && (selected == "" || selected == binding.Name) {
			candidates = append(candidates, binding)
		}
	}
	if len(candidates) != 1 {
		return extension.Binding{}, fmt.Errorf("%w: select one exported command with plugins.packages.<alias>.command", errUtils.ErrPluginInvalidConfig)
	}
	return candidates[0], nil
}
