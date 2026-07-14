package atmos

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CommandNode is a decoupled, serializable snapshot of a single Atmos CLI
// *cobra.Command, used by ListCommandsTool and CommandHelpTool.
//
// # Import-cycle / visibility design note
//
// These two tools need read access to the live Atmos CLI command tree,
// which is owned by cmd/internal's CommandRegistry (see
// cmd/internal.ListProviders / CommandProvider.GetCommand). This package
// (pkg/ai/tools/atmos) CANNOT import cmd/internal directly. Go's
// internal-package visibility rule restricts an "internal" package to
// importers whose own import path is rooted at the directory containing
// it, in this case cmd/. This package does not qualify, so the compiler
// rejects the import outright (error: "use of internal package not
// allowed") regardless of whether a real dependency cycle would also
// result. (One would: cmd (root) blank-imports cmd/mcp, whose
// cmd/mcp/server/start.go imports pkg/ai/tools/atmos; pkg/ai/tools/atmos
// importing cmd/internal directly stays cycle-free only because
// cmd/internal itself never imports anything upstream -- but the
// visibility rule blocks the import before cycle analysis is even
// relevant.)
//
// Rather than importing cmd/internal, atmos_list_commands and
// atmos_command_help depend only on this decoupled CommandNode type plus a
// small injection seam, SetCommandTreeProvider. NewCommandNodeFromCobra
// (below) converts a *cobra.Command subtree into CommandNode using only the
// public spf13/cobra API (a normal third-party dependency, not subject to
// the internal-package rule), so ALL tree-walking, search, and formatting
// logic for these tools still lives here in pkg/ai/tools/atmos, per this
// repo's "no feature logic in cmd/" convention (cmd/ and internal/exec
// should contain only inline call sites).
//
// The one remaining piece of wiring -- calling
// cmd/internal.ListProviders() to obtain the live *cobra.Command tree and
// wrapping each top-level command with NewCommandNodeFromCobra, then
// calling SetCommandTreeProvider with the result -- must happen from a
// package rooted at cmd/ (e.g. cmd/mcp/server/start.go, before it calls
// atmosTools.RegisterTools). That call is intentionally NOT added by this
// change: per this task's scope, central tool registration/wiring is
// handled by a separate follow-up task. Until SetCommandTreeProvider is
// called, both tools return errUtils.ErrAICommandTreeNotConfigured instead of
// panicking or silently returning an empty tree.
type CommandNode struct {
	// Name is the command's own name, e.g. "pull" (cmd.Name(), the first
	// word of Use).
	Name string
	// Use is the raw cobra Use string, e.g. "pull [flags]".
	Use string
	// Short is the one-line description.
	Short string
	// Long is the full description.
	Long string
	// Example is cmd.Example as wired at snapshot time. Most commands only
	// have this populated lazily by cmd/root.go's help renderer, so it is
	// frequently empty here -- see command_help.go's markdown fallback.
	Example string
	// Group is the registry group name (e.g. "Configuration Management").
	// Only meaningful on top-level nodes; descendants leave this empty.
	Group string
	// Hidden mirrors cobra's Hidden flag; hidden nodes are filtered out of
	// listings.
	Hidden bool
	// Flags are the flags defined directly on this command (not inherited
	// from parents).
	Flags []flagInfo
	// Subcommands are this command's immediate children.
	Subcommands []*CommandNode
}

// CommandTreeProvider supplies the live Atmos CLI command tree (its
// top-level nodes; each CommandNode's Subcommands field holds its
// descendants) to atmos_list_commands / atmos_command_help.
type CommandTreeProvider func() []*CommandNode

// commandTreeProvider is injected via SetCommandTreeProvider.
var commandTreeProvider CommandTreeProvider

// SetCommandTreeProvider registers the function these tools use to obtain
// the live Atmos CLI command tree. It must be called once during host
// process startup (typically from cmd/mcp/server, wrapping
// cmd/internal.ListProviders() + NewCommandNodeFromCobra) before
// atmos_list_commands / atmos_command_help are executed. Passing nil clears
// the provider; this is primarily useful for tests, which should always
// restore the previous provider (e.g. via t.Cleanup) to avoid cross-test
// pollution, since the provider is process-global.
func SetCommandTreeProvider(provider CommandTreeProvider) {
	commandTreeProvider = provider
}

// NewCommandNodeFromCobra recursively snapshots cmd (and its subcommands)
// into a decoupled CommandNode tree, using only the public spf13/cobra API.
// The group parameter is attached to cmd itself but left empty on
// descendants, since "group" is a top-level command-registry concept.
func NewCommandNodeFromCobra(cmd *cobra.Command, group string) *CommandNode {
	node := &CommandNode{
		Name:    cmd.Name(),
		Use:     cmd.Use,
		Short:   cmd.Short,
		Long:    cmd.Long,
		Example: cmd.Example,
		Group:   group,
		Hidden:  cmd.Hidden,
		Flags:   collectFlagInfo(cmd),
	}

	children := cmd.Commands()
	if len(children) > 0 {
		node.Subcommands = make([]*CommandNode, 0, len(children))
		for _, child := range children {
			node.Subcommands = append(node.Subcommands, NewCommandNodeFromCobra(child, ""))
		}
	}

	return node
}

// flagInfo is a serializable summary of a single cobra flag.
type flagInfo struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// collectFlagInfo returns the visible flags defined directly on cmd
// (cmd.Flags()), sorted by name. Flags inherited from parent/persistent
// scopes are intentionally out of scope -- this reports only the flags
// specific to the command being snapshotted.
func collectFlagInfo(cmd *cobra.Command) []flagInfo {
	var flagsList []flagInfo

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flagsList = append(flagsList, flagInfo{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Description: f.Usage,
			Default:     f.DefValue,
		})
	})

	sort.Slice(flagsList, func(i, j int) bool { return flagsList[i].Name < flagsList[j].Name })

	return flagsList
}

// ListCommandsTool discovers the Atmos CLI command tree so an AI agent can
// find out what commands exist without shelling out to `atmos --help`. See
// the CommandNode doc comment for how it obtains the tree without an
// import cycle.
type ListCommandsTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewListCommandsTool creates a new command discovery tool.
func NewListCommandsTool(atmosConfig *schema.AtmosConfiguration) *ListCommandsTool {
	return &ListCommandsTool{atmosConfig: atmosConfig}
}

// Name returns the tool name.
func (t *ListCommandsTool) Name() string {
	return "atmos_list_commands"
}

// Description returns the tool description.
func (t *ListCommandsTool) Description() string {
	return "List Atmos CLI commands and subcommands. Use this to discover what commands exist " +
		"before running them. Optionally scope to a subtree with 'path' (e.g. \"vendor\") and use " +
		"atmos_command_help with the returned dotted path to get full usage details for a specific command."
}

// Parameters returns the tool parameters.
func (t *ListCommandsTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name: "path",
			Description: "Dotted command path to scope the listing to a subtree, e.g. \"vendor\" or " +
				"\"vendor pull\". Empty (default) lists from the top level.",
			Type:     tools.ParamTypeString,
			Required: false,
		},
		{
			Name: "recursive",
			Description: "If true (default), include all descendant subcommands recursively. If false, " +
				"only list immediate children of 'path' (or top-level commands when 'path' is empty).",
			Type:     tools.ParamTypeBool,
			Required: false,
			Default:  true,
		},
	}
}

// Execute runs the tool.
func (t *ListCommandsTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	path, _ := params["path"].(string)
	path = strings.TrimSpace(path)

	recursive := true
	if v, ok := params["recursive"].(bool); ok {
		recursive = v
	}

	startNodes, startPrefix, err := resolveListingScope(path)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	var entries []commandEntry
	for _, n := range startNodes {
		entries = appendCommandEntries(entries, n, startPrefix, recursive)
	}

	return buildListCommandsResult(entries, path, recursive), nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ListCommandsTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListCommandsTool) IsRestricted() bool {
	return false
}

// commandEntry is a flattened, discoverable representation of a single
// command node, keyed by its full dotted path (e.g. "vendor pull").
type commandEntry struct {
	Path  string
	Use   string
	Short string
	Group string
}

// commandTreeRoots returns the injected command tree's top-level nodes, or
// errUtils.ErrAICommandTreeNotConfigured if SetCommandTreeProvider hasn't been called yet.
func commandTreeRoots() ([]*CommandNode, error) {
	if commandTreeProvider == nil {
		return nil, errUtils.ErrAICommandTreeNotConfigured
	}
	return commandTreeProvider(), nil
}

// resolveListingScope determines the set of nodes to start listing from and
// the dotted-path prefix those nodes should be reported under.
func resolveListingScope(path string) (startNodes []*CommandNode, startPrefix string, err error) {
	roots, err := commandTreeRoots()
	if err != nil {
		return nil, "", err
	}

	if path == "" {
		return sortedNodes(roots), "", nil
	}

	node, err := findNodeByPath(roots, path)
	if err != nil {
		return nil, "", err
	}

	return sortedNodes(node.Subcommands), path, nil
}

// appendCommandEntries adds an entry for node (and, if recursive, every
// descendant) to entries, using dotted paths rooted at parentPath.
func appendCommandEntries(entries []commandEntry, node *CommandNode, parentPath string, recursive bool) []commandEntry {
	path := node.Name
	if parentPath != "" {
		path = parentPath + " " + node.Name
	}

	entries = append(entries, commandEntry{
		Path:  path,
		Use:   node.Use,
		Short: node.Short,
		Group: node.Group,
	})

	if recursive {
		for _, child := range sortedNodes(node.Subcommands) {
			entries = appendCommandEntries(entries, child, path, recursive)
		}
	}

	return entries
}

// sortedNodes returns the visible (non-hidden, non-nil) nodes, sorted by name.
func sortedNodes(nodes []*CommandNode) []*CommandNode {
	visible := make([]*CommandNode, 0, len(nodes))

	for _, n := range nodes {
		if n == nil || n.Hidden {
			continue
		}
		visible = append(visible, n)
	}

	sort.Slice(visible, func(i, j int) bool { return visible[i].Name < visible[j].Name })

	return visible
}

// findNodeByPath resolves a dotted command path (e.g. "vendor pull") to its
// CommandNode by walking roots and then descending through subcommands by
// name. It returns errUtils.ErrAICommandNotFound, wrapped with the offending path, if
// any segment cannot be resolved.
func findNodeByPath(roots []*CommandNode, path string) (*CommandNode, error) {
	tokens := strings.Fields(path)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrAICommandNotFound, path)
	}

	node := findNodeByName(roots, tokens[0])
	if node == nil {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrAICommandNotFound, path)
	}

	for _, token := range tokens[1:] {
		node = findNodeByName(node.Subcommands, token)
		if node == nil {
			return nil, fmt.Errorf("%w: %q", errUtils.ErrAICommandNotFound, path)
		}
	}

	return node, nil
}

// findNodeByName looks up an immediate node by name within nodes.
func findNodeByName(nodes []*CommandNode, name string) *CommandNode {
	for _, n := range nodes {
		if n != nil && n.Name == name {
			return n
		}
	}
	return nil
}

// buildListCommandsResult formats the discovered commands into a tools.Result.
func buildListCommandsResult(entries []commandEntry, path string, recursive bool) *tools.Result {
	var output strings.Builder

	header := "Available Atmos Commands"
	if path != "" {
		header = fmt.Sprintf("Available Atmos Commands under %q", path)
	}
	fmt.Fprintf(&output, "%s (%d):\n\n", header, len(entries))

	data := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		if e.Short != "" {
			fmt.Fprintf(&output, "  %-40s %s\n", e.Path, e.Short)
		} else {
			fmt.Fprintf(&output, "  %s\n", e.Path)
		}

		data = append(data, map[string]interface{}{
			"path":  e.Path,
			"use":   e.Use,
			"short": e.Short,
			"group": e.Group,
		})
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"path":      path,
			"recursive": recursive,
			"commands":  data,
		},
	}
}
