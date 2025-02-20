package store

import "github.com/mitchellh/mapstructure"

type StoreConfig struct {
	Type    string         `yaml:"type"`
	Options map[string]any `yaml:"options"`
}

type StoresConfig = map[string]StoreConfig

func parseOptions(options map[string]any, target any) error {
	return mapstructure.Decode(options, target)
}
