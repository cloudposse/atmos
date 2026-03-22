package client

import (
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultTimeout = 30 * time.Second

// ParsedConfig holds a parsed integration config with resolved timeout.
type ParsedConfig struct {
	Name        string
	Description string
	Command     string
	Args        []string
	Env         map[string]string
	AutoStart   bool
	Timeout     time.Duration
}

// ParseConfig validates and converts an MCPIntegrationConfig into a ParsedConfig.
func ParseConfig(name string, cfg schema.MCPIntegrationConfig) (*ParsedConfig, error) { //nolint:gocritic // hugeParam: cfg is read-only config value.
	if cfg.Command == "" {
		return nil, fmt.Errorf("%w: integration %q", errUtils.ErrMCPIntegrationCommandEmpty, name)
	}

	timeout := defaultTimeout
	if cfg.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("%w: integration %q: %w", errUtils.ErrMCPIntegrationInvalidTimeout, name, err)
		}
	}

	env := cfg.Env
	if env == nil {
		env = make(map[string]string)
	}

	return &ParsedConfig{
		Name:        name,
		Description: cfg.Description,
		Command:     cfg.Command,
		Args:        cfg.Args,
		Env:         env,
		AutoStart:   cfg.AutoStart,
		Timeout:     timeout,
	}, nil
}
