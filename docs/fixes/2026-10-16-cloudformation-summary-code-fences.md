# Fix: Preserve literal CloudFormation summary output

## Problem

CloudFormation CI summaries used fixed triple-backtick fences around output and errors.
A backtick fence in either value could end the block and turn subsequent command text into
headings or other summary content.

## Changes

The five built-in summary templates use a fence longer than any backtick run in the command
output or error messages. The original values remain unchanged for custom templates.
The CloudFormation skill also clarifies that render inputs can require authenticated service
calls even though rendering does not deploy or validate a template through CloudFormation.
Its diff entry now documents preview cleanup errors instead of promising that cleanup always succeeds.

## Validation

Regression tests parse all five rendered summaries as Markdown and confirm that embedded
three-, four-, and twelve-backtick runs cannot create headings or truncate output/error blocks.
A no-backtick case confirms that ordinary content is preserved as well.
