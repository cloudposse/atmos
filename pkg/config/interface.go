package config

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// Loader defines operations for loading Atmos configuration.
// This interface allows mocking of config loading in tests.
//
//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type Loader interface {
	// InitCliConfig initializes the CLI configuration.
	InitCliConfig(configAndStacksInfo *schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
}

// DefaultLoader implements Loader using real config operations.
type DefaultLoader struct{}

// InitCliConfig initializes the CLI configuration.
func (d *DefaultLoader) InitCliConfig(configAndStacksInfo *schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
	return InitCliConfig(*configAndStacksInfo, processStacks)
}
