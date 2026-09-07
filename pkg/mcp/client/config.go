package client

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultTimeout = 30 * time.Second

var (
	errMCPHTTPURLEmpty       = errors.New("url must not be empty")
	errMCPHTTPURLScheme      = errors.New("url must use http or https")
	errMCPHTTPURLMissingHost = errors.New("url must include a host")
)

// ParsedConfig holds a parsed server config with resolved timeout.
type ParsedConfig struct {
	Name        string
	Description string
	Command     string
	Args        []string
	Env         map[string]string
	Type        string
	URL         string
	Headers     map[string]string
	AutoStart   bool
	Timeout     time.Duration
	Identity    string
}

// ParseConfig validates and converts an MCPServerConfig into a ParsedConfig.
func ParseConfig(name string, cfg schema.MCPServerConfig) (*ParsedConfig, error) { //nolint:gocritic // hugeParam: cfg is read-only config value.
	transportType := cfg.TransportType()
	switch transportType {
	case schema.MCPTransportStdio:
		if cfg.Command == "" {
			return nil, fmt.Errorf("%w: server %q", errUtils.ErrMCPServerCommandEmpty, name)
		}
	case schema.MCPTransportHTTP:
		if err := validateHTTPURL(cfg.URL); err != nil {
			return nil, fmt.Errorf("%w: server %q: %w", errUtils.ErrMCPInvalidTransport, name, err)
		}
	default:
		return nil, fmt.Errorf("%w: server %q: %s", errUtils.ErrMCPInvalidTransport, name, transportType)
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
		Name:        name,
		Description: cfg.Description,
		Command:     cfg.Command,
		Args:        cfg.Args,
		Env:         env,
		Type:        transportType,
		URL:         cfg.URL,
		Headers:     cfg.Headers,
		AutoStart:   cfg.AutoStart,
		Timeout:     timeout,
		Identity:    cfg.Identity,
	}, nil
}

func validateHTTPURL(rawURL string) error {
	if rawURL == "" {
		return errMCPHTTPURLEmpty
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errMCPHTTPURLScheme
	}
	if parsed.Host == "" {
		return errMCPHTTPURLMissingHost
	}
	return nil
}
