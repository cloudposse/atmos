package ui

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	uitree "github.com/cloudposse/atmos/pkg/ui/tree"
)

// spaceChar is the literal single-space string. It's extracted into a constant since it's
// reused across icon placeholders, indentation, and join separators.
const spaceChar = " "

// iconPlaceholder is used as a placeholder when no action icon is needed.
const iconPlaceholder = spaceChar

// twoSpaceIndent is the indent added for nested content (JSON lines, multi-line diffs).
const twoSpaceIndent = "  "

// newlineStr is the literal newline string, reused when splitting/joining multi-line output.
const newlineStr = "\n"

// symbolColumnWidth is the width of the leading "  ●  " action-symbol column that every
// tree and attribute-diff row is indented past.
const symbolColumnWidth = 5

// RenderTree renders the tree as a string with box-drawing characters.
// Uses a two-column layout: action symbol (fixed width) | tree structure.
func (t *DependencyTree) RenderTree() string {
	return t.RenderTreeWithConfig(nil)
}

// RenderTreeWithConfig renders the tree with custom rendering configuration.
func (t *DependencyTree) RenderTreeWithConfig(config *RenderConfig) string {
	defer perf.Track(nil, "terraform.ui.DependencyTree.RenderTreeWithConfig")()

	var b strings.Builder

	// Header style.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Link)).Bold(true)

	// Render stack/component header (cyan, bold) - aligned with tree.
	fmt.Fprintf(&b, "     %s\n", headerStyle.Render(t.Stack+"/"+t.Component))

	// Render resource tree.
	renderChildren(&b, t.Root.Children, nil, config)
	return b.String()
}

// renderChildren renders nodes and their subtrees. The path argument is the position of the nodes'
// parent (nil at the root); every gutter -- the connector, the attribute rows' rail, the
// non-compact spacer -- comes from pkg/ui/tree, so the rows stay connected by
// construction (see that package's Violations for the invariant this relies on).
func renderChildren(b *strings.Builder, nodes []*TreeNode, path uitree.Path, config *RenderConfig) {
	config = resolveRenderConfig(config)

	for i, node := range nodes {
		isLastChild := i == len(nodes)-1
		nodePath := append(append(uitree.Path{}, path...), isLastChild)

		// Colorized action symbol (fixed 2-char width: symbol + space).
		symbol := colorizedActionSymbol(node.Action)

		// Build tree line: "  +  ├── resource_name"
		// Column 1: 2 spaces + symbol + 2 spaces (5 chars total for alignment)
		// Column 2: tree gutter + connector + resource address
		treeLine := config.TreeStyle.Render(uitree.Connector(nodePath)) + node.Address

		fmt.Fprintf(b, "  %s  %s\n", symbol, treeLine)

		// Attribute changes sit between this row and the children's rows; their gutter
		// carries a rail down to the children when there are any.
		if len(node.Changes) > 0 || node.UnchangedAttrCount > 0 {
			gutter := uitree.ContentGutter(nodePath, len(node.Children) > 0)
			if len(node.Changes) > 0 {
				renderAttributeChanges(b, node.Changes, gutter, config)
			}
			if node.UnchangedAttrCount > 0 {
				renderUnchangedAttributesFooter(b, node.UnchangedAttrCount, gutter, config)
			}
		}

		if len(node.Children) > 0 {
			renderChildren(b, node.Children, nodePath, config)
		}

		// Non-compact mode separates sibling blocks with a spacer row, which still has
		// to carry the rails passing through it.
		if !config.Compact && !isLastChild {
			fmt.Fprintf(b, "%s%s\n", strings.Repeat(spaceChar, symbolColumnWidth), config.TreeStyle.Render(uitree.SpacerGutter(nodePath)))
		}
	}
}

// RenderConfig holds configuration for tree rendering, including display options and the
// styles used to render create/update/delete attribute changes.
type RenderConfig struct {
	// ShowAttributeBar shows a thick ┃ bar alongside attributes.
	ShowAttributeBar bool
	// Compact removes blank lines between resources.
	Compact bool
	// MaxLines controls collapsing of large JSON/YAML values (0 = show all).
	MaxLines int
	// Width overrides terminal detection when positive, primarily for embedded rendering.
	Width int
	// AtmosConfig supplies the existing syntax-highlighting and formatting settings.
	AtmosConfig *schema.AtmosConfiguration

	// CreateStyle, UpdateStyle, DeleteStyle, DimStyle, TreeStyle, and BarStyle are the
	// styles used when rendering the tree and attribute changes. Populated with defaults
	// by resolveRenderConfig when not explicitly set.
	CreateStyle lipgloss.Style
	UpdateStyle lipgloss.Style
	DeleteStyle lipgloss.Style
	DimStyle    lipgloss.Style
	TreeStyle   lipgloss.Style
	BarStyle    lipgloss.Style
}

// BuildRenderConfig translates atmos.yaml's components.terraform.ui.{compact,
// show_attribute_bar,max_lines} into a RenderConfig, applying the documented defaults
// (compact=true, show_attribute_bar=false) when left unset.
func BuildRenderConfig(uiConfig schema.TerraformUI) *RenderConfig {
	defer perf.Track(nil, "terraform.ui.BuildRenderConfig")()

	compact := true
	if uiConfig.Compact != nil {
		compact = *uiConfig.Compact
	}
	showAttributeBar := false
	if uiConfig.ShowAttributeBar != nil {
		showAttributeBar = *uiConfig.ShowAttributeBar
	}
	return &RenderConfig{
		Compact:          compact,
		ShowAttributeBar: showAttributeBar,
		MaxLines:         uiConfig.MaxLines,
	}
}

// resolveRenderConfig returns a fully-populated RenderConfig, preserving any caller-provided
// display options (Compact, ShowAttributeBar, MaxLines) while filling in default styles.
func resolveRenderConfig(config *RenderConfig) *RenderConfig {
	resolved := &RenderConfig{
		CreateStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Success)),
		UpdateStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Warning)),
		DeleteStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Error)),
		DimStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)),
		TreeStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)),
		BarStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)),
	}
	if config != nil {
		resolved.ShowAttributeBar = config.ShowAttributeBar
		resolved.Compact = config.Compact
		resolved.MaxLines = config.MaxLines
		resolved.Width = config.Width
		resolved.AtmosConfig = config.AtmosConfig

		overrideStyle(&resolved.CreateStyle, &config.CreateStyle)
		overrideStyle(&resolved.UpdateStyle, &config.UpdateStyle)
		overrideStyle(&resolved.DeleteStyle, &config.DeleteStyle)
		overrideStyle(&resolved.DimStyle, &config.DimStyle)
		overrideStyle(&resolved.TreeStyle, &config.TreeStyle)
		overrideStyle(&resolved.BarStyle, &config.BarStyle)
	}
	if resolved.Width <= 0 {
		resolved.Width = templates.GetTerminalWidth()
	}
	return resolved
}

// overrideStyle replaces *dst with *caller in place when caller is non-zero.
//
// A lipgloss.Style isn't comparable with == (it embeds a func field).
// Its String() method renders the style's own unset text value, so it always returns "" no matter which properties are set (Foreground, Bold, and so on), which makes it useless for a zero check.
// Comparing the struct via reflect.DeepEqual against the Go zero value works instead.
func overrideStyle(dst, caller *lipgloss.Style) {
	if !reflect.DeepEqual(*caller, lipgloss.Style{}) {
		*dst = *caller
	}
}

// attrRenderContext bundles the layout (indent/bar) and style configuration shared across
// all attribute-change renderers for a single tree node.
type attrRenderContext struct {
	Indent string
	Bar    string
	Config *RenderConfig
}

// attrStyleInfo bundles the per-change key style and "forces replacement" annotation,
// computed once and shared by every rendering branch for that change.
type attrStyleInfo struct {
	KeyStyle   lipgloss.Style
	Annotation string
}

// attributeWidths holds the column widths used to align rendered attribute rows.
type attributeWidths struct {
	Key    int
	OldVal int
}

// attributeKeyStyle returns the style used for an attribute key, based on the change type:
// green for a new attribute, red for a deleted attribute, yellow for an update (including
// unknown/computed values).
func attributeKeyStyle(change *AttributeChange, config *RenderConfig) lipgloss.Style {
	switch {
	case change.Before == nil && change.After != nil:
		return config.CreateStyle
	case change.Before != nil && change.After == nil && !change.Unknown:
		return config.DeleteStyle
	default:
		return config.UpdateStyle
	}
}

// forcesReplacementAnnotation renders the "# forces replacement" annotation for a change,
// or an empty string if the change doesn't force replacement.
func forcesReplacementAnnotation(change *AttributeChange) string {
	if !change.ForcesReplacement {
		return ""
	}
	replaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Warning))
	return spaceChar + replaceStyle.Render("# forces replacement")
}

// renderUnchangedAttributesFooter renders a dim "# (N unchanged attributes hidden)" row,
// mirroring Terraform's own plan-output convention, so a diff-only attribute list doesn't
// read as if it were the resource's entire content. Indentation matches
// renderAttributeChanges exactly (same gutter, same base indent) so the row lines up with
// the attribute rows above it.
func renderUnchangedAttributesFooter(b *strings.Builder, unchangedCount int, gutter string, config *RenderConfig) {
	config = resolveRenderConfig(config)

	baseIndent := strings.Repeat(spaceChar, symbolColumnWidth) + config.TreeStyle.Render(gutter)

	noun := "attributes"
	if unchangedCount == 1 {
		noun = "attribute"
	}
	comment := fmt.Sprintf("# (%d unchanged %s hidden)", unchangedCount, noun)
	writeWrappedAttributeLine(b, config.DimStyle.Render(comment), baseIndent, baseIndent, config.Width)
}

// renderAttributeChanges renders attribute-level changes, aligned under the resource line.
//
// The gutter argument is the full tree gutter the rows sit behind: the node's ancestor rails plus one
// more level covering the node's own connector column. The caller builds it (see
// renderChildren) so a node with children keeps a rail there and its child connectors
// below the diff block stay connected. The gutter's box-drawing characters are rendered
// verbatim (styled), never blanked to spaces: blanking them is what used to break the
// vertical rails for the height of every attribute block.
func renderAttributeChanges(b *strings.Builder, changes []*AttributeChange, gutter string, config *RenderConfig) {
	config = resolveRenderConfig(config)

	baseIndent := strings.Repeat(spaceChar, symbolColumnWidth) + config.TreeStyle.Render(gutter)

	// Build attribute bar if enabled.
	var attrBar string
	if config.ShowAttributeBar {
		attrBar = config.BarStyle.Render("┃") + spaceChar
	}

	formatted, maxKeyWidth, maxOldValWidth := precomputeAttributeFormatting(changes, config)
	ctx := attrRenderContext{Indent: baseIndent, Bar: attrBar, Config: config}
	widths := attributeWidths{Key: maxKeyWidth, OldVal: maxOldValWidth}

	for _, fc := range formatted {
		renderOneAttributeChange(b, &fc, ctx, widths)
	}
}

// linesMatch checks if both indices are valid and lines match.
func linesMatch(before, after []string, i, j int) bool {
	return i < len(before) && j < len(after) && before[i] == after[j]
}

// diffCursor tracks the current read position in the before/after line slices while
// collecting a run of changed lines.
type diffCursor struct {
	I, J int
}

// collectChanges collects deleted and added lines until the next matching line.
func collectChanges(before, after []string, start diffCursor) (deleted, added []string, next diffCursor) {
	i, j := start.I, start.J
	for i < len(before) || j < len(after) {
		if linesMatch(before, after, i, j) {
			break
		}
		if i < len(before) {
			deleted = append(deleted, before[i])
			i++
		}
		if j < len(after) {
			added = append(added, after[j])
			j++
		}
	}
	return deleted, added, diffCursor{I: i, J: j}
}

// colorizedActionSymbol maps a Terraform resource action to an indicator in its semantic theme color.
func colorizedActionSymbol(action string) string {
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Success))
	updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Warning))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Error))
	readStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Link))
	replaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Warning)) // Orange for replace (delete+create).

	// Use colored dots (●) for all actions with different colors:
	// - Green: create
	// - Yellow: update/change in place
	// - Red: delete
	// - Orange: replace/recreate
	// - Cyan: read/refresh
	switch action {
	case "create":
		return createStyle.Render(theme.IconActive)
	case "update":
		return updateStyle.Render(theme.IconActive)
	case "delete":
		return deleteStyle.Render(theme.IconActive)
	case "replace":
		return replaceStyle.Render(theme.IconActive)
	case "read":
		return readStyle.Render(theme.IconActive)
	case "no-op":
		return iconPlaceholder
	default:
		return iconPlaceholder
	}
}

// GetChangeSummary returns a summary of changes from the tree.
func (t *DependencyTree) GetChangeSummary() (add, change, remove int) {
	defer perf.Track(nil, "terraform.ui.DependencyTree.GetChangeSummary")()

	countActions(t.Root, &add, &change, &remove)
	return add, change, remove
}

// HasOutputChanges reports whether the plan contains any output-only change (create, update, or
// delete of an output value), independent of GetChangeSummary's resource-only counts. A plan
// whose only diff is an output value has add == change == remove == 0 but must still not be
// treated as "no changes" (see issue #3114).
func (t *DependencyTree) HasOutputChanges() bool {
	return t.outputChanges > 0
}

// OutputChangeCount returns the number of output-only changes in the plan.
func (t *DependencyTree) OutputChangeCount() int {
	return t.outputChanges
}

func countActions(node *TreeNode, add, change, remove *int) {
	if node == nil {
		return
	}

	switch node.Action {
	case "create":
		*add++
	case "update":
		*change++
	case "delete":
		*remove++
	case "replace":
		// Replace counts as both add and remove since the resource is destroyed and recreated.
		*add++
		*remove++
	}

	for _, child := range node.Children {
		countActions(child, add, change, remove)
	}
}

// noChangesBadge renders the "NO CHANGES" badge shown when a plan has no changes.
func noChangesBadge() string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)).
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextPrimary)).
		Bold(true).
		Padding(0, 1).
		Render("NO CHANGES")
}

// changeBadge renders a single "<count> <LABEL>" badge with a background color and a
// contrasting, accessible text color.
func changeBadge(bgColor, text string) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color(getContrastTextColor(bgColor))).
		Bold(true).
		Padding(0, 1).
		Render(text)
}

// buildChangeBadges renders one badge per non-zero change count.
func buildChangeBadges(add, change, remove int) []string {
	var badges []string
	if add > 0 {
		badges = append(badges, changeBadge(theme.GetCurrentColorScheme().Success, fmt.Sprintf("%d ADD", add)))
	}
	if change > 0 {
		badges = append(badges, changeBadge(theme.GetCurrentColorScheme().Warning, fmt.Sprintf("%d CHANGE", change)))
	}
	if remove > 0 {
		badges = append(badges, changeBadge(theme.GetCurrentColorScheme().Error, fmt.Sprintf("%d DELETE", remove)))
	}
	return badges
}

// outputsChangedBadge renders a badge indicating an output value changed. Reuses the "change"
// (yellow) visual language already used for in-place resource updates, since an output-only
// diff is the same kind of change applied to an output instead of a resource.
func outputsChangedBadge() string {
	return changeBadge(theme.GetCurrentColorScheme().Warning, "OUTPUTS CHANGED")
}

// RenderChangeSummaryBadges renders a badge-style change summary.
// Shows "NO CHANGES" badge only when there are no resource changes and no output changes.
// Format: "  1 ADD 2 CHANGE 1 DELETE" with colored badges (green/yellow/red backgrounds).
// The hasOutputChanges parameter reports a plan/apply whose only diff is an output value (see
// DependencyTree.HasOutputChanges) - never reported as "NO CHANGES", even when add, change, and
// remove are all zero.
func RenderChangeSummaryBadges(add, change, remove int, hasOutputChanges bool) string {
	defer perf.Track(nil, "terraform.ui.RenderChangeSummaryBadges")()

	var badges []string
	switch {
	case add > 0 || change > 0 || remove > 0:
		badges = buildChangeBadges(add, change, remove)
		if hasOutputChanges {
			badges = append(badges, outputsChangedBadge())
		}
	case hasOutputChanges:
		badges = []string{outputsChangedBadge()}
	default:
		badges = []string{noChangesBadge()}
	}

	// Join badges with a space, add blank line above and below, and indent 2 spaces.
	return "\n  " + strings.Join(badges, spaceChar) + "\n\n"
}
