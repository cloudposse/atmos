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
- [epiclabs-io/diff3](https://github.com/epiclabs-io/diff3) - **Shipped**: used directly by `pkg/generator/merge/text_merger.go`
- [nasdf/diff3](https://github.com/nasdf/diff3) - Considered during research, not what shipped
- [git2go](https://github.com/libgit2/git2go) - libgit2 bindings (advanced, not used)

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
   - Uses `epiclabs-io/diff3` (shipped; `nasdf/diff3` was considered during
     research but isn't what shipped — see research.md)
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
- [x] Text merge implementation (shipped with `epiclabs-io/diff3`, not the
      originally planned `nasdf/diff3`)
- [x] YAML merge implementation (custom with `yaml.v3`)
- [x] Base content retrieval — shipped as git-ref-based (`--base-ref`, via
      `pkg/generator/storage.GitBaseStorage`), **not** the originally planned
      `.atmos/init/base/` on-disk snapshot (that file store was never built)
- [x] Generator integration
- [x] CLI flag `--merge-strategy` (both `atmos init` and `atmos scaffold
      generate`); `--max-changes` was **not** implemented as a flag (the
      merger's conflict-percentage threshold is an internal hardcoded default)
- [x] `--dry-run` combined with `--update` on `atmos scaffold generate` (runs
      the real merge and previews create/update/conflict status without
      writing); **`atmos init` still has no `--dry-run` flag at all** — a real,
      open gap, not a "future" line item
- [ ] Documentation and tests (this document set is being reconciled with the
      shipped implementation; `pkg/generator/merge/*_test.go` has test coverage)

## Key Decisions

### Shipped: epiclabs-io/diff3 for Text

`pkg/generator/merge/text_merger.go` imports `github.com/epiclabs-io/diff3`
directly. This research originally recommended `nasdf/diff3` (below); the
richer conflict-detection API of `epiclabs-io/diff3` (structured conflict
info, customizable labels) is what shipped instead.

**Originally recommended: nasdf/diff3** (not what shipped)

**Why it was considered:**
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

### Storage: git-ref-based, not file-snapshot (shipped differently than planned)

The original plan was to store original template content on disk for future
comparisons (`.atmos/init/base/`, shown below) — **this was never built**.
What shipped instead reads the base directly from git: `--base-ref` (defaulting
to `HEAD`) is resolved via `pkg/generator/storage.GitBaseStorage.LoadBase`,
which reads each file's blob content straight out of that git ref. There is no
on-disk base snapshot and no `.atmos/init/metadata.yaml` / `.atmos/scaffold/metadata.yaml`
in the shipped code.

**Original plan (not implemented, retained for historical context):**
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
