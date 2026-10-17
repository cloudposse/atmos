package manifest

import (
	"errors"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// shortFormIntrinsics maps each CloudFormation intrinsic function short-form YAML tag to its
// long-form key. Short forms are a CloudFormation-template YAML feature; a stack manifest is parsed
// by Atmos, which only knows its own function tags, so they must be written in long form there.
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

// unsupportedTagInError extracts the tag from an ErrUnsupportedYamlTag message, which names it
// between single quotes ("unsupported YAML tag: '!Ref' found in file ...").
var unsupportedTagInError = regexp.MustCompile(`'(![^']+)'`)

// ShortFormIntrinsicLongForm returns the long-form key of a CloudFormation intrinsic short-form tag
// such as "!Sub" (-> "Fn::Sub"), and whether the tag is one.
func ShortFormIntrinsicLongForm(tag string) (string, bool) {
	defer perf.Track(nil, "cloudformation.manifest.ShortFormIntrinsicLongForm")()

	long, ok := shortFormIntrinsics[strings.TrimSpace(tag)]
	return long, ok
}

// UnsupportedTagHint returns a hint for a stack-manifest parse error caused by a CloudFormation
// intrinsic short-form tag, or "" when err is any other failure. The generic advice for unsupported
// tags ("rename the file to .yaml.tmpl") is wrong for these: the fix is the long form, or moving the
// template body into a file referenced by `path:`.
func UnsupportedTagHint(err error) string {
	defer perf.Track(nil, "cloudformation.manifest.UnsupportedTagHint")()

	if !errors.Is(err, errUtils.ErrUnsupportedYamlTag) {
		return ""
	}
	match := unsupportedTagInError.FindStringSubmatch(err.Error())
	if match == nil {
		return ""
	}
	long, ok := ShortFormIntrinsicLongForm(match[1])
	if !ok {
		return ""
	}
	return "`" + match[1] + "` is a CloudFormation short-form intrinsic, which stack manifests do not support: " +
		"use the long form (`" + long + ":`) in the manifest, or keep the template in a file referenced by `path:`"
}
