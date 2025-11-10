// Package devcontainer provides naming and validation for devcontainer instances.
//
// # Naming Convention
//
// Devcontainer names use dot separators to avoid parsing ambiguity:
//   Format: atmos-devcontainer.{name}.{instance}
//   Example: atmos-devcontainer.backend-api.test-1
//
// Both name and instance can contain hyphens and underscores without ambiguity.
// The dot separator ensures unambiguous parsing when splitting container names.
//
// For backward compatibility, the parser also supports the legacy hyphen format:
//   Legacy: atmos-devcontainer-{name}-{instance}
//   Note: Legacy parsing is best-effort and may be ambiguous with hyphenated names.
package devcontainer

import (
	"fmt"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ContainerPrefix is the prefix for all Atmos devcontainer names.
	ContainerPrefix = "atmos-devcontainer"

	// DefaultInstance is the default instance name.
	DefaultInstance = "default"

	// LabelType is the label key for container type.
	LabelType = "com.atmos.type"

	// LabelDevcontainerName is the label key for devcontainer name.
	LabelDevcontainerName = "com.atmos.devcontainer.name"

	// LabelDevcontainerInstance is the label key for devcontainer instance.
	LabelDevcontainerInstance = "com.atmos.devcontainer.instance"

	// LabelWorkspace is the label key for workspace path.
	LabelWorkspace = "com.atmos.workspace"

	// LabelCreated is the label key for creation timestamp.
	LabelCreated = "com.atmos.created"
)

// namePattern validates devcontainer and instance names.
// Allows alphanumeric, hyphens, and underscores.
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// GenerateContainerName generates a container name from devcontainer name and instance.
// Format: atmos-devcontainer.{name}.{instance}.
func GenerateContainerName(name, instance string) (string, error) {
	defer perf.Track(nil, "devcontainer.GenerateContainerName")()

	if err := ValidateName(name); err != nil {
		return "", fmt.Errorf("%w: invalid devcontainer name: %w", errUtils.ErrInvalidDevcontainerConfig, err)
	}

	if instance == "" {
		instance = DefaultInstance
	}

	if err := ValidateName(instance); err != nil {
		return "", fmt.Errorf("%w: invalid instance name: %w", errUtils.ErrInvalidDevcontainerConfig, err)
	}

	containerName := fmt.Sprintf("%s.%s.%s", ContainerPrefix, name, instance)

	// Validate total container name length (Docker/Podman limit is 253, but 63 for DNS compatibility).
	if len(containerName) > maxNameLength {
		return "", fmt.Errorf("%w: container name '%s' exceeds %d characters (name: %d chars, instance: %d chars, prefix: %d chars)",
			errUtils.ErrDevcontainerNameTooLong, containerName, maxNameLength, len(name), len(instance), len(ContainerPrefix)+2)
	}

	return containerName, nil
}

// ParseContainerName parses a container name into devcontainer name and instance.
// Returns empty strings if the name doesn't match the expected format.
//
// Supports both new dot format and legacy hyphen format for backward compatibility:
//   - New format: atmos-devcontainer.{name}.{instance}
//   - Legacy format: atmos-devcontainer-{name}-{instance} (best-effort, may be ambiguous)
func ParseContainerName(containerName string) (name, instance string) {
	defer perf.Track(nil, "devcontainer.ParseContainerName")()

	// Try new dot format first
	if strings.HasPrefix(containerName, ContainerPrefix+".") {
		remainder := strings.TrimPrefix(containerName, ContainerPrefix+".")

		// Split by dot - unambiguous
		parts := strings.SplitN(remainder, ".", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}

		return "", ""
	}

	// Fallback to legacy hyphen format (best-effort)
	if strings.HasPrefix(containerName, ContainerPrefix+"-") {
		remainder := strings.TrimPrefix(containerName, ContainerPrefix+"-")

		// Split into parts
		parts := strings.Split(remainder, "-")
		if len(parts) < 2 {
			return "", ""
		}

		// Last part is instance, everything before is name
		// Note: This is ambiguous if both name and instance contain hyphens
		instance = parts[len(parts)-1]
		name = strings.Join(parts[:len(parts)-1], "-")

		return name, instance
	}

	return "", ""
}

// ValidateName validates a devcontainer or instance name.
const maxNameLength = 63

func ValidateName(name string) error {
	defer perf.Track(nil, "devcontainer.ValidateName")()

	if name == "" {
		return errUtils.ErrDevcontainerNameEmpty
	}

	if !namePattern.MatchString(name) {
		return fmt.Errorf("%w: '%s' must start with alphanumeric and contain only alphanumeric, hyphens, and underscores", errUtils.ErrDevcontainerNameInvalid, name)
	}

	if len(name) > maxNameLength {
		return fmt.Errorf("%w: '%s' exceeds %d characters", errUtils.ErrDevcontainerNameTooLong, name, maxNameLength)
	}

	return nil
}

// IsAtmosDevcontainer checks if a container name is an Atmos devcontainer.
// Supports both new dot format and legacy hyphen format.
func IsAtmosDevcontainer(containerName string) bool {
	defer perf.Track(nil, "devcontainer.IsAtmosDevcontainer")()

	return strings.HasPrefix(containerName, ContainerPrefix+".") ||
		strings.HasPrefix(containerName, ContainerPrefix+"-")
}
