package store

type StoreConfig struct {
	Type     string                 `yaml:"type"`
	Identity string                 `yaml:"identity,omitempty"`
	Options  map[string]interface{} `yaml:"options"`
}

type StoresConfig = map[string]StoreConfig
