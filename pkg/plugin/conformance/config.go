// Package conformance runs explicit local plugin fixtures and authored Atmos test steps.
package conformance

import (
	"bytes"
	"errors"
	"io"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Config declares mock protocol fixtures separately from opt-in live tests.
type Config struct {
	Version     int                   `yaml:"version"`
	OIDC        *OIDCFixture          `yaml:"oidc,omitempty"`
	Commands    []CommandFixture      `yaml:"commands,omitempty"`
	Steps       []schema.WorkflowStep `yaml:"steps,omitempty"`
	Integration []schema.WorkflowStep `yaml:"integration,omitempty"`
}

// OIDCFixture selects the scaffold's versioned private-session conformance contract.
type OIDCFixture struct {
	Provider string `yaml:"provider"`
	Identity string `yaml:"identity"`
	Session  string `yaml:"session"`
}

// CommandFixture asserts both output channels and the process status.
type CommandFixture struct {
	Name      string   `yaml:"name"`
	Arguments []string `yaml:"arguments,omitempty"`
	Stdout    string   `yaml:"stdout"`
	Stderr    string   `yaml:"stderr"`
	ExitCode  int      `yaml:"exit_code"`
}

func load(directory string) (*Config, error) {
	raw, err := manifest.ReadFile(directory, "plugin.tests.yaml")
	if err != nil {
		return nil, err
	}
	var config Config
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, errUtils.Build(errors.Join(errUtils.ErrPluginInvalidConfig, err)).
			WithHint("Check plugin.tests.yaml against the fixture fields; unknown fields are rejected.").Err()
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("plugin.tests.yaml must contain exactly one YAML document.").
			WithHint("Remove the extra `---` document.").Err()
	}
	if err := validateConfig(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func validateConfig(config *Config) error {
	// Registered extension hooks may supply the entire local suite. The runner
	// rejects a selection that executes neither hooks nor authored fixtures.
	if config.Version != 1 {
		return errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanationf("plugin.tests.yaml `version` is %d, but only version 1 is supported.", config.Version).
			WithHint("Set `version: 1` at the top of plugin.tests.yaml.").Err()
	}
	if config.OIDC != nil && (config.OIDC.Provider == "" || config.OIDC.Identity == "" || config.OIDC.Session != "oidc-starter/v1") {
		return errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("The `oidc` fixture needs `provider`, `identity`, and `session: oidc-starter/v1`.").
			WithHint("Copy the `oidc` block from a freshly scaffolded plugin (`atmos plugin create`).").Err()
	}
	return nil
}
