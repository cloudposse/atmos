package create

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// OutputKind classifies a file a plan writes.
type OutputKind int

const (
	// OutputManifest is the requested stack manifest.
	OutputManifest OutputKind = iota
	// OutputFile is any other file the scaffold renders.
	OutputFile
	// OutputRecord is the Atmos-owned generation record.
	OutputRecord
)

// Output is one file a plan writes.
type Output struct {
	Path string
	Kind OutputKind
}

// Outputs lists every file the plan writes, as project-relative slash paths, in write order.
func (p *Plan) Outputs() []Output {
	defer perf.Track(nil, "create.Plan.Outputs")()

	outputs := make([]Output, 0, len(p.Generation.Files))
	for _, file := range p.Generation.Files {
		kind := OutputFile
		switch {
		case filepath.Clean(file.Path) == filepath.Clean(p.Manifest) || slices.Contains(p.Manifests, filepath.ToSlash(file.Path)):
			kind = OutputManifest
		case isRecordPath(file.Path):
			kind = OutputRecord
		}
		outputs = append(outputs, Output{Path: filepath.ToSlash(file.Path), Kind: kind})
	}
	return outputs
}

func isRecordPath(path string) bool {
	return filepath.Dir(filepath.Clean(path)) == recordDirectory
}

// Preview prints what Apply would do. Stdout carries file previews, each introduced by a comment
// naming its path and separated from the next by `---`, so it stays pipeable. Files may contain
// non-YAML content, so the stream is not guaranteed to be valid YAML.
// The generation record is only mentioned on stderr: it holds every answer and is not worth dumping.
func (p *Plan) Preview(skip func(string) bool) error {
	defer perf.Track(nil, "create.Plan.Preview")()

	written := 0
	for _, file := range p.Generation.Files {
		if isRecordPath(file.Path) {
			ui.Infof("Would record the generation in %s", filepath.ToSlash(file.Path))
			continue
		}
		path := filepath.ToSlash(file.Path)
		ui.Infof("Would create %s", path)
		if written > 0 {
			if err := data.Writeln("---"); err != nil {
				return err
			}
		}
		written++
		if err := data.Writeln("# " + path); err != nil {
			return err
		}
		if err := data.Writeln(strings.TrimSuffix(file.Content, "\n")); err != nil {
			return err
		}
	}
	return previewHooks(p.Hooks, skip)
}

// hookRank orders before.* hooks ahead of after.* hooks; ok is false for a hook no scaffold event reaches.
func hookRank(hook *hooks.Hook) (rank int, ok bool) {
	switch {
	case hook.MatchesEvent(hooks.BeforeScaffoldGenerate):
		return 0, true
	case hook.MatchesEvent(hooks.AfterScaffoldGenerate):
		return 1, true
	default:
		return 0, false
	}
}

func previewHooks(declared map[string]hooks.Hook, skip func(string) bool) error {
	names := make([]string, 0, len(declared))
	for name := range declared {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		first, second := declared[names[i]], declared[names[j]]
		ri, oki := hookRank(&first)
		rj, okj := hookRank(&second)
		if oki != okj {
			return oki
		}
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		hook := declared[name]
		if err := previewHook(name, &hook, skip); err != nil {
			return err
		}
	}
	return nil
}

func previewHook(name string, hook *hooks.Hook, skip func(string) bool) error {
	if _, ok := hookRank(hook); !ok {
		ui.Infof("Scaffold hook %s ignored: not a scaffold event (events: %s)", name, strings.Join(hook.Events, ", "))
		return nil
	}
	if skip != nil && skip(name) {
		ui.Infof("Would skip scaffold hook %s (--skip-hooks)", name)
		return nil
	}
	ui.Infof("Scaffold hook %s would be evaluated at generation time", name)
	content, err := hookYAML(hook)
	if err != nil {
		return err
	}
	// Separate hook declarations with the same markers as file previews.
	if err := data.Writeln("---"); err != nil {
		return err
	}
	if err := data.Writeln("# scaffold hook " + name); err != nil {
		return err
	}
	return data.Writeln(strings.TrimSuffix(content, "\n"))
}

// hookYAML renders a hook with its `when:` shown the way the author wrote it, without the internal
// "!cel" marker the condition normalizer adds.
func hookYAML(hook *hooks.Hook) (string, error) {
	var node yaml.Node
	if err := node.Encode(hook); err != nil {
		return "", fmt.Errorf("encode hook: %w", err)
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "when" {
				stripConditionMarker(node.Content[i+1])
			}
		}
	}
	content, err := yaml.Marshal(&node)
	return string(content), err
}

func stripConditionMarker(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode {
		// A normalized CEL condition marshals as a scalar carrying the tag, or as text with a tag prefix.
		if node.Tag == condition.CELTag {
			node.Tag = ""
			node.Style = 0
		}
		if rest, found := strings.CutPrefix(node.Value, condition.CELTag+" "); found {
			node.Value = strings.TrimSpace(rest)
			node.Style = 0
		}
		return
	}
	for _, child := range node.Content {
		stripConditionMarker(child)
	}
}
