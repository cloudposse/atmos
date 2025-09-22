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

	if partsLen == 2 {
		includeFile = strings.TrimSpace(parts[0])
		includeQuery = strings.TrimSpace(parts[1])
	} else if partsLen == 1 {
		includeFile = strings.TrimSpace(parts[0])
	} else {
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

// findLocalFile attempts to find a local file from various possible paths.
func findLocalFile(includeFile, manifestFile string, atmosConfig *schema.AtmosConfiguration) string {
	// Check if it's a URL - if so, it's not a local file
	if strings.HasPrefix(includeFile, "http://") || strings.HasPrefix(includeFile, "https://") ||
		strings.HasPrefix(includeFile, "s3://") || strings.HasPrefix(includeFile, "gcs://") ||
		strings.HasPrefix(includeFile, "git://") || strings.HasPrefix(includeFile, "oci://") ||
		strings.HasPrefix(includeFile, "scp://") || strings.HasPrefix(includeFile, "sftp://") ||
		strings.Contains(includeFile, "::") {
		return ""
	}

	// If absolute path is provided, check if the file exists
	if filepath.IsAbs(includeFile) {
		if FileExists(includeFile) {
			return includeFile
		}
		return ""
	}

	// Try relative to the manifest file
	resolved := ResolveRelativePath(includeFile, manifestFile)
	if FileExists(resolved) {
		resolvedAbsolutePath, err := filepath.Abs(resolved)
		if err == nil {
			return resolvedAbsolutePath
		}
	}

	// Try relative to the base_path from atmos.yaml
	atmosManifestPath := filepath.Join(atmosConfig.BasePath, includeFile)
	if FileExists(atmosManifestPath) {
		atmosManifestAbsolutePath, err := filepath.Abs(atmosManifestPath)
		if err == nil {
			return atmosManifestAbsolutePath
		}
	}

	return ""
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

// updateYamlNode updates the YAML node with the processed result.
func updateYamlNode(node *yaml.Node, res any, val string, file string) error {
	// Handle string values that start with '#' (YAML comments)
	if strVal, ok := res.(string); ok && strings.HasPrefix(strVal, "#") {
		node.Kind = yaml.ScalarNode
		node.Tag = "!!str"
		node.Value = strVal
		node.Style = yaml.SingleQuotedStyle
	} else {
		// Convert result to YAML and update the node
		y, err := ConvertToYAML(res)
		if err != nil {
			return err
		}

		var includedNode yaml.Node
		err = yaml.Unmarshal([]byte(y), &includedNode)
		if err != nil {
			return fmt.Errorf("%w: %s, stack manifest: %s, error: %v",
				ErrIncludeYamlFunctionFailedStackManifest, val, file, err)
		}

		*node = includedNode
	}
	return nil
}
