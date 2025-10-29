package agents

const (
	// Built-in agent names.
	GeneralAgent           = "general"
	StackAnalyzerAgent     = "stack-analyzer"
	ComponentRefactorAgent = "component-refactor"
	SecurityAuditorAgent   = "security-auditor"
	ConfigValidatorAgent   = "config-validator"
)

// GetBuiltInAgents returns all built-in agents.
func GetBuiltInAgents() []*Agent {
	return []*Agent{
		getGeneralAgent(),
		getStackAnalyzerAgent(),
		getComponentRefactorAgent(),
		getSecurityAuditorAgent(),
		getConfigValidatorAgent(),
	}
}

// getGeneralAgent returns the general-purpose agent (default).
func getGeneralAgent() *Agent {
	return &Agent{
		Name:             GeneralAgent,
		DisplayName:      "General",
		Description:      "General-purpose assistant for all Atmos operations",
		SystemPromptPath: "general.md", // Load from embedded filesystem
		AllowedTools:     []string{},   // Empty = all tools allowed
		RestrictedTools:  []string{},
		Category:         "general",
		IsBuiltIn:        true,
	}
}

// getStackAnalyzerAgent returns the stack analysis agent.
func getStackAnalyzerAgent() *Agent {
	return &Agent{
		Name:             StackAnalyzerAgent,
		DisplayName:      "Stack Analyzer",
		Description:      "Specialized agent for analyzing Atmos stack configurations, dependencies, and architecture",
		SystemPromptPath: "stack-analyzer.md", // Load from embedded filesystem
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

// getComponentRefactorAgent returns the component refactoring agent.
func getComponentRefactorAgent() *Agent {
	return &Agent{
		Name:             ComponentRefactorAgent,
		DisplayName:      "Component Refactor",
		Description:      "Specialized agent for refactoring Terraform/Helmfile component code",
		SystemPromptPath: "component-refactor.md", // Load from embedded filesystem
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

// getSecurityAuditorAgent returns the security audit agent.
func getSecurityAuditorAgent() *Agent {
	return &Agent{
		Name:             SecurityAuditorAgent,
		DisplayName:      "Security Auditor",
		Description:      "Specialized agent for security review of infrastructure configurations",
		SystemPromptPath: "security-auditor.md", // Load from embedded filesystem
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

// getConfigValidatorAgent returns the configuration validation agent.
func getConfigValidatorAgent() *Agent {
	return &Agent{
		Name:             ConfigValidatorAgent,
		DisplayName:      "Config Validator",
		Description:      "Specialized agent for validating Atmos configuration files",
		SystemPromptPath: "config-validator.md", // Load from embedded filesystem
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
