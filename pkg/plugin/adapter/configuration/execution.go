package configuration

import (
	"fmt"
	"maps"
	"os"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/execution"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// preparedExecution copies the provider's hooks and extra arguments, preserving declaration
// order. The host runs them, so anything it cannot run is a protocol error, never a silent skip.
func preparedExecution(source *pluginv1.ComponentExecutionConfiguration) (*schema.ComponentExecution, error) {
	if source == nil {
		return nil, nil
	}
	result, err := executionMetadata(source)
	if err != nil {
		return nil, err
	}
	// The apply-time evaluation is kept even when empty: it means apply adds nothing,
	// which differs from an absent evaluation, where the plan-time metadata applies.
	if apply := source.GetApply(); apply != nil {
		if apply.GetApply() != nil {
			return nil, fmt.Errorf("%w: apply-time execution metadata cannot nest another", errUtils.ErrPluginProtocol)
		}
		if result.Apply, err = executionMetadata(apply); err != nil {
			return nil, err
		}
	}
	if result.IsEmpty() && result.Apply == nil {
		return nil, nil
	}
	return result, nil
}

// executionMetadata converts one evaluation of hooks and extra arguments and validates it.
func executionMetadata(source *pluginv1.ComponentExecutionConfiguration) (*schema.ComponentExecution, error) {
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

// preparedIAM copies the role the provider asks the host to assume. A web identity token names a file
// when one exists at that path, as it does for Terragrunt, and is the token itself otherwise; only the
// file path may be kept with a saved plan, so a literal token stays in memory.
func preparedIAM(source *pluginv1.ComponentIam) (*schema.ComponentIAM, error) {
	if source == nil || source.GetRole() == "" {
		return nil, nil
	}
	token, err := adapter.DecodeString(source.GetWebIdentityToken())
	if err != nil {
		return nil, err
	}
	result := &schema.ComponentIAM{Role: source.GetRole(), SessionName: source.GetSessionName(), DurationSeconds: source.GetDurationSeconds()}
	if info, statErr := os.Stat(token); token != "" && statErr == nil && info.Mode().IsRegular() {
		result.WebIdentityTokenFile = token
	} else {
		result.WebIdentityToken, result.WebIdentityTokenLiteral = token, token != ""
	}
	return result, nil
}
