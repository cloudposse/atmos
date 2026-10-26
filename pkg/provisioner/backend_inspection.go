package provisioner

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// BackendInspectionOptions supplies static stack discovery without an exec package dependency.
type BackendInspectionOptions struct {
	Stack          string
	Format         string
	DescribeStacks func(*schema.AtmosConfiguration, string) (map[string]any, error)
}

// BackendConfiguration describes a component's merged configuration, not remote resource status.
type BackendConfiguration struct {
	Stack       string         `json:"stack" yaml:"stack"`
	Component   string         `json:"component" yaml:"component"`
	BackendType string         `json:"backend_type" yaml:"backend_type"`
	Backend     map[string]any `json:"backend" yaml:"backend"`
}

// ValidateBackendInspectionFormat rejects unsupported output before loading stack configuration.
func ValidateBackendInspectionFormat(format string) error {
	defer perf.Track(nil, "provisioner.ValidateBackendInspectionFormat")()

	if !slices.Contains([]string{"table", "json", "yaml"}, format) {
		return fmt.Errorf("%w: %q (supported: table, json, yaml)", errUtils.ErrInvalidFlagValue, format)
	}
	return nil
}

// ListBackends prints each concrete Terraform component's backend configuration in the selected stack.
func ListBackends(atmosConfig *schema.AtmosConfiguration, opts *BackendInspectionOptions) error {
	defer perf.Track(atmosConfig, "provision.ListBackends")()

	stacks, err := loadBackendInspectionStacks(atmosConfig, opts)
	if err != nil {
		return err
	}
	backends, err := collectBackendConfigurations(stacks, "")
	if err != nil {
		return err
	}
	return writeBackendConfigurations(backends, opts.Format, false)
}

// DescribeBackend prints one concrete Terraform component's backend configuration.
func DescribeBackend(atmosConfig *schema.AtmosConfiguration, component string, opts *BackendInspectionOptions) error {
	defer perf.Track(atmosConfig, "provision.DescribeBackend")()

	if component == "" {
		return errUtils.ErrComponentRequired
	}
	stacks, err := loadBackendInspectionStacks(atmosConfig, opts)
	if err != nil {
		return err
	}
	backends, err := collectBackendConfigurations(stacks, component)
	if err != nil {
		return fmt.Errorf("stack %q: %w", opts.Stack, err)
	}
	return writeBackendConfigurations(backends, opts.Format, true)
}

func loadBackendInspectionStacks(atmosConfig *schema.AtmosConfiguration, opts *BackendInspectionOptions) (map[string]any, error) {
	if opts == nil {
		return nil, errUtils.ErrInvalidConfig
	}
	if err := ValidateBackendInspectionFormat(opts.Format); err != nil {
		return nil, err
	}
	if opts.Stack == "" {
		return nil, fmt.Errorf("%w: --stack is required", errUtils.ErrRequiredFlagNotProvided)
	}
	if opts.DescribeStacks == nil || atmosConfig == nil {
		return nil, errUtils.ErrInvalidConfig
	}
	stacks, err := opts.DescribeStacks(atmosConfig, opts.Stack)
	if err != nil {
		return nil, err
	}
	if len(stacks) == 0 {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrStackNotFound, opts.Stack)
	}
	return stacks, nil
}

func collectBackendConfigurations(stacks map[string]any, component string) ([]BackendConfiguration, error) {
	result := make([]BackendConfiguration, 0)
	matches := 0
	for stack, raw := range stacks {
		items, matched := backendConfigurationsInStack(stack, raw, component)
		result = append(result, items...)
		if matched {
			matches++
		}
	}
	if component != "" {
		if matches == 0 {
			return nil, fmt.Errorf("%w: Terraform component %q", errUtils.ErrInvalidComponent, component)
		}
		if matches > 1 {
			return nil, fmt.Errorf("%w: component %q matches multiple logical stacks", errUtils.ErrInvalidComponent, component)
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("%w: Terraform component %q", errUtils.ErrBackendNotFound, component)
		}
	}
	slices.SortFunc(result, func(a, b BackendConfiguration) int {
		if n := strings.Compare(a.Stack, b.Stack); n != 0 {
			return n
		}
		return strings.Compare(a.Component, b.Component)
	})
	return result, nil
}

func backendConfigurationsInStack(stack string, raw any, component string) ([]BackendConfiguration, bool) {
	section, _ := raw.(map[string]any)
	components, _ := section["components"].(map[string]any)
	terraform, _ := components["terraform"].(map[string]any)
	var result []BackendConfiguration
	found := false
	for name, rawComponent := range terraform {
		if component != "" && name != component {
			continue
		}
		config, _ := rawComponent.(map[string]any)
		metadata, _ := config["metadata"].(map[string]any)
		if metadata["type"] == "abstract" {
			continue
		}
		found = true
		backendType, _ := config["backend_type"].(string)
		if backendType == "" {
			continue
		}
		backend, _ := config["backend"].(map[string]any)
		if backend == nil {
			backend = map[string]any{}
		}
		result = append(result, BackendConfiguration{Stack: stack, Component: name, BackendType: backendType, Backend: backend})
	}
	return result, found
}

func writeBackendConfigurations(backends []BackendConfiguration, format string, describe bool) error {
	if format != "yaml" {
		backends = displayBackendConfigurations(backends)
	}
	var output any = backends
	if describe {
		output = backends[0]
	}
	switch format {
	case "json":
		return data.WriteJSON(output)
	case "yaml":
		serialized, err := u.ConvertToYAML(output)
		if err != nil {
			return err
		}
		return data.Write(serialized)
	default:
		return writeBackendTable(backends, describe)
	}
}

func writeBackendTable(backends []BackendConfiguration, describe bool) error {
	headers := []string{"STACK", "COMPONENT", "BACKEND TYPE"}
	if describe {
		headers = append(headers, "CONFIGURATION")
	}
	rows := make([][]string, 0, len(backends))
	for _, backend := range backends {
		row := []string{backend.Stack, backend.Component, backend.BackendType}
		if describe {
			config, err := json.Marshal(backend.Backend)
			if err != nil {
				return err
			}
			row = append(row, string(config))
		}
		rows = append(rows, row)
	}
	style := theme.DefaultTableConfig()
	return data.Writeln(theme.CreateTable(&style, headers, rows))
}

// displayBackendConfigurations unwraps native expressions only on output copies.
// YAML keeps the source identities so its encoder can restore actual tags.
func displayBackendConfigurations(backends []BackendConfiguration) []BackendConfiguration {
	result := make([]BackendConfiguration, len(backends))
	for i, backend := range backends {
		backend.Stack = yamlsource.Expression(backend.Stack)
		backend.Component = yamlsource.Expression(backend.Component)
		backend.BackendType = yamlsource.Expression(backend.BackendType)
		backend.Backend, _ = yamlsource.Display(backend.Backend).(map[string]any)
		result[i] = backend
	}
	return result
}
