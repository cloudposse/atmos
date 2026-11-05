# Fix: `aws cfn get template` pretty-prints as block-style YAML instead of echoing AWS's raw response

**Date:** 2026-09-09

## Summary

`atmos aws cfn get template` wrote CloudFormation's `GetTemplate` response body to
the data channel verbatim. CloudFormation commonly stores/returns a deployed
template as JSON regardless of how it was originally authored, so a user
displaying a template they wrote in YAML could see raw, unindented JSON
instead. The command now re-serializes the body as pretty-printed, block-style
YAML via a new `formatTemplateAsBlockYAML` helper, while still preserving
CloudFormation's intrinsic function tags (`!Ref`, `!Sub`, `!GetAtt`, ...) and
their long-form equivalents (`{"Fn::Sub": ...}`) exactly.

## Context

Found during a real-AWS field-test/DX session: a user asked why `get template`
didn't use "our standard YAML output method which pretty prints it"
(`pkg/data.WriteYAML`). Investigation showed `data.WriteYAML` itself is a
plain `yaml.Marshal` and wasn't directly reusable here, since a naive
unmarshal into `map[string]any` would silently discard CFN's custom intrinsic
tags — corrupting the displayed template. The existing `formatTemplate`
helper (used by `aws cfn fmt`) does preserve tags via `yaml.v3`'s `Node` API,
but it also preserves each node's original *style* — so JSON's flow notation
(`{...}`/`[...]`) survives untouched, just re-indented. `aws cfn fmt`
genuinely wants that (don't reformat a user's own file's flow/block choices),
but `get template` displays AWS's own internal representation, which has no
comments or intentional style to preserve — so a real "pretty print" needs to
force block style while still keeping tags intact.

## Changes

- `pkg/component/aws/cloudformation/fmt.go`:
  - Added `forceBlockStyle(node *yaml.Node)`: recursively resets every node's
    `Style` field to 0 (the encoder's default heuristics), leaving `Tag`
    untouched. Only `Style` controls flow-vs-block/quoting presentation;
    `Tag` carries the intrinsic function short forms.
  - Added `formatTemplateAsBlockYAML(body string) (string, error)`: parses
    into a `yaml.Node`, applies `forceBlockStyle`, and re-encodes with
    `yamlIndent` — the same encoder settings as `formatTemplate`, but always
    normalized to block style regardless of the source's original
    presentation. Reused the existing `wrapFmt` constant (from
    `changeset.go`, same package) for its error wraps rather than repeating
    the `"%w: %w"` literal a 4th time in this file.
  - `formatTemplate` itself (used by `aws cfn fmt`) is untouched — it still
    preserves a local file's existing style/comments, which is correct for
    that command.
- `pkg/component/aws/cloudformation/get.go`: `runGetTemplate` now calls
  `formatTemplateAsBlockYAML(body)` before writing to the data channel and
  populating `summary["template"]`, instead of writing the raw AWS response
  body verbatim.

## Validation

- Manual verification (temporary test, removed): a JSON body with long-form
  intrinsic functions (`{"Ref": "Marker"}`, `{"Fn::Sub": "..."}`) renders as
  normal YAML mapping keys, unchanged in meaning. A YAML body with short-form
  tags (`!Ref Marker`, `!Sub 'hello ${AWS::StackName}'`) round-trips with the
  tags intact.
- `pkg/component/aws/cloudformation/get_test.go`:
  - Updated `TestRunGetTemplate_Success`'s expected output for the new
    encoder-default quoting (trailing newline, `"2010-09-09"` re-quoted per
    `yaml.v3`'s own heuristic for an ambiguous date-like scalar).
  - Added `TestRunGetTemplate_JSONBodyPrettyPrintedAsYAML`: proves a JSON
    `TemplateBody` renders as block-style YAML (asserts the output contains
    no `{` and does contain properly-indented `key:` lines) rather than being
    echoed as flow-style/JSON.
- `go build ./...` — clean.
- `go test ./pkg/component/aws/cloudformation/...` — all pass.
- `GOTOOLCHAIN=go1.26.6 ./custom-gcl run --config=.golangci.yml --new-from-rev=origin/main ./pkg/component/aws/cloudformation/...` — zero findings on the changed files (one `revive: add-constant` finding was fixed by reusing `wrapFmt`; the only remaining findings are pre-existing `dupl` hits in `executor_test.go` from unrelated, concurrently-landed test additions).

## Follow-ups

None.
