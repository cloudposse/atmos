package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
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
	builder *dependency.GraphBuilder
	nodeMap map[string]string
}

// NewDependencyParser creates a new dependency parser.
func NewDependencyParser(builder *dependency.GraphBuilder, nodeMap map[string]string) *DependencyParser {
	return &DependencyParser{
		builder: builder,
		nodeMap: nodeMap,
	}
}

// ParseComponentDependencies parses all dependencies from a component's settings.
func (p *DependencyParser) ParseComponentDependencies(
	stackName string,
	componentName string,
	componentSection map[string]any,
) error {
	// Skip abstract components.
	if p.shouldSkipComponent(componentSection) {
		return nil
	}

	fromID := fmt.Sprintf(nodeIDFormat, componentName, stackName)

	// Check for dependencies in settings.depends_on.
	settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any)
	if !ok {
		return nil
	}

	dependsOn := settingsSection["depends_on"]
	if dependsOn == nil {
		return nil
	}

	// Parse different dependency formats.
	switch deps := dependsOn.(type) {
	case []any:
		return p.parseDependencyArray(fromID, stackName, deps)
	case map[string]any:
		return p.parseDependencyMap(fromID, stackName, deps)
	case map[any]any:
		return p.parseDependencyMapAnyAny(fromID, stackName, deps)
	default:
		log.Warn("Unknown depends_on format", logFieldType, fmt.Sprintf("%T", deps), logFieldFrom, fromID)
	}

	return nil
}

// parseDependencyArray parses dependencies in array format.
func (p *DependencyParser) parseDependencyArray(fromID, defaultStack string, deps []any) error {
	for _, dep := range deps {
		if err := p.parseSingleDependency(fromID, defaultStack, dep); err != nil {
			log.Warn("Failed to parse dependency", logFieldFrom, fromID, logFieldError, err)
		}
	}
	return nil
}

// parseDependencyMap parses dependencies in map format.
func (p *DependencyParser) parseDependencyMap(fromID, defaultStack string, deps map[string]any) error {
	for _, dep := range deps {
		if err := p.parseSingleDependency(fromID, defaultStack, dep); err != nil {
			log.Warn("Failed to parse dependency", logFieldFrom, fromID, logFieldError, err)
		}
	}
	return nil
}

// parseDependencyMapAnyAny parses dependencies in map[any]any format.
func (p *DependencyParser) parseDependencyMapAnyAny(fromID, defaultStack string, deps map[any]any) error {
	for _, dep := range deps {
		if err := p.parseSingleDependency(fromID, defaultStack, dep); err != nil {
			log.Warn("Failed to parse dependency", logFieldFrom, fromID, logFieldError, err)
		}
	}
	return nil
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
