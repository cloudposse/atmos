package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const maxFileInputBytes = 16 * 1024 * 1024

// FileSelector selects literal YAML/JSON documents relative to the check root.
type FileSelector struct {
	Include []string `json:"include" yaml:"include"`
	Exclude []string `json:"exclude,omitempty" yaml:"exclude"`
}

// FileResult identifies the document independently of its unwrapped input contents.
type FileResult struct {
	File     string   `json:"file"`
	Decision Decision `json:"decision"`
	Error    string   `json:"error,omitempty"`
}

func validateFileSelector(kind InputKind, selector *FileSelector) error {
	if kind != File {
		if selector != nil {
			return fmt.Errorf("%w: files requires on: file", errUtils.ErrPolicyInvalid)
		}
		return nil
	}
	if selector == nil || len(selector.Include) == 0 {
		return fmt.Errorf("%w: on: file requires files.include", errUtils.ErrPolicyInvalid)
	}
	for _, patterns := range [][]string{selector.Include, selector.Exclude} {
		for _, pattern := range patterns {
			if !fs.ValidPath(pattern) || strings.Contains(pattern, "\\") || !doublestar.ValidatePattern(pattern) {
				return fmt.Errorf("%w: invalid root-relative file glob %q", errUtils.ErrPolicyInvalid, pattern)
			}
		}
	}
	return nil
}

// CheckFiles evaluates file policies without loading Atmos configuration or executing effects.
// It returns all collected outcomes, including denials and input errors, before failing the check.
func CheckFiles(ctx context.Context, bundle *Bundle, directory string, options ...FileCheckOption) ([]FileResult, error) {
	defer perf.Track(nil, "policy.CheckFiles")()
	if bundle == nil {
		return nil, fmt.Errorf("%w: missing policy bundle", errUtils.ErrPolicyInvalid)
	}
	selected, err := selectFiles(ctx, bundle.RulesFor(File), directory)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	results := make([]FileResult, 0, len(names))
	var failures []error
	evaluation := &fileCheckContext{engine: NewEngine()}
	for _, option := range options {
		option(evaluation)
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return results, errors.Join(append(failures, err)...)
		}
		result, err := checkFile(ctx, evaluation, directory, name, selected[name])
		results = append(results, result)
		if err != nil {
			failures = append(failures, err)
		}
	}
	return results, errors.Join(failures...)
}

func selectFiles(ctx context.Context, rules []Rule, directory string) (map[string][]Rule, error) {
	selected := map[string][]Rule{}
	if len(rules) == 0 {
		return selected, nil
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("%w: file check root: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	defer root.Close()
	for index := range rules {
		rule := &rules[index]
		if err := validateFileSelector(File, rule.Files); err != nil {
			return nil, err
		}
		names, err := matchingFiles(ctx, root.FS(), rule.Files)
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("%w: rule %q matched no YAML/JSON files", errUtils.ErrPolicyInputUnavailable, rule.ID)
		}
		for name := range names {
			selected[name] = append(selected[name], *rule)
		}
	}
	return selected, nil
}

func matchingFiles(ctx context.Context, root fs.FS, selector *FileSelector) (map[string]bool, error) {
	names := map[string]bool{}
	for _, pattern := range selector.Include {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matches, err := doublestar.Glob(root, pattern, doublestar.WithFilesOnly(), doublestar.WithNoFollow(), doublestar.WithFailOnIOErrors())
		if err != nil {
			return nil, fmt.Errorf("%w: glob %q: %w", errUtils.ErrPolicyInputInvalid, pattern, err)
		}
		for _, name := range matches {
			if isDocument(name) && !excludedFile(selector.Exclude, name) {
				names[name] = true
			}
		}
	}
	return names, nil
}

func isDocument(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func excludedFile(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if matched, _ := doublestar.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

func checkFile(ctx context.Context, evaluation *fileCheckContext, directory, name string, rules []Rule) (FileResult, error) {
	result := FileResult{File: name}
	raw, err := readLocalResource(directory, name, maxFileInputBytes)
	if err == nil {
		raw, err = parseFileDocument(name, raw)
	}
	if err != nil {
		result.Decision = Decision{Status: EvaluationError, Findings: []Finding{}}
		result.Error = err.Error()
		return result, fmt.Errorf("%w: file %q: %w", errUtils.ErrPolicyInputInvalid, name, err)
	}
	result.Decision, err = evaluation.engine.Evaluate(ctx, &Request{Kind: File, File: name, Input: raw, CI: evaluation.ci}, rules)
	if err == nil {
		err = EnforceDecision(result.Decision)
	}
	return result, err
}

func parseFileDocument(name string, raw []byte) (json.RawMessage, error) {
	if strings.EqualFold(path.Ext(name), ".json") {
		err := validateDocument(raw)
		return raw, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, fmt.Errorf("%w: parse YAML: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	if err := validateYAMLTags(&node); err != nil {
		return nil, err
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: decode YAML: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: exactly one YAML document is required", errUtils.ErrPolicyInputInvalid)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: YAML must be JSON-compatible: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	return data, nil
}

func validateYAMLTags(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Tag != "!!str" && key.Tag != "!!merge" {
				return fmt.Errorf("%w: YAML object keys must be strings", errUtils.ErrPolicyInputInvalid)
			}
		}
	}
	if node.Tag != "" && !strings.HasPrefix(node.Tag, "!!") && !strings.HasPrefix(node.Tag, "tag:yaml.org,2002:") {
		return fmt.Errorf("%w: custom YAML tags are not evaluated in file checks", errUtils.ErrPolicyInputInvalid)
	}
	for _, child := range node.Content {
		if err := validateYAMLTags(child); err != nil {
			return err
		}
	}
	return nil
}

func validateDocument(raw json.RawMessage) error {
	if !json.Valid(raw) {
		return fmt.Errorf("%w: expected a JSON document", errUtils.ErrPolicyInputInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%w: decode document: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	_, err := normalizeNumbers(value)
	return err
}

// FileCheckOption configures optional host evidence for file policies.
type FileCheckOption func(*fileCheckContext)

type fileCheckContext struct {
	engine *Engine
	ci     *CISnapshot
}

// WithFileEvidence supplies an engine and CI snapshot without coupling policy to CI providers.
func WithFileEvidence(engine *Engine, snapshot *CISnapshot) FileCheckOption {
	defer perf.Track(nil, "policy.WithFileEvidence")()
	return func(evaluation *fileCheckContext) {
		if engine != nil {
			evaluation.engine = engine
		}
		evaluation.ci = snapshot
	}
}
