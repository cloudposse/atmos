package skills

const (
	// Built-in skill names.
	GeneralSkill           = "general"
	StackAnalyzerSkill     = "stack-analyzer"
	ComponentRefactorSkill = "component-refactor"
	SecurityAuditorSkill   = "security-auditor"
	ConfigValidatorSkill   = "config-validator"
)

// GetBuiltInSkills returns all built-in skills.
func GetBuiltInSkills() []*Skill {
	return []*Skill{
		getGeneralSkill(),
		getStackAnalyzerSkill(),
		getComponentRefactorSkill(),
		getSecurityAuditorSkill(),
		getConfigValidatorSkill(),
	}
}

// getGeneralSkill returns the general-purpose skill (default).
func getGeneralSkill() *Skill {
	return &Skill{
		Name:             GeneralSkill,
		DisplayName:      "General",
		Description:      "General-purpose assistant for all Atmos operations",
		SystemPromptPath: "general/SKILL.md", // Load from embedded filesystem.
		AllowedTools:     []string{},         // Empty = all tools allowed.
		RestrictedTools:  []string{},
		Category:         "general",
		IsBuiltIn:        true,
	}
}

// getStackAnalyzerSkill returns the stack analysis skill.
func getStackAnalyzerSkill() *Skill {
	return &Skill{
		Name:             StackAnalyzerSkill,
		DisplayName:      "Stack Analyzer",
		Description:      "Specialized skill for analyzing Atmos stack configurations, dependencies, and architecture",
		SystemPromptPath: "stack-analyzer/SKILL.md", // Load from embedded filesystem.
		AllowedTools: []string{
			"read_file",
			"search_files",
			"execute_atmos_command",
			"grep",
		},
		RestrictedTools: []string{},
		Category:        "analysis",
		IsBuiltIn:       true,
	}
}

// getComponentRefactorSkill returns the component refactoring skill.
func getComponentRefactorSkill() *Skill {
	return &Skill{
		Name:             ComponentRefactorSkill,
		DisplayName:      "Component Refactor",
		Description:      "Specialized skill for refactoring Terraform/Helmfile component code",
		SystemPromptPath: "component-refactor/SKILL.md", // Load from embedded filesystem.
		AllowedTools: []string{
			"read_file",
			"edit_file",
			"search_files",
			"execute_atmos_command",
			"grep",
		},
		RestrictedTools: []string{},
		Category:        "refactor",
		IsBuiltIn:       true,
	}
}

// getSecurityAuditorSkill returns the security audit skill.
func getSecurityAuditorSkill() *Skill {
	return &Skill{
		Name:             SecurityAuditorSkill,
		DisplayName:      "Security Auditor",
		Description:      "Specialized skill for security review of infrastructure configurations",
		SystemPromptPath: "security-auditor/SKILL.md", // Load from embedded filesystem.
		AllowedTools: []string{
			"read_file",
			"search_files",
			"execute_atmos_command",
			"grep",
		},
		RestrictedTools: []string{},
		Category:        "security",
		IsBuiltIn:       true,
	}
}

// getConfigValidatorSkill returns the configuration validation skill.
func getConfigValidatorSkill() *Skill {
	return &Skill{
		Name:             ConfigValidatorSkill,
		DisplayName:      "Config Validator",
		Description:      "Specialized skill for validating Atmos configuration files",
		SystemPromptPath: "config-validator/SKILL.md", // Load from embedded filesystem.
		AllowedTools: []string{
			"read_file",
			"execute_atmos_command",
			"search_files",
			"grep",
		},
		RestrictedTools: []string{},
		Category:        "validation",
		IsBuiltIn:       true,
	}
}
