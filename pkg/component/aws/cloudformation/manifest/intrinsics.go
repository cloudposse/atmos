package manifest

import (
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// shortFormIntrinsics maps each CloudFormation intrinsic function short-form YAML tag to its
// long-form key. Short forms are a CloudFormation-template YAML feature; a stack manifest is parsed
// by Atmos, which only knows its own function tags, so each one is registered with the YAML tag
// walker as a foreign tag that rewrites in place to its long form (`!Ref X` -> `{Ref: X}`). That
// lets an inline `template:` map, or a template file pulled in with `template: !include`, keep
// CloudFormation's own short-form syntax while Atmos YAML functions (`!env`, `!include`) resolve
// alongside it.
var shortFormIntrinsics = map[string]string{
	"!Ref":         "Ref",
	"!Condition":   "Condition",
	"!Sub":         "Fn::Sub",
	"!GetAtt":      "Fn::GetAtt",
	"!Join":        "Fn::Join",
	"!Select":      "Fn::Select",
	"!Split":       "Fn::Split",
	"!If":          "Fn::If",
	"!Equals":      "Fn::Equals",
	"!And":         "Fn::And",
	"!Or":          "Fn::Or",
	"!Not":         "Fn::Not",
	"!FindInMap":   "Fn::FindInMap",
	"!Base64":      "Fn::Base64",
	"!Cidr":        "Fn::Cidr",
	"!GetAZs":      "Fn::GetAZs",
	"!ImportValue": "Fn::ImportValue",
	"!Transform":   "Fn::Transform",
}

const (
	getAttTag  = "!GetAtt"
	yamlStrTag = "!!str"
	yamlSeqTag = "!!seq"
	yamlMapTag = "!!map"
)

func init() {
	RegisterIntrinsicRewriters()
	u.RegisterForeignTagHint(RainDirectiveHint)
}

// RegisterIntrinsicRewriters registers every CloudFormation short-form intrinsic as a foreign YAML
// tag with the stack-manifest tag walker. It runs from this package's init; it is exported so
// tests that construct the registry state explicitly can call it too.
func RegisterIntrinsicRewriters() {
	defer perf.Track(nil, "cloudformation.manifest.RegisterIntrinsicRewriters")()

	for tag, long := range shortFormIntrinsics {
		u.RegisterForeignTagRewriter(tag, intrinsicRewriter(tag, long))
	}
}

// ShortFormIntrinsicLongForm returns the long-form key of a CloudFormation intrinsic short-form tag
// such as "!Sub" (-> "Fn::Sub"), and whether the tag is one.
func ShortFormIntrinsicLongForm(tag string) (string, bool) {
	defer perf.Track(nil, "cloudformation.manifest.ShortFormIntrinsicLongForm")()

	long, ok := shortFormIntrinsics[strings.TrimSpace(tag)]
	return long, ok
}

// intrinsicRewriter returns the rewriter for one short-form tag. The tagged node becomes a
// single-key mapping `{<long>: <value>}` where value is the node's own content with the tag
// removed: a scalar for `!Ref`/`!Sub "..."`/`!Base64`, a sequence for `!Join`/`!Select`/`!If`,
// a mapping for `!Transform`. The one special case is the dotted scalar form of `!GetAtt`
// (`!GetAtt Role.Arn`), which CloudFormation defines as shorthand for the two-element list
// `[Role, Arn]`; the list form (`!GetAtt [Role, Arn]`) passes through unchanged.
func intrinsicRewriter(tag, long string) u.ForeignTagRewriter {
	return func(node *yaml.Node) error {
		inner := *node
		inner.Tag = ""
		if inner.Kind == yaml.ScalarNode {
			inner.Tag = yamlStrTag
			if tag == getAttTag {
				if logical, attr, ok := strings.Cut(inner.Value, "."); ok {
					inner = yaml.Node{
						Kind: yaml.SequenceNode,
						Tag:  yamlSeqTag,
						Content: []*yaml.Node{
							{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: logical},
							{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: attr},
						},
					}
				}
			}
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: long}
		*node = yaml.Node{
			Kind:    yaml.MappingNode,
			Tag:     yamlMapTag,
			Line:    node.Line,
			Column:  node.Column,
			Content: []*yaml.Node{key, &inner},
		}
		return nil
	}
}
