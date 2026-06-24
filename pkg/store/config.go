package store

import "github.com/go-viper/mapstructure/v2"

type StoreConfig struct {
	// Type is the legacy backend selector (e.g. "aws-ssm-parameter-store").
	Type string `yaml:"type"`
	// Kind is the new cloud/thing backend selector (e.g. "aws/ssm"); when set it takes
	// precedence over Type. The registry maps legacy Type to Kind for backward compatibility.
	Kind string `yaml:"kind,omitempty"`
	// Secret marks this store as a secret backend (subsystem membership). A secret store
	// is the only backend the !secret function and the `atmos secret` CLI resolve from, and
	// `!store` against it is an error ("use !secret"). Secret stores always write the
	// sensitive variant at rest (e.g. SSM SecureString).
	Secret   bool                   `yaml:"secret,omitempty"`
	Identity string                 `yaml:"identity,omitempty"`
	Options  map[string]interface{} `yaml:"options"`
}

type StoresConfig = map[string]StoreConfig

func parseOptions(options map[string]interface{}, target interface{}) error {
	return mapstructure.Decode(options, target)
}
