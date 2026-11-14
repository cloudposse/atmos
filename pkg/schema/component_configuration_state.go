package schema

import "context"

// ComponentConfigurationStateRuntime resolves state without preparing or executing a unit.
// It is optional so discovery-only providers do not imply backend support.
type ComponentConfigurationStateRuntime interface {
	ResolveState(context.Context, *ConfigAndStacksInfo) (*ComponentConfigurationState, error)
}

// ComponentConfigurationState identifies the upstream backend independently of Atmos workdirs.
type ComponentConfigurationState struct {
	BackendType      string
	Backend          map[string]any
	Workspace        string
	WorkingDirectory string
}
