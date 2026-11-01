// Package model contains evaluator data, independent of the plugin transport.
package model

type Request struct {
	IncludedPaths  []string          `json:"included_paths,omitempty"`
	ExcludedPaths  []string          `json:"excluded_paths,omitempty"`
	Phase          string            `json:"phase,omitempty"`
	GenerationRoot string            `json:"generation_root,omitempty"`
	BasePath       string            `json:"base_path,omitempty"`
	OriginFiles    []string          `json:"origin_files,omitempty"`
	OriginFolders  []string          `json:"origin_folders,omitempty"`
	LogLevel       string            `json:"log_level,omitempty"`
	Path           string            `json:"path"`
	Mode           string            `json:"mode"`
	Command        string            `json:"command"`
	Arguments      []string          `json:"arguments"`
	Tofu           string            `json:"tofu"`
	Environment    map[string]string `json:"environment"`
}

type Resolution struct {
	Inventory          []DiscoveredUnit  `json:"inventory,omitempty"`
	Path               string            `json:"path,omitempty"`
	OutputDependencies []string          `json:"output_dependencies,omitempty"`
	Dependencies       []string          `json:"dependencies,omitempty"`
	Files              []string          `json:"files,omitempty"`
	Folders            []string          `json:"folders,omitempty"`
	WorkingDir         string            `json:"working_dir,omitempty"`
	UnitSources        map[string]string `json:"unit_sources,omitempty"`
	StackSources       map[string]string `json:"stack_sources,omitempty"`
	UnitConfigs        map[string]string `json:"unit_configs,omitempty"`
	StackConfigs       map[string]string `json:"stack_configs,omitempty"`
	Inputs             map[string]any    `json:"inputs"`
	Source             string            `json:"source"`
	BackendType        string            `json:"backend_type"`
	Backend            map[string]any    `json:"backend"`
	Generated          map[string]string `json:"generated"`
	Units              map[string]string `json:"units,omitempty"`
	Stacks             map[string]string `json:"stacks,omitempty"`
}

// DiscoveredUnit carries source identity independently from its Atmos name.
type DiscoveredUnit struct {
	Stack          string     `json:"stack"`
	Name           string     `json:"name"`
	GenerationRoot string     `json:"generation_root,omitempty"`
	Config         Resolution `json:"config"`
}
