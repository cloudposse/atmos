package utils

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Atmos YAML functions.
	AtmosYamlFuncExec            = "!exec"
	AtmosYamlFuncStore           = "!store"
	AtmosYamlFuncTemplate        = "!template"
	AtmosYamlFuncTerraformOutput = "!terraform.output"
	AtmosYamlFuncEnv             = "!env"
	AtmosYamlFuncInclude         = "!include"
	AtmosYamlFuncGitRoot         = "!repo-root"

	DefaultYAMLIndent = 2
)

var (
	AtmosYamlTags = []string{
		AtmosYamlFuncExec,
		AtmosYamlFuncStore,
		AtmosYamlFuncTemplate,
		AtmosYamlFuncTerraformOutput,
		AtmosYamlFuncEnv,
	}

	ErrIncludeYamlFunctionInvalidArguments    = errors.New("invalid number of arguments in the !include function")
	ErrIncludeYamlFunctionInvalidFile         = errors.New("the !include function references a file that does not exist")
	ErrIncludeYamlFunctionInvalidAbsPath      = errors.New("failed to convert the file path to an absolute path in the !include function")
	ErrIncludeYamlFunctionFailedStackManifest = errors.New("failed to process the stack manifest with the !include function")
	ErrNilAtmosConfig                         = errors.New("atmosConfig cannot be nil")
)

// PrintAsYAML prints the provided value as YAML document to the console
func PrintAsYAML(atmosConfig *schema.AtmosConfiguration, data any) error {
	y, err := GetHighlightedYAML(atmosConfig, data)
	if err != nil {
		return err
	}
	PrintMessage(y)
	return nil
}

func getIndentFromConfig(atmosConfig *schema.AtmosConfiguration) int {
	if atmosConfig == nil || atmosConfig.Settings.Terminal.TabWidth <= 0 {
		return DefaultYAMLIndent
	}
	return atmosConfig.Settings.Terminal.TabWidth
}

func PrintAsYAMLWithConfig(atmosConfig *schema.AtmosConfiguration, data any) error {
	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}

	highlighted, err := HighlightCodeWithConfig(atmosConfig, y, "yaml")
	if err != nil {
		PrintMessage(y)
		return nil
	}
	PrintMessage(highlighted)
	return nil
}

func GetHighlightedYAML(atmosConfig *schema.AtmosConfiguration, data any) (string, error) {
	y, err := ConvertToYAML(data)
	if err != nil {
		return "", err
	}
	highlighted, err := HighlightCodeWithConfig(atmosConfig, y)
	if err != nil {
		return y, err
	}
	return highlighted, nil
}

// PrintAsYAMLToFileDescriptor prints the provided value as YAML document to a file descriptor
func PrintAsYAMLToFileDescriptor(atmosConfig *schema.AtmosConfiguration, data any) error {
	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	log.Debug("PrintAsYAMLToFileDescriptor", "tabWidth", indent)

	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}
	LogInfo(y)
	return nil
}

// WriteToFileAsYAML converts the provided value to YAML and writes it to the specified file
func WriteToFileAsYAML(filePath string, data any, fileMode os.FileMode) error {
	y, err := ConvertToYAML(data, YAMLOptions{Indent: DefaultYAMLIndent})
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, []byte(y), fileMode)
	if err != nil {
		return err
	}
	return nil
}

func WriteToFileAsYAMLWithConfig(atmosConfig *schema.AtmosConfiguration, filePath string, data any, fileMode os.FileMode) error {
	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	log.Debug("WriteToFileAsYAMLWithConfig", "tabWidth", indent, "filePath", filePath)

	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, []byte(y), fileMode)
	if err != nil {
		return err
	}
	return nil
}

type YAMLOptions struct {
	Indent int
}

func ConvertToYAML(data any, opts ...YAMLOptions) (string, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)

	indent := DefaultYAMLIndent
	if len(opts) > 0 && opts[0].Indent > 0 {
		indent = opts[0].Indent
	}
	encoder.SetIndent(indent)

	if err := encoder.Encode(data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func processCustomTags(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return processCustomTags(atmosConfig, node.Content[0], file)
	}

	for _, n := range node.Content {
		tag := strings.TrimSpace(n.Tag)
		val := strings.TrimSpace(n.Value)

		if SliceContainsString(AtmosYamlTags, tag) {
			n.Value = getValueWithTag(n)
		}

		// Handle the !include tag
		if tag == AtmosYamlFuncInclude {
			var includeFile string
			var includeQuery string
			var res any
			var localFile string

			parts, err := SplitStringByDelimiter(val, ' ')
			if err != nil {
				return err
			}

			partsLen := len(parts)

			if partsLen == 2 {
				includeFile = strings.TrimSpace(parts[0])
				includeQuery = strings.TrimSpace(parts[1])
			} else if partsLen == 1 {
				includeFile = strings.TrimSpace(parts[0])
			} else {
				return fmt.Errorf("%w: %s, stack manifest: %s", ErrIncludeYamlFunctionInvalidArguments, val, file)
			}

			// If absolute path is provided, check if the file exists
			if filepath.IsAbs(includeFile) {
				if !FileExists(includeFile) {
					return fmt.Errorf("%w: %s, stack manifest: %s", ErrIncludeYamlFunctionInvalidFile, val, file)
				}
				localFile = includeFile
			}

			// Detect relative paths (relative to the manifest file) and convert to absolute paths
			if localFile == "" {
				resolved := ResolveRelativePath(includeFile, file)
				if FileExists(resolved) {
					resolvedAbsolutePath, err := filepath.Abs(resolved)
					if err != nil {
						return fmt.Errorf("%w: %s, stack manifest: %s, error: %v", ErrIncludeYamlFunctionInvalidAbsPath, val, file, err)
					}
					localFile = resolvedAbsolutePath
				}
			}

			// Check if the `!include` function points to an Atmos stack manifest relative to the `base_path` defined in `atmos.yaml`
			if localFile == "" {
				atmosManifestPath := filepath.Join(atmosConfig.BasePath, includeFile)
				if FileExists(atmosManifestPath) {
					atmosManifestAbsolutePath, err := filepath.Abs(atmosManifestPath)
					if err != nil {
						return fmt.Errorf("%w: %s, stack manifest: %s, error: %v", ErrIncludeYamlFunctionInvalidAbsPath, val, file, err)
					}
					localFile = atmosManifestAbsolutePath
				}
			}

			// Process local file
			if localFile != "" {
				res, err = filetype.DetectFormatAndParseFile(os.ReadFile, localFile)
				if err != nil {
					return err
				}
			} else {
				// Process remote file with `go-getter`
				res, err = downloader.NewGoGetterDownloader(atmosConfig).FetchAndAutoParse(includeFile)
				if err != nil {
					return err
				}
			}

			// Evaluate the YQ expression if provided
			if includeQuery != "" {
				res, err = EvaluateYqExpression(atmosConfig, res, includeQuery)
				if err != nil {
					return err
				}
			}

			if strVal, ok := res.(string); ok && strings.HasPrefix(strVal, "#") {
				n.Kind = yaml.ScalarNode
				n.Tag = "!!str"
				n.Value = strVal
				n.Style = yaml.SingleQuotedStyle
			} else {
				y, err := ConvertToYAML(res)
				if err != nil {
					return err
				}

				var includedNode yaml.Node
				err = yaml.Unmarshal([]byte(y), &includedNode)
				if err != nil {
					return fmt.Errorf("%w: %s, stack manifest: %s, error: %v", ErrIncludeYamlFunctionFailedStackManifest, val, file, err)
				}

				*n = includedNode
			}
		}

		// Recursively process the child nodes
		if len(n.Content) > 0 {
			if err := processCustomTags(atmosConfig, n, file); err != nil {
				return err
			}
		}
	}
	return nil
}

func getValueWithTag(n *yaml.Node) string {
	tag := strings.TrimSpace(n.Tag)
	val := strings.TrimSpace(n.Value)
	return strings.TrimSpace(tag + " " + val)
}

// UnmarshalYAML unmarshals YAML into a Go type.
func UnmarshalYAML[T any](input string) (T, error) {
	return UnmarshalYAMLFromFile[T](&schema.AtmosConfiguration{}, input, "")
}

// UnmarshalYAMLFromFile unmarshals YAML downloaded from a file into a Go type.
func UnmarshalYAMLFromFile[T any](atmosConfig *schema.AtmosConfiguration, input string, file string) (T, error) {
	if atmosConfig == nil {
		return *new(T), ErrNilAtmosConfig
	}

	var zeroValue T
	var node yaml.Node
	b := []byte(input)

	// Unmarshal into yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return zeroValue, err
	}

	if err := processCustomTags(atmosConfig, &node, file); err != nil {
		return zeroValue, err
	}

	// Decode the yaml.Node into the desired type T
	var data T
	if err := node.Decode(&data); err != nil {
		return zeroValue, err
	}

	return data, nil
}
