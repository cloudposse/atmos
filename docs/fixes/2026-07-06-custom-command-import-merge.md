# Fix: custom command imports preserve sibling command trees

**Date:** 2026-07-06

## Problem

Custom command definitions loaded from explicit imports, glob imports, nested
imports, and `atmos.d` command files could replace previously loaded command
trees instead of merging with them.

This was especially visible when commands used path-style names or nested
`commands:` blocks. Loading a later file could keep one branch of the command
tree while dropping sibling commands from earlier files.

## Fix

Custom command arrays are normalized and merged by command path. Nested command
children are preserved, sibling branches remain available, and project-local
commands can still override matching imported commands.

## Tests

```shell
go test ./pkg/config -run 'TestImportCommandMerging|TestImportCommandMergingEdgeCases|TestImportCommandMergingNestedCommands|TestImportCommandMergingPathNamesPreserveDeepMerge|TestNormalizeCommandArray|TestMergeCommandArrays|TestMergeConfigFilePathCommandsAcrossFiles|TestLoadAtmosDPathCommandsPreservesSiblings' -count=1
```
