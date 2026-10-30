package configuration

import (
	"maps"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/execution"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// preparedExecution copies the provider's hooks and extra arguments, preserving declaration
// order. The host runs them, so anything it cannot run is a protocol error, never a silent skip.
func preparedExecution(source *pluginv1.ComponentExecutionConfiguration) (*schema.ComponentExecution, error) {
	if source == nil {
		return nil, nil
	}
	result := &schema.ComponentExecution{}
	for _, item := range source.GetExtraArguments() {
		if item == nil {
			return nil, errUtils.ErrPluginProtocol
		}
		result.ExtraArguments = append(result.ExtraArguments, schema.ComponentExtraArguments{
			Commands:         slices.Clone(item.GetCommands()),
			Arguments:        slices.Clone(item.GetArguments()),
			RequiredVarFiles: slices.Clone(item.GetRequiredVarFiles()),
			OptionalVarFiles: slices.Clone(item.GetOptionalVarFiles()),
			Environment:      maps.Clone(item.GetEnvironment()),
		})
	}
	var err error
	if result.BeforeHooks, err = preparedHooks(source.GetBeforeHooks()); err != nil {
		return nil, err
	}
	if result.AfterHooks, err = preparedHooks(source.GetAfterHooks()); err != nil {
		return nil, err
	}
	if result.ErrorHooks, err = preparedHooks(source.GetErrorHooks()); err != nil {
		return nil, err
	}
	if err := execution.Validate(result); err != nil {
		return nil, err
	}
	if result.IsEmpty() {
		return nil, nil
	}
	return result, nil
}

func preparedHooks(hooks []*pluginv1.ComponentHook) ([]schema.ComponentHook, error) {
	var result []schema.ComponentHook
	for _, hook := range hooks {
		if hook == nil {
			return nil, errUtils.ErrPluginProtocol
		}
		result = append(result, schema.ComponentHook{
			Name:             hook.GetName(),
			Commands:         slices.Clone(hook.GetCommands()),
			Execute:          slices.Clone(hook.GetExecute()),
			WorkingDirectory: hook.GetWorkingDirectory(),
			RunOnError:       hook.GetRunOnError(),
			SuppressStdout:   hook.GetSuppressStdout(),
			OnErrors:         slices.Clone(hook.GetOnErrors()),
		})
	}
	return result, nil
}
