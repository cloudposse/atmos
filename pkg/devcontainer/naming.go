// Package devcontainer provides naming and validation for devcontainer instances.
//
// # Naming Convention
//
// Devcontainer names use dot separators to avoid parsing ambiguity:
//
//	Format: atmos-devcontainer.{name}.{instance}
//	Example: atmos-devcontainer.backend-api.test-1
//
// Both name and instance can contain hyphens and underscores without ambiguity.
// The dot separator ensures unambiguous parsing when splitting container names.
//
// For backward compatibility, the parser also supports the legacy hyphen format:
//
//	Legacy: atmos-devcontainer-{name}-{instance}
//	Note: Legacy parsing is best-effort and may be ambiguous with hyphenated names.
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
		return "", errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanationf("Invalid devcontainer name `%s`", name).
			WithHint("Devcontainer names must start with alphanumeric and contain only alphanumeric, hyphens, and underscores").
			WithHintf("Maximum length is %d characters", maxNameLength).
			WithHint("Update the devcontainer name in `atmos.yaml`").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithExample(`components:
  devcontainer:
    my-backend-api:  # Valid: alphanumeric and hyphens
      spec:
        image: golang:latest`).
			WithContext("devcontainer_name", name).
			WithExitCode(2).
			Err()
	}

	if instance == "" {
		instance = DefaultInstance
	}

	if err := ValidateName(instance); err != nil {
		return "", errUtils.Build(errUtils.ErrInvalidDevcontainerConfig).
			WithExplanationf("Invalid instance name `%s`", instance).
			WithHint("Instance names must start with alphanumeric and contain only alphanumeric, hyphens, and underscores").
			WithHintf("Maximum length is %d characters", maxNameLength).
			WithHint("Use valid instance names like `default`, `test-1`, `prod`, etc.").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/").
			WithContext("devcontainer_name", name).
			WithContext("instance", instance).
			WithExitCode(2).
			Err()
	}

	containerName := fmt.Sprintf("%s.%s.%s", ContainerPrefix, name, instance)

	// Validate total container name length (Docker/Podman limit is 253, but 63 for DNS compatibility).
	if len(containerName) > maxNameLength {
		return "", errUtils.Build(errUtils.ErrDevcontainerNameTooLong).
			WithExplanationf("Container name `%s` exceeds %d characters", containerName, maxNameLength).
			WithHintf("Name uses %d chars, instance uses %d chars, prefix uses %d chars", len(name), len(instance), len(ContainerPrefix)+2).
			WithHint("Use shorter devcontainer or instance names").
			WithHint("Consider abbreviating the devcontainer name or using a shorter instance identifier").
			WithHint("See Docker naming constraints: https://docs.docker.com/engine/reference/commandline/create/#name").
			WithContext("devcontainer_name", name).
			WithContext("instance", instance).
			WithContext("container_name", containerName).
			WithContext("total_length", fmt.Sprintf("%d", len(containerName))).
			WithContext("max_length", fmt.Sprintf("%d", maxNameLength)).
			WithExitCode(2).
			Err()
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
		return errUtils.Build(errUtils.ErrDevcontainerNameEmpty).
			WithExplanation("Devcontainer or instance name cannot be empty").
			WithHint("Provide a valid name in `atmos.yaml` configuration").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithExitCode(2).
			Err()
	}

	if !namePattern.MatchString(name) {
		return errUtils.Build(errUtils.ErrDevcontainerNameInvalid).
			WithExplanationf("Name `%s` contains invalid characters or format", name).
			WithHint("Names must start with alphanumeric character").
			WithHint("Names can only contain alphanumeric characters, hyphens, and underscores").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithExample(`Valid names:
  - backend-api
  - my_service
  - app1
  - test-env_2

Invalid names:
  - -backend  (starts with hyphen)
  - api.service  (contains dot)
  - my service  (contains space)`).
			WithContext("name", name).
			WithExitCode(2).
			Err()
	}

	if len(name) > maxNameLength {
		return errUtils.Build(errUtils.ErrDevcontainerNameTooLong).
			WithExplanationf("Name `%s` exceeds maximum length of %d characters", name, maxNameLength).
			WithHintf("Current length: %d characters", len(name)).
			WithHint("Use a shorter name or abbreviation").
			WithHint("See Atmos docs: https://atmos.tools/cli/commands/devcontainer/configuration/").
			WithContext("name", name).
			WithContext("length", fmt.Sprintf("%d", len(name))).
			WithContext("max_length", fmt.Sprintf("%d", maxNameLength)).
			WithExitCode(2).
			Err()
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
