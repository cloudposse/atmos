package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

const (
	// Common log field names.
	logFieldFrom  = "from"
	logFieldTo    = "to"
	logFieldError = "error"
	logFieldType  = "type"

	// NodeIDFormat is the format for node IDs.
	nodeIDFormat = "%s-%s"
)

// DependencyParser handles parsing of component dependencies from configuration.
type DependencyParser struct {
	builder      *dependency.GraphBuilder
	nodeMap      map[string]string
	targetStates map[string]string
	leftDelim    string
}

// NewDependencyParser creates a new dependency parser.
func NewDependencyParser(builder *dependency.GraphBuilder, nodeMap map[string]string, targetStates ...map[string]string) *DependencyParser {
	defer perf.Track(nil, "exec.NewDependencyParser")()

	states := map[string]string(nil)
	if len(targetStates) > 0 {
		states = targetStates[0]
	}
	return &DependencyParser{
		builder:      builder,
		nodeMap:      nodeMap,
		targetStates: states,
	}
}

// NewDependencyParserWithDelimiter creates a dependency parser with a configured template delimiter.
func NewDependencyParserWithDelimiter(builder *dependency.GraphBuilder, nodeMap, targetStates map[string]string, leftDelim string) *DependencyParser {
	defer perf.Track(nil, "exec.NewDependencyParserWithDelimiter")()
	parser := NewDependencyParser(builder, nodeMap, targetStates)
	parser.leftDelim = leftDelim
	return parser
}

// ParseComponentDependencies parses all dependencies from a component's settings.
func (p *DependencyParser) ParseComponentDependencies(
	stackName string,
	componentName string,
	componentSection map[string]any,
) error {
	defer perf.Track(nil, "exec.DependencyParser.ParseComponentDependencies")()

	// Skip abstract and disabled source components.
	if p.shouldSkipComponent(componentSection) {
		return nil
	}
	fromID := fmt.Sprintf(nodeIDFormat, componentName, stackName)

	//nolint:nestif // Modern and legacy dependency surfaces require distinct fallback semantics.
	if dependenciesSection, ok := componentSection[cfg.DependenciesSectionName]; ok {
		depsMap, ok := dependenciesSection.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: dependencies must be a map", errUtils.ErrInvalidDependenciesSection)
		}
		if _, modern := depsMap["components"]; modern {
			dependencies, err := schema.ParseComponentDependencies(depsMap, cfg.TerraformComponentType, stackName)
			if err != nil {
				return fmt.Errorf("%w: parse dependencies: %w", errUtils.ErrDependencyResolution, err)
			}
			for i := range dependencies {
				dep := &dependencies[i]
				if dep.Kind != "" && dep.Kind != cfg.TerraformComponentType {
					continue
				}
				if tags.SelectorUnresolved(dep.Component, p.leftDelim) || tags.SelectorUnresolved(dep.Stack, p.leftDelim) {
					return fmt.Errorf("%w: from=%s component=%s stack=%s", errUtils.ErrDependencyResolution, fromID, dep.Component, dep.Stack)
				}
				if err := p.addModernDependency(fromID, stackName, dep); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Check for dependencies in settings.depends_on, then the historical
	// component-level location accepted for backward compatibility.
	settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any)
	var dependsOn any
	if ok && hasDependencies(settingsSection["depends_on"]) {
		dependsOn = settingsSection["depends_on"]
	}
	if !hasDependencies(dependsOn) {
		dependsOn = componentSection["depends_on"]
	}
	if !hasDependencies(dependsOn) {
		return nil
	}

	// Parse different dependency formats.
	switch deps := dependsOn.(type) {
	case []any:
		p.parseDependencyArray(fromID, stackName, deps)
		return nil
	case map[string]any:
		p.parseDependencyMap(fromID, stackName, deps)
		return nil
	case map[any]any:
		p.parseDependencyMapAnyAny(fromID, stackName, deps)
		return nil
	default:
		log.Warn("Unknown depends_on format", logFieldType, fmt.Sprintf("%T", deps), logFieldFrom, fromID)
		return fmt.Errorf("%w: %s -> unsupported depends_on format %T", errUtils.ErrUnsupportedDependencyType, fromID, deps)
	}
}

func hasDependencies(value any) bool {
	switch dependencies := value.(type) {
	case []any:
		return len(dependencies) > 0
	case map[string]any:
		return len(dependencies) > 0
	case map[any]any:
		return len(dependencies) > 0
	default:
		return value != nil
	}
}

// parseDependencyArray parses dependencies in array format.
func (p *DependencyParser) parseDependencyArray(fromID, defaultStack string, deps []any) {
	for _, dep := range deps {
		if err := p.parseSingleDependency(fromID, defaultStack, dep); err != nil {
			log.Warn("Failed to parse dependency", logFieldFrom, fromID, logFieldError, err)
		}
	}
}

// parseDependencyMap parses dependencies in map format.
func (p *DependencyParser) parseDependencyMap(fromID, defaultStack string, deps map[string]any) {
	for _, dep := range deps {
		if err := p.parseSingleDependency(fromID, defaultStack, dep); err != nil {
			log.Warn("Failed to parse dependency", logFieldFrom, fromID, logFieldError, err)
		}
	}
}

// parseDependencyMapAnyAny parses dependencies in map[any]any format.
func (p *DependencyParser) parseDependencyMapAnyAny(fromID, defaultStack string, deps map[any]any) {
	for _, dep := range deps {
		if err := p.parseSingleDependency(fromID, defaultStack, dep); err != nil {
			log.Warn("Failed to parse dependency", logFieldFrom, fromID, logFieldError, err)
		}
	}
}

// parseSingleDependency parses a single dependency entry.
func (p *DependencyParser) parseSingleDependency(fromID, defaultStack string, dep any) error {
	switch depTyped := dep.(type) {
	case map[string]any:
		return p.parseDependencyMapEntry(fromID, defaultStack, depTyped)
	case map[any]any:
		return p.parseDependencyMapAnyEntry(fromID, defaultStack, depTyped)
	case string:
		// Shorthand: depends_on:
		//   - component
		component := depTyped
		toID := fmt.Sprintf(nodeIDFormat, component, defaultStack)
		return p.addDependencyIfExists(fromID, toID)
	default:
		return fmt.Errorf("%w: %T", errUtils.ErrUnsupportedDependencyType, dep)
	}
}

// parseDependencyMapEntry parses a map[string]any dependency entry.
func (p *DependencyParser) parseDependencyMapEntry(fromID, defaultStack string, depMap map[string]any) error {
	component, ok := depMap["component"].(string)
	if !ok {
		return fmt.Errorf("%w: component", errUtils.ErrMissingDependencyField)
	}

	stack := defaultStack
	if stackVal, ok := depMap["stack"].(string); ok {
		stack = stackVal
	}

	toID := fmt.Sprintf(nodeIDFormat, component, stack)
	return p.addDependencyIfExists(fromID, toID)
}

func (p *DependencyParser) addModernDependency(fromID, defaultStack string, dep *schema.ComponentDependency) error {
	stack := dep.Stack
	if stack == "" {
		stack = defaultStack
	}
	toID := fmt.Sprintf(nodeIDFormat, dep.Component, stack)
	reason, unavailable := p.targetStates[toID]
	if !unavailable {
		_, exists := p.nodeMap[toID]
		unavailable = !exists
		reason = "target_missing"
	}
	if unavailable {
		if dep.IsRequired() {
			targetErr := errUtils.ErrDependencyTargetNotFound
			if reason == "target_disabled" {
				targetErr = errUtils.ErrDependencyTargetUnavailable
			}
			return fmt.Errorf("%w: from=%s to=%s reason=%s", targetErr, fromID, toID, reason)
		}
		log.Info("optional dependency skipped", "event", "optional_dependency_skipped", "from", fromID, "to", toID,
			"reason", reason, "kind", dep.Kind)
		return nil
	}
	if err := p.builder.AddDependencyWithOptional(fromID, toID, !dep.IsRequired()); err != nil {
		return err
	}
	if !dep.IsRequired() {
		log.Debug("optional dependency included", "event", "optional_dependency_included", "from", fromID, "to", toID,
			"kind", dep.Kind)
	}
	return nil
}

// parseDependencyMapAnyEntry parses a map[any]any dependency entry.
func (p *DependencyParser) parseDependencyMapAnyEntry(fromID, defaultStack string, depMap map[any]any) error {
	component, ok := depMap["component"].(string)
	if !ok {
		return fmt.Errorf("%w: component", errUtils.ErrMissingDependencyField)
	}

	stack := defaultStack
	if stackVal, ok := depMap["stack"].(string); ok {
		stack = stackVal
	}

	toID := fmt.Sprintf(nodeIDFormat, component, stack)
	return p.addDependencyIfExists(fromID, toID)
}

// addDependencyIfExists adds a dependency only if the target node exists.
func (p *DependencyParser) addDependencyIfExists(fromID, toID string) error {
	if _, exists := p.nodeMap[toID]; !exists {
		log.Warn("Dependency target not found", logFieldFrom, fromID, logFieldTo, toID)
		return fmt.Errorf("%w: %s", errUtils.ErrDependencyTargetNotFound, toID)
	}

	if err := p.builder.AddDependency(fromID, toID); err != nil {
		log.Warn("Failed to add dependency", logFieldFrom, fromID, logFieldTo, toID, logFieldError, err)
		return err
	}

	return nil
}

// shouldSkipComponent checks if a component should be skipped.
func (p *DependencyParser) shouldSkipComponent(componentSection map[string]any) bool {
	metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return false
	}

	// Skip abstract components.
	if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
		return true
	}

	// Skip disabled components.
	if enabled, ok := metadataSection["enabled"].(bool); ok && !enabled {
		return true
	}

	return false
}
