package utils

import (
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-getter"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/function/parser"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestHTTPClient allows tests to inject a custom HTTP client for remote file fetching.
// This is used to mock GitHub requests in tests.
var TestHTTPClient *http.Client

// ProcessIncludeTag processes the !include tag with extension-based parsing.
// It parses files based on their extension, not their content.
func ProcessIncludeTag(
	atmosConfig *schema.AtmosConfiguration,
	node *yaml.Node,
	val string,
	file string,
) error {
	defer perf.Track(atmosConfig, "utils.ProcessIncludeTag")()

	_, err := processIncludeTagInternal(atmosConfig, node, val, file, includeMode{})
	return err
}

// ProcessIncludeTagWalked is ProcessIncludeTag for callers that own a tag
// walker: when the include is a local YAML file evaluated with `| eval`, the
// spliced subtree is walked through walkIn with the included file as its
// context, so a nested `!include ./x` resolves relative to that file rather
// than the outer manifest. The returned bool reports that the subtree has
// already been walked and the caller must not recurse into it again.
func ProcessIncludeTagWalked(
	atmosConfig *schema.AtmosConfiguration,
	node *yaml.Node,
	val string,
	file string,
	walkIn func(node *yaml.Node, file string) error,
) (bool, error) {
	defer perf.Track(atmosConfig, "utils.ProcessIncludeTagWalked")()

	return processIncludeTagInternal(atmosConfig, node, val, file, includeMode{walkIn: walkIn})
}

// includeMode carries the caller-selected include behavior.
type includeMode struct {
	// forceRaw returns the file text verbatim (!include.raw).
	forceRaw bool
	// walkIn, when set, walks an evaluated local YAML include with the
	// included file as the tag-resolution context.
	walkIn func(node *yaml.Node, file string) error
}

// ProcessIncludeRawTag processes the !include.raw tag.
// It always returns the file content as a raw string, regardless of extension.
func ProcessIncludeRawTag(
	atmosConfig *schema.AtmosConfiguration,
	node *yaml.Node,
	val string,
	file string,
) error {
	defer perf.Track(atmosConfig, "utils.ProcessIncludeRawTag")()

	_, err := processIncludeTagInternal(atmosConfig, node, val, file, includeMode{forceRaw: true})
	return err
}

// processIncludeTagInternal handles both !include and !include.raw tags.
func processIncludeTagInternal(
	atmosConfig *schema.AtmosConfiguration,
	node *yaml.Node,
	val string,
	file string,
	mode includeMode,
) (bool, error) {
	defer perf.Track(atmosConfig, "utils.processIncludeTagInternal")()

	forceRaw := mode.forceRaw

	var includeFile string
	var includeQuery string
	var res any
	var localFile string

	// Parse the include arguments
	parsed, err := parser.ParseInclude(val)
	if err != nil {
		return false, err
	}
	includeFile = parsed.Path
	includeQuery = parsed.Query
	evalFunctions := parsed.Eval

	// Try to find the file locally
	localFile = findLocalFile(includeFile, file, atmosConfig)

	// Process the file
	if localFile != "" {
		if isYAMLInclude(localFile, forceRaw) {
			data, err := os.ReadFile(localFile)
			if err != nil {
				return false, fmt.Errorf("%w: %s, stack manifest: %s, error: %w",
					ErrIncludeYamlFunctionFailedStackManifest, val, file, err)
			}
			if err := spliceYAMLText(atmosConfig, node, string(data), includeSplice{query: includeQuery, eval: evalFunctions, val: val, file: file}); err != nil {
				return false, err
			}
			if evalFunctions && mode.walkIn != nil {
				// Resolve tags nested in the included file relative to that file.
				return true, mode.walkIn(node, localFile)
			}
			return false, nil
		}
		// Process local file
		res, err = processLocalFile(localFile, forceRaw)
		if err != nil {
			return false, err
		}
	} else if shouldFetchRemote(includeFile) {
		if isYAMLInclude(includeFile, forceRaw) {
			raw, err := processRemoteFile(atmosConfig, includeFile, true)
			if err != nil {
				return false, err
			}
			var text string
			switch v := raw.(type) {
			case string:
				text = v
			case []byte:
				text = string(v)
			default:
				return false, fmt.Errorf("%w: %s, stack manifest: %s, error: unexpected remote content type %T",
					ErrIncludeYamlFunctionFailedStackManifest, val, file, raw)
			}
			// Nested paths inside remote content keep the outer manifest as
			// their context; there is no local directory to resolve against.
			return false, spliceYAMLText(atmosConfig, node, text, includeSplice{query: includeQuery, eval: evalFunctions, val: val, file: file})
		}
		// Process as remote if it's a URL or go-getter detects it as remote
		res, err = processRemoteFile(atmosConfig, includeFile, forceRaw)
		if err != nil {
			return false, err
		}
	} else {
		// Local file not found - provide helpful error message.
		// Prefer BasePathAbsolute for the error since it's more informative.
		errBasePath := atmosConfig.BasePathAbsolute
		if errBasePath == "" {
			errBasePath = atmosConfig.BasePath
		}
		return false, fmt.Errorf("%w: could not find local file '%s' (tried relative to manifest '%s' and base path '%s')",
			ErrIncludeYamlFunctionInvalidFile, includeFile, file, errBasePath)
	}

	// Apply YQ expression if provided
	if includeQuery != "" {
		res, err = EvaluateYqExpression(atmosConfig, res, includeQuery)
		if err != nil {
			return false, err
		}
	}

	// Update the YAML node with the result
	return false, updateYamlNode(node, res, val, file)
}

// includeSplice carries the per-include inputs spliceYAMLText needs: the yq
// query, whether `| eval` was given, and the raw tag value and manifest file
// for error messages.
type includeSplice struct {
	query string
	eval  bool
	val   string
	file  string
}

// isYAMLInclude reports whether an !include target is a YAML document that
// should be spliced into the manifest as YAML nodes, keeping any tags it
// contains, rather than decoded to Go values first. !include.raw never is.
func isYAMLInclude(path string, forceRaw bool) bool {
	if forceRaw {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filetype.ExtractFilenameFromPath(path)))
	return ext == YamlFileExtension || ext == YmlFileExtension
}

// spliceYAMLText parses yamlText (after applying query, if any) and replaces
// node with the parsed document. Unlike the decode-then-re-encode path every
// other file type takes, this keeps the included document's YAML tags, so a
// foreign tag such as a CloudFormation short-form intrinsic reaches its
// registered rewriter instead of being decoded to a bare string (before this,
// `template: !include template.yaml` on an aws/cloudformation component
// silently turned `Role: !GetAtt Role.Arn` into the literal string "Role.Arn").
//
// Atmos function tags inside the included file follow the documented
// "included content is data" contract unless the include opted in with
// `| eval`: without it the tag is dropped and its argument stays a string,
// exactly as before; with it the walker's recursion into the spliced nodes
// resolves them like any other manifest value.
func spliceYAMLText(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, yamlText string, opts includeSplice) error {
	defer perf.Track(atmosConfig, "utils.spliceYAMLText")()

	query, val, file := opts.query, opts.val, opts.file
	if query != "" {
		evaluated, err := evaluateYqToYAML(atmosConfig, yamlText, query)
		if err != nil {
			return fmt.Errorf("%w: %s, stack manifest: %s, error: %w",
				ErrIncludeYamlFunctionFailedStackManifest, val, file, err)
		}
		yamlText = evaluated
	}

	trimmed := strings.TrimSpace(yamlText)
	if trimmed == "" {
		*node = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		return nil
	}

	// A yq query can yield a bare scalar whose text the YAML parser would
	// misread (a trailing colon, a leading '#'); keep it as the string it is,
	// exactly as EvaluateYqExpression does.
	if query != "" && isScalarString(trimmed) {
		setStringScalar(node, trimmed)
		return nil
	}

	contentNode, err := unmarshalYamlContent(yamlText, val, file)
	if err != nil {
		return err
	}
	if query != "" && isMisinterpretedScalar(contentNode, trimmed) {
		setStringScalar(node, trimmed)
		return nil
	}

	if !opts.eval {
		literalizeAtmosTags(contentNode)
	}
	*node = *contentNode
	return nil
}

// literalizeAtmosTags drops every Atmos YAML function tag in the subtree,
// keeping the tagged value as plain data (`!env HOME` -> "HOME"). Foreign
// tags are left alone for their rewriters.
func literalizeAtmosTags(node *yaml.Node) {
	if node == nil {
		return
	}
	tag := strings.TrimSpace(node.Tag)
	if strings.HasPrefix(tag, "!") && !strings.HasPrefix(tag, "!!") && fntag.IsValidYAML(tag) {
		node.Tag = ""
		if node.Kind == yaml.ScalarNode {
			node.Tag = "!!str"
		}
	}
	for _, child := range node.Content {
		literalizeAtmosTags(child)
	}
}

// setStringScalar turns node into a plain string scalar holding value.
func setStringScalar(node *yaml.Node, value string) {
	if strings.HasPrefix(value, "#") {
		handleCommentString(node, value)
		return
	}
	*node = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// isRemoteURL checks if the path is a remote URL.
func isRemoteURL(path string) bool {
	remoteProtocols := []string{"http://", "https://", "s3://", "gcs://", "git://", "oci://", "scp://", "sftp://", "github://"}
	for _, protocol := range remoteProtocols {
		if strings.HasPrefix(path, protocol) {
			return true
		}
	}
	return strings.Contains(path, "::")
}

// shouldFetchRemote checks if the path should be processed as a remote resource.
// It first checks for explicit URL protocols, then uses go-getter's detection
// to handle shorthand formats like "github.com/org/repo".
func shouldFetchRemote(path string) bool {
	defer perf.Track(nil, "utils.shouldFetchRemote")()

	// First check for explicit URL protocols
	if isRemoteURL(path) {
		return true
	}

	// Use go-getter to detect if this is a remote source
	// This handles shorthands like "github.com/org/repo"
	detectors := []getter.Detector{
		&getter.GitDetector{},
		&getter.GitHubDetector{},
		&getter.GitLabDetector{},
		&getter.BitBucketDetector{},
		&getter.S3Detector{},
		&getter.GCSDetector{},
		&getter.FileDetector{},
	}

	for _, detector := range detectors {
		src, ok, err := detector.Detect(path, "")
		if err != nil || !ok {
			continue
		}
		// If any non-file detector matched, treat it as remote
		if _, isFile := detector.(*getter.FileDetector); !isFile && src != "" {
			return true
		}
	}

	return false
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
	defer perf.Track(atmosConfig, "utils.findLocalFile")()

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

	// Try relative to the base_path from atmos.yaml.
	// Prefer BasePathAbsolute (resolved during config init) over BasePath (which may be relative).
	basePath := atmosConfig.BasePathAbsolute
	if basePath == "" {
		basePath = atmosConfig.BasePath
	}
	atmosManifestPath := filepath.Join(basePath, includeFile)
	return resolveAbsolutePath(atmosManifestPath)
}

// processLocalFile reads and parses a local file.
func processLocalFile(localFile string, forceRaw bool) (any, error) {
	defer perf.Track(nil, "utils.processLocalFile")()

	if forceRaw {
		// Always return raw content for !include.raw
		return filetype.ParseFileRaw(os.ReadFile, localFile)
	}
	// Use extension-based parsing for regular !include
	return filetype.ParseFileByExtension(os.ReadFile, localFile)
}

// processRemoteFile downloads and parses a remote file.
func processRemoteFile(atmosConfig *schema.AtmosConfiguration, includeFile string, forceRaw bool) (any, error) {
	defer perf.Track(atmosConfig, "utils.processRemoteFile")()

	// Convert GitHub URLs to raw URLs if needed.
	downloadURL := includeFile
	if isGitHubURL(includeFile) {
		rawURL, err := github.ConvertToRawURL(includeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to convert GitHub URL to raw URL: %w", err)
		}
		downloadURL = rawURL
	}

	// Build options, including test HTTP client if set.
	var opts []downloader.GoGetterOption
	if TestHTTPClient != nil {
		opts = append(opts, downloader.WithHTTPClient(TestHTTPClient))
	}

	dl := downloader.NewGoGetterDownloader(atmosConfig, opts...)

	if forceRaw {
		// Always return raw content for !include.raw
		return dl.FetchAndParseRaw(downloadURL)
	}
	// Use extension-based parsing for regular !include
	return dl.FetchAndParseByExtension(downloadURL)
}

// isGitHubURL checks if the URL is a GitHub (or configured GitHub Enterprise Server) URL that
// needs conversion to raw content via github.ConvertToRawURL.
//
// RawURL is parsed and its Host compared via IsHost rather than a literal string-prefix match.
// IsHost normalizes case, a trailing dot, and the port on both sides (dropping only the scheme
// default), so a GITHUB_SERVER_URL with a non-default port such as "https://ghe.example.com:8443"
// is recognized and a URL on a different port is not.
//
// Public github.com is matched in addition to RepoEndpoints (github.IsPublicGitHubHost), not
// instead of it: when GHES is configured (GITHUB_SERVER_URL points at a different host), a
// public "https://github.com/owner/repo/blob/..." include must still be converted to raw
// content -- it is a link to public GitHub, unrelated to the caller's own GHES instance.
func isGitHubURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, "github://") {
		return true
	}
	parsed, err := neturl.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	return github.IsPublicGitHubHost(parsed.Host) || github.RepoEndpoints().IsHost(parsed.Host)
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
	defer perf.Track(nil, "utils.updateYamlNode")()

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
