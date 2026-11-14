package schema

// The typed value cannot be forged by a YAML component section.
const backendFileGenerationKey = "_backend_file_generation"

type backendGenerationChoice struct{ enabled bool }

// SetBackendFileGeneration stores the prepared choice on this invocation and
// its section for provisioners that receive only the section.
func (info *ConfigAndStacksInfo) SetBackendFileGeneration(enabled bool) {
	info.BackendFileGeneration = &enabled
	info.ComponentSection[backendFileGenerationKey] = backendGenerationChoice{enabled: enabled}
}

// GenerateBackendFile returns the prepared choice when present, otherwise the
// project default. A nil info covers read-only and early-return paths.
func GenerateBackendFile(config *AtmosConfiguration, info *ConfigAndStacksInfo) bool {
	if info != nil && info.BackendFileGeneration != nil {
		return *info.BackendFileGeneration
	}
	return config.Components.Terraform.AutoGenerateBackendFile
}

// GenerateBackendFileForSection returns a prepared choice carried on a local
// section or the project default for fresh stack descriptions.
func GenerateBackendFileForSection(config *AtmosConfiguration, section map[string]any) bool {
	if choice, ok := section[backendFileGenerationKey].(backendGenerationChoice); ok {
		return choice.enabled
	}
	return config.Components.Terraform.AutoGenerateBackendFile
}
