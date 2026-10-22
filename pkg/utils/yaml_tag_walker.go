package utils

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	atmosGit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
)

// This file holds the shared YAML custom-tag walker both the stack-manifest
// loader (processCustomTags, below) and the scaffold-manifest loader
// (pkg/manifest, via ScaffoldTagPolicy) dispatch through. Each caller gets
// its own TagWalkPolicy rather than sharing one: a stack manifest defers
// most tags to a later phase that has real stack/component/backend context
// (internal/exec's processCustomTagsWithContext); a scaffold manifest has no
// such later phase, so its policy resolves a fixed, context-free tag set
// immediately and rejects everything else outright instead of silently
// deferring it to a phase that will never run.

// TagContext carries the per-walk inputs a TagHandler needs.
type TagContext struct {
	AtmosConfig *schema.AtmosConfiguration
	File        string
	// origin memoizes File's origin for the walk: resolving it stats every path component and
	// takes the guard lock, so it must not run once per scalar node.
	origin      *tagOrigin
	parent      *yaml.Node
	dataSection bool
	// Walk recurses into node's own children under the SAME policy this
	// handler was invoked with -- used by handlers (e.g. !append) that must
	// resolve nested tags inside content they rewrite before finishing.
	Walk func(node *yaml.Node) error
	// WalkIn is Walk with a different file context: used by !include to
	// resolve tags nested inside an included file relative to that file.
	WalkIn func(node *yaml.Node, file string) error
	// WalkInRemote is WalkIn for remote content. File stays the outer manifest, which nested
	// relative paths resolve against, while source names the remote origin that function
	// admission and provenance attribute the content to.
	WalkInRemote func(node *yaml.Node, file, source string) error
	// source, when set, is the origin of the walked content, overriding File for attribution.
	source string
}

// TagHandler resolves a single recognized YAML tag on node in place
// (mutating node.Value/node.Tag/node.Kind/node.Content as needed), given val
// (the tag's trimmed scalar value). The returned skipChildren reports
// whether the walker must NOT recurse into node.Content afterward -- true
// for tags whose children must not be independently treated as further tags
// (e.g. !literal, whose entire point is to bypass further processing;
// !append, whose handler already recursed into its own rewritten content
// itself).
type TagHandler func(ctx TagContext, node *yaml.Node, val string) (skipChildren bool, err error)

// TagWalkPolicy controls walkYAMLTags' behavior for a tag with no entry in
// Handlers.
type TagWalkPolicy struct {
	// Prepare optionally records host metadata before children are processed.
	// Its returned callback runs after successful processing of the children.
	Prepare func(TagContext, *yaml.Node) func()

	// Handlers maps a tag name (e.g. "!include") to its resolver. Every key
	// must be a tag fntag.IsValidYAML accepts.
	Handlers map[string]TagHandler
	// Defer, true: rewrite an unhandled-but-valid tag into a "<tag> <value>"
	// string for a later evaluation phase (today's stack-manifest
	// behavior -- see getValueWithTag). false: an unhandled-but-valid tag is
	// a hard error naming the tag and listing Handlers' own keys as the
	// supported set.
	Defer bool
}

// WalkYAMLTags recurses through node's tree, dispatching every recognized
// YAML tag to policy's Handlers (or policy's Defer/reject fallback for a
// valid tag with no handler). An invalid tag (one fntag.IsValidYAML
// rejects) is always a hard error, regardless of policy.
func WalkYAMLTags(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string, policy TagWalkPolicy) error {
	return walkYAMLTags(TagContext{AtmosConfig: atmosConfig, File: file, origin: &tagOrigin{}}, node, policy)
}

// tagOrigin resolves a walked file's origin at most once.
type tagOrigin struct {
	once   sync.Once
	origin guard.Origin
}

// attribution is the file or URL the walked content comes from: the remote include target while
// walking remote content, otherwise the file being walked.
func (c *TagContext) attribution() string {
	if c.source != "" {
		return c.source
	}
	return c.File
}

// fileOrigin returns the origin of the content being walked, resolving it once per walk.
func (c *TagContext) fileOrigin() guard.Origin {
	if c.origin == nil {
		return guard.FileOrigin(c.attribution())
	}
	c.origin.once.Do(func() { c.origin.origin = guard.FileOrigin(c.attribution()) })
	return c.origin.origin
}

func walkYAMLTags(ctx TagContext, node *yaml.Node, policy TagWalkPolicy) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return walkYAMLTags(ctx, node.Content[0], policy)
	}

	ctx.Walk = func(n *yaml.Node) error {
		return walkYAMLTags(ctx, n, policy)
	}
	ctx.WalkIn = func(n *yaml.Node, inFile string) error {
		included := ctx
		included.File = inFile
		included.source = ""
		included.origin = &tagOrigin{}
		return walkYAMLTags(included, n, policy)
	}
	ctx.WalkInRemote = func(n *yaml.Node, outerFile, source string) error {
		included := ctx
		included.File = outerFile
		included.source = source
		included.origin = &tagOrigin{}
		return walkYAMLTags(included, n, policy)
	}

	var after func()
	if policy.Prepare != nil {
		after = policy.Prepare(ctx, node)
	}
	for i, n := range node.Content {
		child := childTagContext(ctx, node, i, policy)
		skipChildren, err := dispatchTag(child, n, policy, ctx.File)
		if err != nil {
			return err
		}

		if !skipChildren && len(n.Content) > 0 {
			if err := child.Walk(n); err != nil {
				return err
			}
		}
	}
	if after != nil {
		after()
	}
	return nil
}

// childTagContext preserves location metadata when a handler walks rewritten content.
func childTagContext(ctx TagContext, parent *yaml.Node, index int, policy TagWalkPolicy) TagContext {
	child := ctx
	child.parent = parent
	child.dataSection = ctx.dataSection || isScriptSourceDataValue(parent, index)
	child.Walk = func(value *yaml.Node) error { return walkYAMLTags(child, value, policy) }
	child.WalkIn = func(value *yaml.Node, file string) error {
		included := child
		included.File = file
		included.source = ""
		included.origin = &tagOrigin{}
		return walkYAMLTags(included, value, policy)
	}
	child.WalkInRemote = func(value *yaml.Node, outerFile, source string) error {
		included := child
		included.File = outerFile
		included.source = source
		included.origin = &tagOrigin{}
		return walkYAMLTags(included, value, policy)
	}
	return child
}

// dispatchTag resolves a single node's own tag (not its children) per
// policy, returning whether the walker must skip recursing into its
// children afterward (see TagHandler's doc comment for skipChildren).
func dispatchTag(ctx TagContext, n *yaml.Node, policy TagWalkPolicy, file string) (bool, error) {
	if n.Kind == yaml.ScalarNode {
		guard.RememberValue(n.Value, ctx.fileOrigin())
	}
	tag := strings.TrimSpace(n.Tag)
	if tag == "" || strings.HasPrefix(tag, "!!") {
		// No tag, or a standard YAML tag (!!str, !!int, ...) -- nothing to
		// dispatch.
		return false, nil
	}

	// A registered foreign tag (see yaml_tag_foreign.go) is rewritten in place
	// into its native long form; the walker then recurses into the rewritten
	// children so nested tags inside the value are still resolved.
	if rewrite, ok := lookupForeignTagRewriter(tag); ok {
		if err := rewrite(n); err != nil {
			return false, fmt.Errorf("%w: '%s' found in file '%s': %w", errUtils.ErrUnsupportedYamlTag, tag, file, err)
		}
		return false, nil
	}

	if !fntag.IsValidYAML(tag) {
		// A foreign tag a subsystem recognizes but does not emulate (e.g. a
		// Rain directive) gets that subsystem's own hint instead of the
		// generic supported-tags list.
		if hints := lookupForeignTagHint(tag); len(hints) > 0 {
			builder := errUtils.Build(errUtils.ErrUnsupportedYamlTag).
				WithExplanationf("'%s' found in file '%s'", tag, file)
			for _, hint := range hints {
				builder = builder.WithHint(hint)
			}
			return false, builder.Err()
		}
		// Exact message shape preserved verbatim from before this walker was
		// extracted -- existing tests assert on it via err.Error().
		supportedTags := strings.Join(fntag.AllYAML(), ", ")
		return false, fmt.Errorf("%w: '%s' found in file '%s'. Supported tags are: %s",
			errUtils.ErrUnsupportedYamlTag, tag, file, supportedTags)
	}

	val := strings.TrimSpace(n.Value)
	if handler, ok := policy.Handlers[tag]; ok {
		if err := guard.FunctionAt(context.Background(), tag, ctx.attribution()); err != nil {
			return false, err
		}
		return handler(ctx, n, val)
	}
	if policy.Defer {
		strict, err := ExplicitYAMLTags(ctx.AtmosConfig)
		if err != nil {
			return false, err
		}
		if strict || tag == AtmosYamlFuncCEL {
			n.Value = (yamlsource.Source{Tag: tag, Argument: strings.TrimSpace(n.Value), File: file, Line: n.Line, Column: n.Column}).Encode()
		} else {
			n.Value = getValueWithTag(n)
		}
		guard.RememberValue(n.Value, ctx.fileOrigin())
		n.Tag = ""
		return false, nil
	}
	return false, errUtils.Build(errUtils.ErrUnsupportedYamlTag).
		WithExplanationf("'%s' found in file '%s'", tag, file).
		WithHintf("Supported tags are: %s", strings.Join(sortedHandlerTags(policy.Handlers), ", ")).
		Err()
}

// sortedHandlerTags returns handlers' keys sorted, for a deterministic
// "supported tags" error message.
func sortedHandlerTags(handlers map[string]TagHandler) []string {
	tags := make([]string, 0, len(handlers))
	for tag := range handlers {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// handleLiteralTag keeps literal values as written. Stack loading preserves scalar
// identity until evaluation finishes; generic/config/workflow loading returns plain
// strings and retains its existing step metadata contract. Collections skip children.
//
//nolint:gocritic // TagHandler requires its context by value, including this literal handler.
func handleLiteralTag(ctx TagContext, node *yaml.Node, _ string) (bool, error) {
	if node.Kind == yaml.ScalarNode {
		if ctx.AtmosConfig != nil && ctx.AtmosConfig.PreserveStackLiterals {
			node.Value = literalsource.Encode(node.Value)
		}
		node.Tag = "!!str"
		return true, nil
	}
	node.Tag = ""
	return true, nil
}

// Keep scalar literal identity until component template and function evaluation finish.
//
//nolint:gocritic // TagHandler requires TagContext by value.
func handleStackLiteralTag(ctx TagContext, node *yaml.Node, val string) (bool, error) {
	if node.Kind == yaml.ScalarNode && ctx.AtmosConfig != nil && ctx.AtmosConfig.Templates.Settings.Enabled {
		node.Value = (yamlsource.Source{Tag: AtmosYamlFuncLiteral, Argument: node.Value, File: ctx.File, Line: node.Line, Column: node.Column}).Encode()
		node.Tag = "!!str"
		return true, nil
	}
	return handleLiteralTag(ctx, node, val)
}

// handleAppendTag implements !append: rewrite the tagged sequence node in
// place into a single-entry mapping wrapping it, after resolving any nested
// tags inside the sequence's own items first (via ctx.Walk) -- mirrors the
// historical rewriteAppendNode, now routed through the shared walker so a
// nested tag inside an appended list resolves under the same policy as
// everything else. After the node is decoded, the merge phase (pkg/merge)
// detects the wrapper via ExtractAppendListValue and appends the list
// instead of replacing it.
func handleAppendTag(ctx TagContext, node *yaml.Node, _ string) (bool, error) {
	if node.Kind != yaml.SequenceNode {
		node.Tag = ""
		return true, nil
	}

	inner := *node
	inner.Tag = ""
	if err := ctx.Walk(&inner); err != nil {
		return false, err
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: AppendTagMetadataKey}
	node.Kind = yaml.MappingNode
	node.Tag = ""
	node.Value = ""
	node.Style = 0
	node.Content = []*yaml.Node{keyNode, &inner}
	return true, nil
}

// handleIncludeTag and handleIncludeRawTag implement !include/!include.raw
// by delegating straight to the existing leaf functions -- shared verbatim
// between the stack-manifest and scaffold-manifest policies.
func handleIncludeTag(ctx TagContext, node *yaml.Node, val string) (bool, error) {
	return ProcessIncludeTagWalkedFrom(ctx.AtmosConfig, node, val, ctx.File, IncludeWalk{Local: ctx.WalkIn, Remote: ctx.WalkInRemote, Source: ctx.source})
}

func handleIncludeRawTag(ctx TagContext, node *yaml.Node, val string) (bool, error) {
	return false, ProcessIncludeRawTag(ctx.AtmosConfig, node, val, ctx.File)
}

// simpleTagHandler adapts a leaf function shaped like
// pkg/config/process_yaml.go's handle* functions -- given the full
// "<tag> <value>" string (see getValueWithTag), it returns the resolved
// value (of any type) or an error -- into a TagHandler that writes the
// result back into node via updateYamlNode (the same node-encoding helper
// !include already uses, so a non-scalar result like !exec's JSON-decoded
// output is represented correctly, not just coerced into node.Value).
func simpleTagHandler(resolve func(fullTagValue string) (any, error)) TagHandler {
	return func(ctx TagContext, node *yaml.Node, val string) (bool, error) {
		result, err := resolve(getValueWithTag(node))
		if err != nil {
			return false, err
		}
		return false, updateYamlNode(node, result, val, ctx.File)
	}
}

// ScaffoldTagPolicy returns the TagWalkPolicy used to resolve scaffold.yaml
// manifests. Unlike the stack-manifest policy, Defer is false: scaffold.yaml
// has no later evaluation phase, so a tag needing real stack/component/
// backend context (!terraform.state, !store, !secret, etc.) must be
// rejected outright here rather than silently deferred to a phase that will
// never run and leaving an inert "<tag> <value>" string in the final config.
//
// The supported set mirrors fntag.ScaffoldYAML() (kept in sync by
// TestScaffoldTagPolicy_HandlersMatchScaffoldYAML) -- the scaffold-manifest
// analogue of fntag.AtmosConfigYAML(), which atmos.yaml's own loader
// (pkg/config/process_yaml.go) uses for the identical "no stack context
// available yet" problem. !unset is excluded (scaffold.yaml has no
// stack-manifest-style inheritance chain for it to override) and !literal is
// included (bypassing scaffold's own Go-template evaluation of a field
// value, a real and stack-context-free need) in its place. !exec is
// excluded too, unlike AtmosConfigYAML -- see fntag.ScaffoldYAML's own doc
// comment: scaffold.yaml is resolved for every configured template just to
// populate `atmos scaffold list`/the interactive picker, not only the one
// actually selected, so allowing shell execution here would let any
// configured template run code merely by being listed.
//
// The onInclude callback, when non-nil, is invoked with each
// !include/!include.raw tag's raw path argument as encountered, before
// resolution -- see pkg/manifest.WithIncludeResolution's doc comment for why
// a caller wants this.
func ScaffoldTagPolicy(onInclude func(path string)) TagWalkPolicy {
	defer perf.Track(nil, "utils.ScaffoldTagPolicy")()

	includeHandler := handleIncludeTag
	includeRawHandler := handleIncludeRawTag
	if onInclude != nil {
		includeHandler = func(ctx TagContext, node *yaml.Node, val string) (bool, error) {
			onInclude(val)
			return handleIncludeTag(ctx, node, val)
		}
		includeRawHandler = func(ctx TagContext, node *yaml.Node, val string) (bool, error) {
			onInclude(val)
			return handleIncludeRawTag(ctx, node, val)
		}
	}

	return TagWalkPolicy{
		Handlers: map[string]TagHandler{
			AtmosYamlFuncInclude:       includeHandler,
			AtmosYamlFuncIncludeRaw:    includeRawHandler,
			AtmosYamlFuncLiteral:       handleLiteralTag,
			AtmosYamlFuncEnv:           simpleTagHandler(func(s string) (any, error) { return ProcessTagEnv(s, nil) }),
			AtmosYamlFuncRandom:        simpleTagHandler(func(s string) (any, error) { return ProcessTagRandom(s) }),
			AtmosYamlFuncCwd:           simpleTagHandler(func(s string) (any, error) { return ProcessTagCwd(s) }),
			AtmosYamlFuncGitRoot:       simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagRoot(s) }),
			AtmosYamlFuncGitRootAlias:  simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagRoot(s) }),
			AtmosYamlFuncGitSha:        simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagSHA(s) }),
			AtmosYamlFuncGitRef:        simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagSHA(s) }),
			AtmosYamlFuncGitBranch:     simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagBranch(s) }),
			AtmosYamlFuncGitRepository: simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagRepository(s) }),
			AtmosYamlFuncGitOwner:      simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagOwner(s) }),
			AtmosYamlFuncGitName:       simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagName(s) }),
			AtmosYamlFuncGitHost:       simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagHost(s) }),
			AtmosYamlFuncGitUrl:        simpleTagHandler(func(s string) (any, error) { return atmosGit.ProcessTagURL(s) }),
		},
		Defer: false,
	}
}

// stackManifestTagPolicy and its sync.Once guard back
// getStackManifestTagPolicy, the policy processCustomTags (yaml_utils.go)
// walks stack manifests with: resolve !literal/!append/!include/!include.raw
// immediately, defer every other valid tag to the later evaluation phase
// (internal/exec's processCustomTagsWithContext).
//
// Built lazily behind sync.Once rather than as a plain package-level var
// literal: a var initializer referencing handleIncludeTag transitively
// reaches back into this same package's UnmarshalYAML/processCustomTags
// (via ProcessIncludeTag -> ... -> EvaluateYqExpression -> UnmarshalYAML),
// which the Go compiler flags as a package-initialization cycle even though
// nothing is actually invoked at init time. Deferring construction to first
// real use breaks that static cycle while still computing the handler map
// only once -- processCustomTags runs on the order of thousands of times in
// a large describe-affected run (see its own doc comment), so reallocating
// it per call would matter.
var (
	stackManifestTagPolicyOnce sync.Once
	stackManifestTagPolicyVal  TagWalkPolicy
)

func getStackManifestTagPolicy() TagWalkPolicy {
	stackManifestTagPolicyOnce.Do(func() {
		stackManifestTagPolicyVal = TagWalkPolicy{
			Prepare: prepareScriptSource,
			Handlers: map[string]TagHandler{
				AtmosYamlFuncStarlark:   handleStarlarkTag,
				AtmosYamlFuncCILabels:   handleCILabelsTag,
				AtmosYamlFuncLiteral:    handleStackLiteralTag,
				AtmosYamlFuncAppend:     handleAppendTag,
				AtmosYamlFuncInclude:    handleIncludeTag,
				AtmosYamlFuncIncludeRaw: handleIncludeRawTag,
			},
			Defer: true,
		}
	})
	return stackManifestTagPolicyVal
}

// handleCILabelsTag adapts the function parser to the stack loader's tag handler.
//
//nolint:gocritic // TagHandler requires a value context so handlers cannot mutate the walk's context.
func handleCILabelsTag(ctx TagContext, node *yaml.Node, _ string) (bool, error) {
	expression, err := parser.EncodeCILabels(node)
	if err != nil {
		return false, err
	}
	node.Kind = yaml.ScalarNode
	node.Tag = "!!str"
	strict, err := ExplicitYAMLTags(ctx.AtmosConfig)
	if err != nil {
		return false, err
	}
	node.Value = expression
	if strict {
		node.Value = (yamlsource.Source{Tag: AtmosYamlFuncCILabels, Argument: strings.TrimSpace(strings.TrimPrefix(expression, AtmosYamlFuncCILabels)), File: ctx.File, Line: node.Line, Column: node.Column}).Encode()
	}
	node.Content = nil
	return true, nil
}

// handleStarlarkTag defers a scalar body, preserving its source through merging.
func handleStarlarkTag(ctx TagContext, node *yaml.Node, _ string) (bool, error) {
	if node.Kind != yaml.ScalarNode {
		return false, fmt.Errorf("%w: !starlark requires a scalar function body in %s:%d", errUtils.ErrInvalidStackManifest, ctx.File, node.Line)
	}
	node.Value = starlarksource.FromNode(node, ctx.File).Encode()
	node.Tag = "!!str"
	return true, nil
}
