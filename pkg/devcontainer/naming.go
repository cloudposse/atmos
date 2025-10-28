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
// Format: atmos-devcontainer-{name}-{instance}.
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

	return fmt.Sprintf("%s-%s-%s", ContainerPrefix, name, instance), nil
}

// ParseContainerName parses a container name into devcontainer name and instance.
// Returns empty strings if the name doesn't match the expected format.
func ParseContainerName(containerName string) (name, instance string) {
	defer perf.Track(nil, "devcontainer.ParseContainerName")()

	// Remove prefix
	if !strings.HasPrefix(containerName, ContainerPrefix+"-") {
		return "", ""
	}

	remainder := strings.TrimPrefix(containerName, ContainerPrefix+"-")

	// Split into parts
	parts := strings.Split(remainder, "-")
	if len(parts) < 2 {
		return "", ""
	}

	// Last part is instance, everything before is name
	instance = parts[len(parts)-1]
	name = strings.Join(parts[:len(parts)-1], "-")

	return name, instance
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
func IsAtmosDevcontainer(containerName string) bool {
	defer perf.Track(nil, "devcontainer.IsAtmosDevcontainer")()

	return strings.HasPrefix(containerName, ContainerPrefix+"-")
}
