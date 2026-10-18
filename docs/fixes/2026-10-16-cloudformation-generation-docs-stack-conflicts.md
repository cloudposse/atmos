# Fix: Preserve migration and render help updates across the stack

**Date:** 2026-10-16

## Summary

Resolve conflicts between the concise migration guide and corrected catalog imports, and synchronize the generated render-help recording.

## Context

Earlier stack layers corrected a missing catalog import and regenerated CloudFormation render help. The later documentation rewrite removed that example and carried a different recording, so combining those layers produced conflicts.

## Changes

- Retain the concise migration guide and restore its small catalog/deployment example with `import: catalog/vpc-defaults` and the final `path: template.yaml` schema.
- Copy the authentic regenerated render-help cast from the earlier CI layer without editing recorded events.

## Validation

The guide passes a three-way merge against the original base and corrected lower-layer version without further changes. All 26 website prebuild tests, all 59 navigation tests, applicable pre-commit hooks, and fix-record validation passed. The cast is byte-identical to the freshly regenerated earlier-layer artifact and its neutral help text was checked. The website production build passed with existing MDX normalization and broken-anchor warnings. Both YAML snippets parse and the deployment import matches the inherited abstract catalog component.

## Follow-ups

Verify the cumulative stack merge, then publish the conflict resolution and check CI and review on the final commit.
