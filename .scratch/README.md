# Scratch Directory

This directory contains temporary working files, planning documents, and analysis that are not part of the permanent codebase.

## Purpose

- **Planning documents** - Analysis, brainstorming, design explorations
- **Working notes** - Temporary files during development
- **Research** - Investigation findings, comparisons, evaluations
- **Draft documents** - Content that may eventually move to proper locations

## Convention

Files in this directory:
- Are temporary and may be deleted at any time
- Should not be referenced from production code or documentation
- Are gitignored to avoid cluttering the repository
- Can be used freely during development without cleanup obligation

## Migration

When content here becomes permanent:
- **Agent designs** → `.claude/agents/`
- **PRDs** → `docs/prd/`
- **Documentation** → `website/docs/`
- **Code** → appropriate package directories
