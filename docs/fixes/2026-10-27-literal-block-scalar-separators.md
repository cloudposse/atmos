# Fix: Keep block-scalar punctuation inside literal protection

**Date:** 2026-10-27

## Summary

Preserve commas and closing brackets inside plain block scalars tagged with
`!literal`, including template expressions following that punctuation. Preserve
embedded hashes unless whitespace separates them from the scalar value.

## Context

The raw-template protector treated commas and closing collection delimiters as
scalar terminators everywhere. For `message: !literal hello, {{ .vars.stage }}`,
the template expression escaped protection and was evaluated.

## Changes

Track flow collections separately from block scalar content. End a plain scalar
at flow separators only inside a flow collection. Recognize nested, tagged, and
anchored collections while retaining the source-scanning path for templates that
are not valid YAML before rendering. Treat `#` as a comment boundary only after
whitespace, so punctuation followed by a hash cannot expose a literal expression.

## Validation

Rendering regressions reproduce the original punctuation failures. Race-enabled
literal, source-protection, YAML utility, and stack template tests pass. Cases
cover block mappings and sequences, nested and multiline flow collections,
anchors and tags, ordinary scalar bracket text, and template control lines.
Untagged sibling expressions still render normally. Additional cases reproduce
hashes after commas, colons, opening brackets, question marks, and hyphens; hashes
after ordinary text and closing brackets remain protected, while genuine
space-separated and tab-separated comments retain normal template behavior.

## Follow-ups

None.
