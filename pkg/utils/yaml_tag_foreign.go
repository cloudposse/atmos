package utils

import (
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Foreign YAML tags are tags that are not Atmos YAML functions but that a
// subsystem either knows how to rewrite in place, or recognizes well enough
// to explain why they cannot be used.
//
// The first kind is a tag with a native long-form spelling the rest of Atmos
// already understands: a CloudFormation short-form intrinsic such as `!Ref`
// inside an aws/cloudformation inline (or `!include`d) template rewrites to
// `{Ref: ...}`, so a template file can be included verbatim and still carry
// Atmos YAML functions alongside CloudFormation's own tags. The second kind is
// a tag from another tool that Atmos deliberately does not emulate, such as a
// Rain `!Rain::Env` directive: the walker still rejects it, but with a hint
// that names the Atmos-native replacement instead of the generic "supported
// tags are" list.
//
// Subsystems register both kinds at init time; the walker consults this
// registry before deciding a tag is unsupported.

// ForeignTagRewriter rewrites node (whose tag is the registered foreign tag)
// in place. The walker recurses into the rewritten node's children afterward
// under the same policy, so nested Atmos functions and further foreign tags
// inside the rewritten value are still resolved.
type ForeignTagRewriter func(node *yaml.Node) error

// ForeignTagHint returns hints for an unsupported tag the registrant
// recognizes (one entry per hint line), or nil when the tag is not one of
// its own.
type ForeignTagHint func(tag string) []string

var (
	foreignTagMu        sync.RWMutex
	foreignTagRewriters = map[string]ForeignTagRewriter{}
	foreignTagHints     []ForeignTagHint
)

// RegisterForeignTagRewriter registers rewrite for tag (including the leading
// `!`). Registering the same tag twice replaces the earlier rewriter.
func RegisterForeignTagRewriter(tag string, rewrite ForeignTagRewriter) {
	defer perf.Track(nil, "utils.RegisterForeignTagRewriter")()

	foreignTagMu.Lock()
	defer foreignTagMu.Unlock()
	foreignTagRewriters[strings.TrimSpace(tag)] = rewrite
}

// RegisterForeignTagHint registers a hint provider consulted when the walker
// rejects a tag no Atmos function and no rewriter handles.
func RegisterForeignTagHint(hint ForeignTagHint) {
	defer perf.Track(nil, "utils.RegisterForeignTagHint")()

	foreignTagMu.Lock()
	defer foreignTagMu.Unlock()
	foreignTagHints = append(foreignTagHints, hint)
}

func lookupForeignTagRewriter(tag string) (ForeignTagRewriter, bool) {
	foreignTagMu.RLock()
	defer foreignTagMu.RUnlock()
	rewrite, ok := foreignTagRewriters[tag]
	return rewrite, ok
}

func lookupForeignTagHint(tag string) []string {
	foreignTagMu.RLock()
	defer foreignTagMu.RUnlock()
	for _, hint := range foreignTagHints {
		if lines := hint(tag); len(lines) > 0 {
			return lines
		}
	}
	return nil
}
