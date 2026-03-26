package client

import (
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultTimeout = 30 * time.Second

// ParsedConfig holds a parsed server config with resolved timeout.
type ParsedConfig struct {
	Name         string
	Description  string
	Command      string
	Args         []string
	Env          map[string]string
	AutoStart    bool
	Timeout      time.Duration
	AuthIdentity string
	ReadOnly     bool
}

// ParseConfig validates and converts an MCPServerConfig into a ParsedConfig.
func ParseConfig(name string, cfg schema.MCPServerConfig) (*ParsedConfig, error) { //nolint:gocritic // hugeParam: cfg is read-only config value.
	if cfg.Command == "" {
		return nil, fmt.Errorf("%w: server %q", errUtils.ErrMCPServerCommandEmpty, name)
	}

	timeout := defaultTimeout
	if cfg.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("%w: server %q: %w", errUtils.ErrMCPServerInvalidTimeout, name, err)
		}
	}

	env := cfg.Env
	if env == nil {
		env = make(map[string]string)
	}

	return &ParsedConfig{
		Name:         name,
		Description:  cfg.Description,
		Command:      cfg.Command,
		Args:         cfg.Args,
		Env:          env,
		AutoStart:    cfg.AutoStart,
		Timeout:      timeout,
		AuthIdentity: cfg.AuthIdentity,
		ReadOnly:     cfg.ReadOnly,
	}, nil
}
