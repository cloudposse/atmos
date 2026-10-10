package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	goyaml "go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// rejectStarlarkTags fails when atmos.yaml, a profile, or an imported configuration file uses the
// `!starlark` tag. Only stack manifests evaluate it, because its context is a component's
// configuration. Left alone, the tag would reach the generic tag handler, which reports an
// unsupported tag without saying which file holds it.
func rejectStarlarkTags(root *goyaml.Node, sourceFile string) error {
	path, found := findStarlarkTag(root, nil)
	if !found {
		return nil
	}
	location := strings.Join(slices.DeleteFunc(path, func(label string) bool { return label == "" }), ".")
	if location == "" {
		location = "(document root)"
	}
	return fmt.Errorf("%w: %s at %s; evaluate the value in a stack manifest, or compute it with !exec or !env",
		errUtils.ErrStarlarkUnsupportedInConfig, sourceFile, location)
}

// findStarlarkTag returns the dotted key path of the first node tagged !starlark.
func findStarlarkTag(node *goyaml.Node, path []string) ([]string, bool) {
	if node == nil {
		return nil, false
	}
	if node.Tag == u.AtmosYamlFuncStarlark {
		return path, true
	}
	for _, child := range labeledChildren(node) {
		if found, ok := findStarlarkTag(child.node, append(slices.Clone(path), child.label)); ok {
			return found, true
		}
	}
	return nil, false
}

// labeledChild is a child node with the key or index that reaches it. A document's single child
// has an empty label, which the path join drops.
type labeledChild struct {
	label string
	node  *goyaml.Node
}

// labeledChildren lists the children of a document, mapping, or sequence node in order.
func labeledChildren(node *goyaml.Node) []labeledChild {
	var children []labeledChild
	switch node.Kind {
	case goyaml.DocumentNode:
		for _, child := range node.Content {
			children = append(children, labeledChild{node: child})
		}
	case goyaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			children = append(children, labeledChild{label: node.Content[i].Value, node: node.Content[i+1]})
		}
	case goyaml.SequenceNode:
		for i, child := range node.Content {
			children = append(children, labeledChild{label: strconv.Itoa(i), node: child})
		}
	}
	return children
}
