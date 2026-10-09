package source

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

var labelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// EditOptions specifies a configuration-only operation.
type EditOptions struct {
	Operation, Label, Field, Value, File string
	DryRun                               bool
}

// Edit changes one declaration while preserving the surrounding YAML document.
//
//nolint:gocritic,revive // Copy public options and keep sequential validation next to its I/O operations.
func Edit(config *schema.AtmosConfiguration, o EditOptions) (string, error) {
	if !labelPattern.MatchString(o.Label) {
		return "", fmt.Errorf("%w: label must contain letters, digits, underscores or hyphens", ErrInvalid)
	}
	path := "ai.skills." + o.Label
	file, declaring, err := editFile(config, &o, path)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	updated, err := editDeclaration(raw, path, &o)
	if err != nil {
		return "", err
	}
	if o.Operation != editRemove {
		if err = validateEdited(updated, o.Label); err != nil {
			return "", err
		}
	}
	if o.DryRun {
		return string(updated), nil
	}
	if err = writeAtomic(file, updated); err != nil {
		return "", err
	}
	message := "Updated " + file
	if o.Operation == editRemove && len(declaring) > 1 {
		message += "; other layers still declare " + o.Label + ": " + strings.Join(declaring, ", ")
	}
	return message, nil
}

//nolint:revive // Keep sequential validation and I/O errors next to the operation they guard.
func editFile(config *schema.AtmosConfiguration, o *EditOptions, path string) (string, []string, error) {
	declaring := []string{}
	for _, file := range cfg.EffectiveConfigFilesAscending(config) {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", nil, err
		}
		if hasDeclaration(raw, path) {
			declaring = append(declaring, file)
		}
	}
	if o.Operation == "add" && len(declaring) > 0 {
		return "", nil, fmt.Errorf("%w: declaration %s already exists", ErrInvalid, o.Label)
	}
	if o.Operation == editRemove && o.File == "" && len(declaring) > 1 {
		return "", nil, fmt.Errorf("%w: declaration occurs in %s; select one --config file", ErrInvalid, strings.Join(declaring, ", "))
	}
	if o.File != "" {
		return o.File, declaring, nil
	}
	if len(declaring) > 0 {
		return declaring[len(declaring)-1], declaring, nil
	}
	file, err := cfg.ResolveEditableConfigFile(config, "")
	return file, declaring, err
}

func hasDeclaration(raw []byte, path string) bool {
	value, err := atmosyaml.Get(raw, path)
	return err == nil && value != "null" && value != ""
}

func editDeclaration(raw []byte, path string, o *EditOptions) ([]byte, error) {
	switch o.Operation {
	case "add":
		if hasDeclaration(raw, path) {
			return nil, fmt.Errorf("%w: declaration already exists", ErrInvalid)
		}
		value, err := json.Marshal(map[string]string{"source": o.Value})
		if err != nil {
			return nil, err
		}
		return atmosyaml.SetWithType(raw, path, string(value), atmosyaml.TypeYAML)
	case editRemove:
		return atmosyaml.Delete(raw, path)
	case "set":
		if !hasDeclaration(raw, path) {
			return nil, fmt.Errorf("%w: declaration %s not found", ErrInvalid, o.Label)
		}
		return setField(raw, path, o.Field, o.Value)
	default:
		return nil, ErrInvalid
	}
}

func setField(raw []byte, path, field, value string) ([]byte, error) {
	switch field {
	case "source", "ref", "kind", "subpath", "scope":
		if field == "ref" && strings.HasPrefix(value, "!version ") {
			dependency := strings.TrimSpace(strings.TrimPrefix(value, "!version "))
			if dependency == "" {
				return nil, ErrInvalid
			}
			updated, err := atmosyaml.Set(raw, path+".ref", dependency)
			if err != nil {
				return nil, err
			}
			return atmosyaml.Eval(updated, "."+path+".ref tag = \"!version\"")
		}
		updated, err := atmosyaml.Set(raw, path+"."+field, value)
		if err != nil {
			return nil, err
		}
		return atmosyaml.Eval(updated, "."+path+"."+field+" tag = \"!!str\" | ."+path+"."+field+" style = \"\"")
	case "plugins", "include", "exclude", "clients":
		var list []string
		if err := yaml.Unmarshal([]byte(value), &list); err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(list)
		return atmosyaml.SetWithType(raw, path+"."+field, string(encoded), atmosyaml.TypeYAML)
	default:
		return nil, fmt.Errorf("%w: unknown source field %s", ErrInvalid, field)
	}
}

func validateEdited(raw []byte, label string) error {
	var document struct {
		AI struct {
			Skills map[string]schema.AISkillConfig `yaml:"skills"`
		} `yaml:"ai"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return err
	}
	value := document.AI.Skills[label]
	normalized := normalize(&value)
	return validateDeclaration(&normalized)
}
