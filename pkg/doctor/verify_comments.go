package doctor

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	idSeparator  = "\x00"
	idKeySuffix  = "\x01key"
	idValueSufix = "\x01value"
	idDocument   = "\x01document"
)

// commentRecord is one comment of the document with the node that carries it.
type commentRecord struct {
	id   string
	foot bool
	text string
}

// commentGatherer lists a document's comments in the order they appear in the text. Foot comments
// follow the content they close, so a key's foot comes after its value.
type commentGatherer struct {
	records []commentRecord
}

func nodeID(path []string, key bool) string {
	suffix := idValueSufix
	if key {
		suffix = idKeySuffix
	}
	return strings.Join(path, idSeparator) + suffix
}

func (g *commentGatherer) head(n *yaml.Node, id string) {
	for _, text := range []string{n.HeadComment, n.LineComment} {
		if text != "" {
			g.records = append(g.records, commentRecord{id: id, text: text})
		}
	}
}

func (g *commentGatherer) foot(n *yaml.Node, id string) {
	if n.FootComment != "" {
		g.records = append(g.records, commentRecord{id: id, foot: true, text: n.FootComment})
	}
}

func (g *commentGatherer) visit(n *yaml.Node, path []string, id string) {
	g.head(n, id)
	switch n.Kind {
	case yaml.DocumentNode:
		for _, child := range n.Content {
			g.visit(child, path, nodeID(path, false))
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			child := append(slices.Clone(path), key.Value)
			g.head(key, nodeID(child, true))
			g.visit(value, child, nodeID(child, false))
			g.foot(key, nodeID(child, true))
		}
	case yaml.SequenceNode:
		for i, item := range n.Content {
			child := append(slices.Clone(path), strconv.Itoa(i))
			g.visit(item, child, nodeID(child, false))
		}
	}
	g.foot(n, id)
}

func gatherComments(doc *yaml.Node) []commentRecord {
	g := &commentGatherer{}
	g.visit(doc, nil, idDocument)
	return g.records
}

// trailingIDs returns the nodes on the document's last-entry spine. Comment blocks at the end of
// a document are attached to these nodes, and the encoder regroups them: blocks separated by blank
// lines merge, and a block can move between the last key, its parents, and the document.
func trailingIDs(doc *yaml.Node) map[string]bool {
	ids := map[string]bool{idDocument: true}
	n := rootNode(doc)
	var path []string
	for n != nil {
		switch {
		case n.Kind == yaml.MappingNode && len(n.Content) >= 2:
			last := len(n.Content) - 2
			path = append(path, n.Content[last].Value)
			ids[nodeID(path, true)] = true
			ids[nodeID(path, false)] = true
			n = n.Content[last+1]
		case n.Kind == yaml.SequenceNode && len(n.Content) > 0:
			path = append(path, strconv.Itoa(len(n.Content)-1))
			ids[nodeID(path, false)] = true
			n = n.Content[len(n.Content)-1]
		default:
			return ids
		}
	}
	return ids
}

// touchedIDs returns the nodes an edit removes and the nodes that replace them.
func touchedIDs(edits []edit) (before, after map[string]bool) {
	before, after = map[string]bool{}, map[string]bool{}
	mark := func(set map[string]bool, path []string) {
		set[nodeID(path, true)] = true
		set[nodeID(path, false)] = true
	}
	child := func(path []string, keys ...string) []string {
		return append(slices.Clone(path), keys...)
	}
	for i := range edits {
		e := &edits[i]
		switch e.kind {
		case ruleLegacyNaming:
			mark(before, child(e.path, e.old))
			mark(after, child(e.path, e.target))
		case ruleComponentDependencies:
			oldPath := child(e.path, "settings", "depends_on")
			newPath := child(e.path, "dependencies", "components")
			mark(before, oldPath)
			mark(after, newPath)
			// Include every descendant comment, which moves with its list entry.
			walk(nodeAt(rootNode(e.doc.node), oldPath), nil, func(_ *yaml.Node, path []string) {
				mark(before, child(oldPath, path...))
				mark(after, child(newPath, path...))
			})
		case ruleTerraformReconfigure:
			mark(before, child(e.path, e.old))
			mark(after, child(e.path, "init", "reconfigure"))
		case ruleCommandProvides:
			mark(before, child(e.path, e.old))
			mark(after, child(e.path, "provides"))
		default:
			before[nodeID(e.path, false)] = true
			after[nodeID(e.path, false)] = true
		}
	}
	return before, after
}

// commentLines flattens comment blocks into trimmed, non-empty lines.
func commentLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// commentGroups splits a document's comments into those carried by the edited keys, those in the
// document's trailing blocks, and the rest.
type commentGroups struct {
	touched  []string
	trailing []string
	others   []string
}

func groupComments(doc *yaml.Node, touched, trailing map[string]bool) commentGroups {
	var groups commentGroups
	for _, record := range gatherComments(doc) {
		switch {
		case touched[record.id]:
			groups.touched = append(groups.touched, commentLines(record.text)...)
		case record.foot && trailing[record.id]:
			groups.trailing = append(groups.trailing, commentLines(record.text)...)
		default:
			groups.others = append(groups.others, strings.Join(commentLines(record.text), "\n"))
		}
	}
	slices.Sort(groups.touched)
	return groups
}

// verifyComments requires that an edit changes no comment except by moving the comments of the
// replaced keys onto their replacements. Comments on every other node must be identical and in
// the same order. Comment blocks at the end of the document are compared as ordered text, not by
// the node that carries them, because re-encoding regroups them.
func verifyComments(before, after *yaml.Node, edits []edit) error {
	touchedBefore, touchedAfter := touchedIDs(edits)
	trailing := trailingIDs(before)
	for id := range trailingIDs(after) {
		trailing[id] = true
	}
	a := groupComments(before, touchedBefore, trailing)
	b := groupComments(after, touchedAfter, trailing)
	if !reflect.DeepEqual(a.touched, b.touched) {
		return fmt.Errorf("%w: comments on the replaced key were not carried to its replacement", ErrVerification)
	}
	if !reflect.DeepEqual(a.trailing, b.trailing) {
		return fmt.Errorf("%w: comments at the end of the document changed", ErrVerification)
	}
	if !reflect.DeepEqual(a.others, b.others) {
		return fmt.Errorf("%w: comments changed", ErrVerification)
	}
	return nil
}
