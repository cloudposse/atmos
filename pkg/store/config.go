package store

import "github.com/mitchellh/mapstructure"

type StoreConfig struct {
	Name    string                 `yaml:"name"`
	Type    string                 `yaml:"type"`
	Options map[string]interface{} `yaml:"options"`
}

type StoresConfig struct {
	Stores []StoreConfig `yaml:"stores"`
}

func parseOptions(options map[string]interface{}, target interface{}) error {
	return mapstructure.Decode(options, target)
}
