# Agent Analysis Against CLAUDE.md

## Executive Summary

This document analyzes existing agents against CLAUDE.md requirements and provides recommendations for updates and new agents.

## Existing Agent Gap Analysis

### 1. changelog-writer ✅ WELL ALIGNED
**Strengths:**
- Clear scope (blog announcements only, not technical docs)
- Proper front matter requirements
- Author requirements aligned with Docusaurus
- Documentation link verification process
- Quality checklist

**Missing CLAUDE.md Context:**
- ✅ Already mentions verifying documentation links
- ✅ Has proper file naming requirements (.mdx format)
- ✅ References CLAUDE.md patterns

**Recommendation:** No significant changes needed.

---

### 2. prd-writer ⚠️ NEEDS UPDATES
**Strengths:**
- Good coverage of architectural patterns
- Mentions registry patterns, interface-driven design
- References CLAUDE.md
- PRD writing guidelines

**Missing CLAUDE.md Context:**
- ❌ Should explicitly mention **Comment Preservation** (MANDATORY)
- ❌ Should reference **Golden Snapshot testing** requirements
- ❌ Missing **Performance Tracking** guidance (`defer perf.Track()`)
- ❌ Should mention **Schema updates** requirement when adding config options
- ⚠️ Test strategy mentioned but not emphasized enough (80%+ coverage)

**Recommendations:**
1. Add section on Comment Preservation (NEVER delete comments without strong reason)
2. Include guidance on updating schemas in `pkg/datafetcher/schema/`
3. Emphasize performance tracking in all public functions
4. Strengthen test strategy requirements
5. Add golden snapshot testing considerations

---

### 3. bug-investigator ⚠️ NEEDS UPDATES
**Strengths:**
- Test-driven bug fixing approach
- Mentions `cmd.NewTestKit(t)` for test isolation
- References mock generation

**Missing CLAUDE.md Context:**
- ❌ Should mention **Comment Preservation** when refactoring
- ❌ Missing **Error Handling** requirements (wrap with static errors from errors/errors.go)
- ❌ Should reference **Pre-commit hooks** requirement (never use --no-verify)
- ❌ Missing **Performance Tracking** for any new functions
- ⚠️ Should mention updating schemas if bug fix changes config behavior

**Recommendations:**
1. Add error handling requirements (use static errors, proper wrapping)
2. Include comment preservation guidance
3. Add performance tracking requirement for new/refactored functions
4. Mention schema updates if applicable
5. Reference pre-commit hook compliance

---

### 4. feature-development-orchestrator ⚠️ NEEDS UPDATES
**Strengths:**
- Comprehensive development lifecycle
- Mentions PRD creation
- References testing

**Missing CLAUDE.md Context:**
- ❌ Should explicitly require **Registry Pattern** for commands and providers
- ❌ Missing **Comment Preservation** guidance
- ❌ Missing **Performance Tracking** requirement
- ❌ Should mention **Schema updates** for new config options
- ❌ Missing **Golden Snapshot** testing for CLI output
- ⚠️ Options pattern not emphasized enough
- ⚠️ Cross-platform compatibility not mentioned

**Recommendations:**
1. Strengthen registry pattern requirement (MANDATORY for commands/providers)
2. Add performance tracking to implementation checklist
3. Include schema update requirements
4. Add golden snapshot testing for CLI commands
5. Emphasize options pattern over parameter drilling
6. Add cross-platform compatibility requirements (Linux/macOS/Windows)
7. Include comment preservation in refactoring guidance

---

### 5. lint-resolver ✅ WELL ALIGNED
**Strengths:**
- Strong enforcement of linting rules
- Never suggest --no-verify
- Good understanding of pre-commit hooks
- References CLAUDE.md patterns

**Missing CLAUDE.md Context:**
- ✅ Already well-aligned with CLAUDE.md requirements
- ⚠️ Could mention comment preservation when refactoring for complexity

**Recommendation:** Add note about preserving comments during complexity refactoring.

---

### 6. pr-manager ⚠️ NEEDS MINOR UPDATES
**Strengths:**
- Good PR structure
- Proper labeling guidance
- Blog post requirements for minor/major

**Missing CLAUDE.md Context:**
- ⚠️ Should reference that blog posts need PR opener's GitHub username as author
- ⚠️ Could mention verifying test coverage meets 80%+ threshold

**Recommendations:**
1. Add reminder about using PR opener's username in blog post authors
2. Include test coverage verification step
3. Reference schema updates if applicable

---

### 7. pr-review-resolver ✅ MOSTLY ALIGNED
**Strengths:**
- Good GitHub API management
- Systematic review resolution
- References CLAUDE.md

**Missing CLAUDE.md Context:**
- ⚠️ Should mention comment preservation when addressing review feedback
- ⚠️ Could reference error handling requirements

**Recommendation:** Add comment preservation reminder when making changes.

---

### 8. tech-docs-writer ⚠️ NEEDS UPDATES
**Strengths:**
- Good documentation structure
- References PRDs
- Mentions existing examples

**Missing CLAUDE.md Context:**
- ❌ Missing **Documentation link verification** process (from CLAUDE.md)
- ❌ Should mention **Golden Snapshot** documentation for CLI output
- ⚠️ Could reference schema documentation when documenting config options

**Recommendations:**
1. Add documentation link verification requirements (same as changelog-writer)
2. Include golden snapshot documentation for CLI commands
3. Reference schema files when documenting configuration

---

## Summary of Common Missing Elements

Across multiple agents, these CLAUDE.md requirements are underemphasized:

1. **Comment Preservation (MANDATORY)** - Missing from: prd-writer, bug-investigator, feature-development-orchestrator
2. **Performance Tracking** - Missing from: prd-writer, bug-investigator, feature-development-orchestrator
3. **Schema Updates** - Missing from: most agents
4. **Golden Snapshot Testing** - Missing from: feature-development-orchestrator, tech-docs-writer
5. **Error Handling with Static Errors** - Missing from: bug-investigator
6. **Cross-Platform Compatibility** - Missing from: feature-development-orchestrator

---

## New Agents Required

### 1. Test Strategy Architect ⭐ HIGH PRIORITY
**Purpose:** Design comprehensive test strategies ensuring 80%+ coverage
**Key CLAUDE.md Integration:**
- Test isolation with `cmd.NewTestKit(t)`
- Mock generation with `go.uber.org/mock/mockgen`
- Table-driven tests
- Golden snapshot testing for CLI output
- 80%+ coverage requirement (CodeCov enforced)
- Unit tests with mocks over integration tests

**Collaboration:**
- Works with **Refactoring Architect** to make code more testable
- Works with **Feature Development Orchestrator** for new feature test design
- Works with **Bug Investigator** for reproduction test strategy

---

### 2. Refactoring Architect ⭐ HIGH PRIORITY
**Purpose:** Systematic refactoring to modern patterns with zero regression
**Key CLAUDE.md Integration:**
- Registry pattern for extensibility
- Interface-driven design with DI
- Options pattern instead of many parameters
- Comment preservation (MANDATORY)
- Performance tracking for refactored functions
- File organization (<600 lines per file)
- Package organization (avoid utils bloat)
- Test coverage expansion during refactoring

**Collaboration:**
- Works with **Test Strategy Architect** to ensure testability
- Works with **PRD Writer** for refactoring plans
- May trigger **Lint Resolver** for complexity issues

---

### 3. Security Auditor ⭐ MEDIUM PRIORITY
**Purpose:** Review code for security vulnerabilities and credential exposure
**Key CLAUDE.md Integration:**
- Keyring usage patterns (system/file/memory)
- Environment variable handling (ATMOS_ prefix)
- Authentication patterns (AWS IAM Identity Center, SAML)
- Credential storage (never store long-term access keys)
- No secrets in logs
- Cross-platform security considerations

**Collaboration:**
- Works with **Bug Investigator** for security-related bugs
- Works with **Feature Development Orchestrator** for auth features
- Works with **PR Review Resolver** on security review comments

---

## Implementation Plan

### Phase 1: Update Existing Agents (1-2 hours)
1. Add comment preservation guidance to relevant agents
2. Add performance tracking requirements
3. Add schema update requirements
4. Strengthen test strategy emphasis
5. Add golden snapshot testing references

### Phase 2: Create New Agents (3-4 hours)
1. **Test Strategy Architect** - Most critical, needed first
2. **Refactoring Architect** - Works closely with Test Strategy Architect
3. **Security Auditor** - Specialized but important for auth/credential features

### Phase 3: Integration & Documentation (1 hour)
1. Document agent collaboration workflows
2. Update agent descriptions with cross-references
3. Test agent interactions

---

## Agent Collaboration Workflows

### Workflow 1: Feature Development with Tests
```
User Request → Feature Development Orchestrator
              ↓
          PRD Writer (creates PRD)
              ↓
          Test Strategy Architect (designs test plan)
              ↓
          Feature Development (implementation)
              ↓
          Lint Resolver (if needed)
              ↓
          Tech Docs Writer (documentation)
              ↓
          Changelog Writer (announcement)
              ↓
          PR Manager (create PR)
```

### Workflow 2: Refactoring with Test Improvement
```
User Request → Refactoring Architect (analyze & plan)
              ↓
          Test Strategy Architect (improve test coverage)
              ↓ (parallel)
          Refactoring + Test Writing
              ↓
          Lint Resolver (if complexity issues)
              ↓
          PR Manager (create refactoring PR)
```

### Workflow 3: Security-Critical Feature
```
User Request → Feature Development Orchestrator
              ↓
          Security Auditor (early review)
              ↓
          PRD Writer (with security requirements)
              ↓
          Test Strategy Architect (security test scenarios)
              ↓
          Implementation
              ↓
          Security Auditor (final review)
              ↓
          PR Manager
```

---

## Next Steps

1. ✅ Create detailed agent specifications for new agents
2. ✅ Implement Test Strategy Architect agent
3. ✅ Implement Refactoring Architect agent
4. ✅ Implement Security Auditor agent
5. ✅ Update existing agents with missing CLAUDE.md context
6. ✅ Document agent collaboration patterns
7. ✅ Test agent interactions in real scenarios
