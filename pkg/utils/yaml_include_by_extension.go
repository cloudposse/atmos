package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessIncludeTag processes the !include tag with extension-based parsing.
// It parses files based on their extension, not their content.
func ProcessIncludeTag(
	atmosConfig *schema.AtmosConfiguration,
	node *yaml.Node,
	val string,
	file string,
) error {
	return processIncludeTagInternal(atmosConfig, node, val, file, false)
}

// ProcessIncludeRawTag processes the !include.raw tag.
// It always returns the file content as a raw string, regardless of extension.
func ProcessIncludeRawTag(
	atmosConfig *schema.AtmosConfiguration,
	node *yaml.Node,
	val string,
	file string,
) error {
	return processIncludeTagInternal(atmosConfig, node, val, file, true)
}

// processIncludeTagInternal handles both !include and !include.raw tags.
func processIncludeTagInternal(
	atmosConfig *schema.AtmosConfiguration,
	node *yaml.Node,
	val string,
	file string,
	forceRaw bool,
) error {
	var includeFile string
	var includeQuery string
	var res any
	var localFile string

	// Parse the include arguments
	parts, err := SplitStringByDelimiter(val, ' ')
	if err != nil {
		return err
	}

	partsLen := len(parts)

	switch partsLen {
	case 2:
		includeFile = strings.TrimSpace(parts[0])
		includeQuery = strings.TrimSpace(parts[1])
	case 1:
		includeFile = strings.TrimSpace(parts[0])
	default:
		return fmt.Errorf("%w: %s, stack manifest: %s", ErrIncludeYamlFunctionInvalidArguments, val, file)
	}

	// Try to find the file locally
	localFile = findLocalFile(includeFile, file, atmosConfig)

	// Process the file
	if localFile != "" {
		// Process local file
		res, err = processLocalFile(localFile, forceRaw)
		if err != nil {
			return err
		}
	} else {
		// Process remote file
		res, err = processRemoteFile(atmosConfig, includeFile, forceRaw)
		if err != nil {
			return err
		}
	}

	// Apply YQ expression if provided
	if includeQuery != "" {
		res, err = EvaluateYqExpression(atmosConfig, res, includeQuery)
		if err != nil {
			return err
		}
	}

	// Update the YAML node with the result
	return updateYamlNode(node, res, val, file)
}

// isRemoteURL checks if the path is a remote URL.
func isRemoteURL(path string) bool {
	remoteProtocols := []string{"http://", "https://", "s3://", "gcs://", "git://", "oci://", "scp://", "sftp://"}
	for _, protocol := range remoteProtocols {
		if strings.HasPrefix(path, protocol) {
			return true
		}
	}
	return strings.Contains(path, "::")
}

// resolveAbsolutePath checks if a file exists and returns its absolute path.
func resolveAbsolutePath(path string) string {
	if !FileExists(path) {
		return ""
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return absPath
}

// findLocalFile attempts to find a local file from various possible paths.
func findLocalFile(includeFile, manifestFile string, atmosConfig *schema.AtmosConfiguration) string {
	// Check if it's a URL - if so, it's not a local file
	if isRemoteURL(includeFile) {
		return ""
	}

	// If absolute path is provided, check if the file exists
	if filepath.IsAbs(includeFile) {
		return resolveAbsolutePath(includeFile)
	}

	// Try relative to the manifest file
	resolved := ResolveRelativePath(includeFile, manifestFile)
	if absPath := resolveAbsolutePath(resolved); absPath != "" {
		return absPath
	}

	// Try relative to the base_path from atmos.yaml
	atmosManifestPath := filepath.Join(atmosConfig.BasePath, includeFile)
	return resolveAbsolutePath(atmosManifestPath)
}

// processLocalFile reads and parses a local file.
func processLocalFile(localFile string, forceRaw bool) (any, error) {
	if forceRaw {
		// Always return raw content for !include.raw
		return filetype.ParseFileRaw(os.ReadFile, localFile)
	}
	// Use extension-based parsing for regular !include
	return filetype.ParseFileByExtension(os.ReadFile, localFile)
}

// processRemoteFile downloads and parses a remote file.
func processRemoteFile(atmosConfig *schema.AtmosConfiguration, includeFile string, forceRaw bool) (any, error) {
	dl := downloader.NewGoGetterDownloader(atmosConfig)

	if forceRaw {
		// Always return raw content for !include.raw
		return dl.FetchAndParseRaw(includeFile)
	}
	// Use extension-based parsing for regular !include
	return dl.FetchAndParseByExtension(includeFile)
}

// handleCommentString updates the node for string values that start with '#'.
func handleCommentString(node *yaml.Node, strVal string) {
	node.Kind = yaml.ScalarNode
	node.Tag = "!!str"
	node.Value = strVal
	node.Style = yaml.SingleQuotedStyle
}

// unmarshalYamlContent unmarshals YAML content and extracts the document content.
func unmarshalYamlContent(y string, val string, file string) (*yaml.Node, error) {
	var includedNode yaml.Node
	err := yaml.Unmarshal([]byte(y), &includedNode)
	if err != nil {
		return nil, fmt.Errorf("%w: %s, stack manifest: %s, error: %v",
			ErrIncludeYamlFunctionFailedStackManifest, val, file, err)
	}

	// yaml.Unmarshal creates a DocumentNode, we need to use its content
	if includedNode.Kind == yaml.DocumentNode {
		if len(includedNode.Content) == 0 {
			return nil, fmt.Errorf("%w: %s, stack manifest: %s, error: empty document",
				ErrIncludeYamlFunctionFailedStackManifest, val, file)
		}
		return includedNode.Content[0], nil
	}
	return &includedNode, nil
}

// updateYamlNode updates the YAML node with the processed result.
func updateYamlNode(node *yaml.Node, res any, val string, file string) error {
	// Handle string values that start with '#' (YAML comments)
	if strVal, ok := res.(string); ok && strings.HasPrefix(strVal, "#") {
		handleCommentString(node, strVal)
		return nil
	}

	// Convert result to YAML and update the node
	y, err := ConvertToYAML(res)
	if err != nil {
		return err
	}

	contentNode, err := unmarshalYamlContent(y, val, file)
	if err != nil {
		return err
	}

	*node = *contentNode
	return nil
}
