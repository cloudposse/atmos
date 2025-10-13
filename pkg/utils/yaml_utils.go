package utils

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Atmos YAML functions.
	AtmosYamlFuncExec            = "!exec"
	AtmosYamlFuncStore           = "!store"
	AtmosYamlFuncStoreGet        = "!store.get"
	AtmosYamlFuncTemplate        = "!template"
	AtmosYamlFuncTerraformOutput = "!terraform.output"
	AtmosYamlFuncTerraformState  = "!terraform.state"
	AtmosYamlFuncEnv             = "!env"
	AtmosYamlFuncInclude         = "!include"
	AtmosYamlFuncIncludeRaw      = "!include.raw"
	AtmosYamlFuncGitRoot         = "!repo-root"

	DefaultYAMLIndent = 2
)

var (
	AtmosYamlTags = []string{
		AtmosYamlFuncExec,
		AtmosYamlFuncStore,
		AtmosYamlFuncStoreGet,
		AtmosYamlFuncTemplate,
		AtmosYamlFuncTerraformOutput,
		AtmosYamlFuncTerraformState,
		AtmosYamlFuncEnv,
	}

	// AtmosYamlTagsMap provides O(1) lookup for custom tag checking.
	// This optimization replaces the O(n) SliceContainsString calls that were previously
	// called 75M+ times, causing significant performance overhead.
	atmosYamlTagsMap = map[string]bool{
		AtmosYamlFuncExec:            true,
		AtmosYamlFuncStore:           true,
		AtmosYamlFuncStoreGet:        true,
		AtmosYamlFuncTemplate:        true,
		AtmosYamlFuncTerraformOutput: true,
		AtmosYamlFuncTerraformState:  true,
		AtmosYamlFuncEnv:             true,
	}

	// ParsedYAMLCache stores parsed yaml.Node objects and their position information
	// to avoid re-parsing the same files multiple times.
	// Cache key: file path + content hash.
	parsedYAMLCache   = make(map[string]*parsedYAMLCacheEntry)
	parsedYAMLCacheMu sync.RWMutex

	ErrIncludeYamlFunctionInvalidArguments    = errors.New("invalid number of arguments in the !include function")
	ErrIncludeYamlFunctionInvalidFile         = errors.New("the !include function references a file that does not exist")
	ErrIncludeYamlFunctionInvalidAbsPath      = errors.New("failed to convert the file path to an absolute path in the !include function")
	ErrIncludeYamlFunctionFailedStackManifest = errors.New("failed to process the stack manifest with the !include function")
	ErrNilAtmosConfig                         = errors.New("atmosConfig cannot be nil")
)

// parsedYAMLCacheEntry stores a parsed YAML node and its position information.
type parsedYAMLCacheEntry struct {
	node      yaml.Node
	positions PositionMap
}

// getCachedParsedYAML retrieves a cached parsed YAML node if it exists.
// Returns a copy of the node to prevent external mutations.
// Note: perf.Track() removed from this hot path to reduce overhead.
func getCachedParsedYAML(file string) (*yaml.Node, PositionMap, bool) {
	if file == "" {
		return nil, nil, false
	}

	parsedYAMLCacheMu.RLock()
	defer parsedYAMLCacheMu.RUnlock()

	entry, found := parsedYAMLCache[file]
	if !found {
		return nil, nil, false
	}

	// Return a copy of the node to prevent mutations affecting the cache.
	nodeCopy := entry.node
	return &nodeCopy, entry.positions, true
}

// cacheParsedYAML stores a parsed YAML node in the cache.
// Stores a copy to prevent external mutations from affecting the cache.
// Note: perf.Track() removed from this hot path to reduce overhead.
func cacheParsedYAML(file string, node *yaml.Node, positions PositionMap) {
	if file == "" || node == nil {
		return
	}

	parsedYAMLCacheMu.Lock()
	defer parsedYAMLCacheMu.Unlock()

	// Store a copy to prevent external mutations from affecting the cache.
	nodeCopy := *node
	parsedYAMLCache[file] = &parsedYAMLCacheEntry{
		node:      nodeCopy,
		positions: positions,
	}
}

// PrintAsYAML prints the provided value as YAML document to the console
func PrintAsYAML(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsYAML")()

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
	defer perf.Track(atmosConfig, "utils.PrintAsYAMLWithConfig")()

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
	defer perf.Track(atmosConfig, "utils.GetHighlightedYAML")()

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
	defer perf.Track(atmosConfig, "utils.PrintAsYAMLToFileDescriptor")()

	if atmosConfig == nil {
		return ErrNilAtmosConfig
	}

	indent := getIndentFromConfig(atmosConfig)
	y, err := ConvertToYAML(data, YAMLOptions{Indent: indent})
	if err != nil {
		return err
	}

	log.Debug("PrintAsYAMLToFileDescriptor", "data", y)
	return nil
}

// WriteToFileAsYAML converts the provided value to YAML and writes it to the specified file
func WriteToFileAsYAML(filePath string, data any, fileMode os.FileMode) error {
	defer perf.Track(nil, "utils.WriteToFileAsYAML")()

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
	defer perf.Track(atmosConfig, "utils.WriteToFileAsYAMLWithConfig")()

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

// LongString is a string type that encodes as a YAML folded scalar (>).
// This is used to wrap long strings across multiple lines for better readability.
type LongString string

// MarshalYAML implements yaml.Marshaler to encode as a folded scalar.
func (s LongString) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Style: yaml.FoldedStyle, // Use > style for folded scalar
		Value: string(s),
	}
	return node, nil
}

// WrapLongStrings walks a data structure and converts strings longer than maxLength
// to LongString type, which will be encoded as YAML folded scalars (>) for better readability.
func WrapLongStrings(data any, maxLength int) any {
	defer perf.Track(nil, "utils.WrapLongStrings")()

	if maxLength <= 0 {
		return data
	}

	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, value := range v {
			result[key] = WrapLongStrings(value, maxLength)
		}
		return result

	case []any:
		result := make([]any, len(v))
		for i, value := range v {
			result[i] = WrapLongStrings(value, maxLength)
		}
		return result

	case string:
		// Convert long single-line strings to LongString
		if len(v) > maxLength && !strings.Contains(v, "\n") {
			return LongString(v)
		}
		return v

	default:
		// For all other types (int, bool, etc.), return as-is
		return data
	}
}

func ConvertToYAML(data any, opts ...YAMLOptions) (string, error) {
	defer perf.Track(nil, "utils.ConvertToYAML")()

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

//nolint:gocognit,revive
func processCustomTags(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string) error {
	defer perf.Track(atmosConfig, "utils.processCustomTags")()

	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return processCustomTags(atmosConfig, node.Content[0], file)
	}

	// Early exit: skip processing if this subtree has no custom tags.
	// This avoids expensive recursive processing for YAML subtrees that don't use custom tags.
	// Most YAML content doesn't use custom tags, so this optimization significantly reduces
	// unnecessary recursion and tag checking.
	if !hasCustomTags(node) {
		return nil
	}

	for _, n := range node.Content {
		tag := strings.TrimSpace(n.Tag)
		val := strings.TrimSpace(n.Value)

		// Use O(1) map lookup instead of O(n) slice search for performance.
		// This optimization reduces 75M+ linear searches to constant-time lookups.
		if atmosYamlTagsMap[tag] {
			n.Value = getValueWithTag(n)
			// Clear the custom tag to prevent the YAML decoder from processing it again.
			// We keep the value as is since it will be processed later by processCustomTags.
			// We don't set a specific type tag (like !!str) because the function might return
			// any type (string, map, list, etc.) when it's actually executed.
			n.Tag = ""
		}

		// Handle the !include tag with extension-based parsing
		if tag == AtmosYamlFuncInclude {
			if err := ProcessIncludeTag(atmosConfig, n, val, file); err != nil {
				return err
			}
		}

		// Handle the !include.raw tag (always returns raw string)
		if tag == AtmosYamlFuncIncludeRaw {
			if err := ProcessIncludeRawTag(atmosConfig, n, val, file); err != nil {
				return err
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

// hasCustomTags performs a fast scan to check if a node or any of its children contain custom Atmos tags.
// This enables early exit optimization in processCustomTags, avoiding expensive recursive processing
// for YAML subtrees that don't use custom tags (which is the majority of YAML content).
func hasCustomTags(node *yaml.Node) bool {
	if node == nil {
		return false
	}

	// Check if this node has a custom tag.
	tag := strings.TrimSpace(node.Tag)
	if atmosYamlTagsMap[tag] || tag == AtmosYamlFuncInclude || tag == AtmosYamlFuncIncludeRaw {
		return true
	}

	// Recursively check children.
	for _, child := range node.Content {
		if hasCustomTags(child) {
			return true
		}
	}

	return false
}

// UnmarshalYAML unmarshals YAML into a Go type.
func UnmarshalYAML[T any](input string) (T, error) {
	return UnmarshalYAMLFromFile[T](&schema.AtmosConfiguration{}, input, "")
}

// UnmarshalYAMLFromFile unmarshals YAML downloaded from a file into a Go type.
func UnmarshalYAMLFromFile[T any](atmosConfig *schema.AtmosConfiguration, input string, file string) (T, error) {
	defer perf.Track(atmosConfig, "utils.UnmarshalYAMLFromFile")()

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

// UnmarshalYAMLFromFileWithPositions unmarshals YAML and returns position information.
// The positions map contains line/column information for each value in the YAML.
// If atmosConfig.TrackProvenance is false, returns an empty position map.
// Uses caching to avoid re-parsing the same files multiple times.
func UnmarshalYAMLFromFileWithPositions[T any](atmosConfig *schema.AtmosConfiguration, input string, file string) (T, PositionMap, error) {
	defer perf.Track(atmosConfig, "utils.UnmarshalYAMLFromFileWithPositions")()

	if atmosConfig == nil {
		return *new(T), nil, ErrNilAtmosConfig
	}

	var zeroValue T

	// Try to get cached parsed YAML first.
	node, positions, found := getCachedParsedYAML(file)
	if !found {
		// Cache miss - parse the YAML.
		var parsedNode yaml.Node
		b := []byte(input)

		// Unmarshal into yaml.Node.
		if err := yaml.Unmarshal(b, &parsedNode); err != nil {
			return zeroValue, nil, err
		}

		// Extract positions if provenance tracking is enabled.
		if atmosConfig.TrackProvenance {
			positions = ExtractYAMLPositions(&parsedNode, true)
		}

		// Process custom tags.
		if err := processCustomTags(atmosConfig, &parsedNode, file); err != nil {
			return zeroValue, nil, err
		}

		// Cache the parsed and processed node.
		cacheParsedYAML(file, &parsedNode, positions)
		node = &parsedNode
	}

	// Decode the yaml.Node into the desired type T.
	var data T
	if err := node.Decode(&data); err != nil {
		return zeroValue, nil, err
	}

	return data, positions, nil
}
