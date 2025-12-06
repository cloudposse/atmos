# Three-Way Merge Documentation

This directory contains all documentation related to the 3-way merge implementation for Atmos template updates.

## Documents

### [implementation.md](./implementation.md)
**Main PRD** - Product Requirements Document for the 3-way merge feature.

Contains:
- Problem statement and user requirements
- Solution architecture (dual-strategy: text + YAML)
- Implementation details and API design
- Integration examples
- Timeline and success criteria
- Prior art and references

**Start here** to understand what we're building and why.

### [research.md](./research.md)
**Deep Research** - Comprehensive research on existing solutions and Go libraries.

Contains:
- Cruft deep dive (our primary inspiration)
- Copier analysis (alternative approach)
- Git merge implementation details
- Go library evaluation (nasdf/diff3, epiclabs-io/diff3, git2go)
- YAML-aware merging research
- Recommendation matrix

**Read this** for technical background and implementation options.

## Quick Links

### External References

**Template Update Tools:**
- [Cruft](https://github.com/cruft/cruft) - Cookiecutter template updates (Python)
- [Copier](https://github.com/copier-org/copier) - Modern templating with updates (Python)

**Go Libraries:**
- [nasdf/diff3](https://github.com/nasdf/diff3) - Pure Go diff3 (recommended for text)
- [epiclabs-io/diff3](https://github.com/epiclabs-io/diff3) - Alternative with customization
- [git2go](https://github.com/libgit2/git2go) - libgit2 bindings (advanced)

**Academic:**
- [Diff3 Paper](https://www.cis.upenn.edu/~bcpierce/papers/diff3-short.pdf) - Formal investigation (UPenn 2007)

**Git Documentation:**
- [git merge-file](https://git-scm.com/docs/git-merge-file) - Git's merge command
- [git merge](https://git-scm.com/docs/git-merge) - Merge strategies

## Overview

### What We're Building

A 3-way merge system for **template updates** in `atmos scaffold` and `atmos init`. Intelligently merges template updates while preserving user customizations to generated files.

**Dual-strategy approach**:

1. **Text-based merge** - For general files (.tf, .md, .sh, etc.)
   - Uses `epiclabs-io/diff3` library
   - Line-by-line merge with conflict detection

2. **YAML-aware merge** - For config files (.yaml, .yml)
   - Custom implementation with `gopkg.in/yaml.v3`
   - Structure-aware merging at key level
   - Preserves comments and anchors

### Why This Matters

When scaffold/init templates evolve, users need to:
- ✅ Get new template features automatically
- ✅ Keep their customizations to generated files
- ✅ See conflicts when both sides changed the same thing
- ❌ NOT lose their work when running `--update`

**Current 2-way diff approach doesn't work** - it can't distinguish "unchanged" from "deleted".

**What this is NOT for**:
- ❌ Merging stack configurations (that's import/override)
- ❌ Merging component configs (that's stack inheritance)
- ❌ Version control (that's Git)

### How 3-Way Merge Works

Uses three versions:
- **Base**: Original template (what was initially generated)
- **Ours**: User's file (with customizations)
- **Theirs**: New template (with updates)

Algorithm:
```
For each section:
├─ Both unchanged → Keep
├─ Only ours changed → Use ours
├─ Only theirs changed → Use theirs
├─ Both changed identically → Use changed version
└─ Both changed differently → CONFLICT
```

### Use Cases in Atmos

1. **`atmos scaffold generate --update`** - Update generated files from scaffold templates
   - Update Terraform components when scaffold template evolves
   - Update README files, workflow configs, etc.

2. **`atmos init --update`** - Update project structure from init templates
   - Update atmos.yaml when init template adds new features
   - Update directory structure, documentation files

**Note**: `atmos init` builds on `atmos scaffold` - scaffold is the generic implementation.

## Implementation Status

- [x] Research completed
- [x] PRD written
- [ ] Text merge implementation (`nasdf/diff3`)
- [ ] YAML merge implementation (custom with `yaml.v3`)
- [ ] Base content storage (`.atmos/init/base/`)
- [ ] Generator integration
- [ ] CLI flags (`--merge-strategy`, `--max-changes`)
- [ ] Documentation and tests

## Key Decisions

### Recommendation: nasdf/diff3 for Text

**Why:**
- ✅ Pure Go (no CGO, no external binaries)
- ✅ Based on academic paper (formally correct)
- ✅ Simple API
- ✅ MIT licensed

**Alternative considered:** `git merge-file` (most battle-tested but requires git binary)

### Recommendation: Custom YAML Merger

**Why:**
- ✅ Structure-aware (fewer false conflicts)
- ✅ Comment preservation (using yaml.v3 Node API)
- ✅ Anchor preservation
- ✅ Full control over behavior

**Uses:** `gopkg.in/yaml.v3` (already in dependencies)

### Storage: .atmos/init/base/

Store original template content for future comparisons.

**Format:**
```
.atmos/
└── init/
    ├── metadata.yaml     # What was generated
    └── base/
        ├── atmos.yaml    # Original content
        └── ...
```

## Contributing

When working on 3-way merge implementation:

1. **Read implementation.md first** - Understand requirements
2. **Reference research.md** - Technical details and options
3. **Follow the plan** - Phase 1 (text), Phase 2 (YAML), Phase 3 (integration)
4. **Write tests** - Comprehensive coverage required (>80%)
5. **Update docs** - Keep these documents current

## Questions?

- Implementation questions → See [implementation.md](./implementation.md)
- Technical details → See [research.md](./research.md)
- Prior art → See research.md "Cruft Deep Dive" section
- Go libraries → See research.md "Go Library Evaluation" section
