# Product Requirements Documents (PRDs)

This directory contains Product Requirements Documents for Atmos features.

## Purpose

PRDs serve as the source of truth for feature design and implementation. They document:

- **Problem Statement**: What problem are we solving and why
- **Goals & Non-Goals**: What we're trying to achieve and what's out of scope  
- **User Stories**: Who will use this and how
- **Design Decisions**: Technical choices made and alternatives considered
- **Implementation Plan**: How we'll build and roll out the feature
- **Success Metrics**: How we'll measure if the feature is successful

## Format

PRDs follow a consistent structure. See existing PRDs for examples:

- `vendor-symlink-security.prd.md` - Symlink security for vendor operations (CVE-2025-8959)

## When to Write a PRD

Create a PRD when:

- Adding a significant new feature
- Making breaking changes
- Implementing security features
- Changing core architecture
- Adding new configuration options that affect users

## How to Use PRDs

### For Implementers

1. Read the relevant PRD before starting implementation
2. Follow the technical specification and implementation plan
3. Ensure all acceptance criteria are met
4. Update the PRD if requirements change during implementation

### For Reviewers

1. Check that implementations match the PRD specifications
2. Verify that all user stories are addressed
3. Ensure success metrics can be measured
4. Confirm documentation matches the design

### For Product Decisions

1. Use PRDs to document design decisions
2. Include alternatives considered and why they were rejected
3. Link to relevant issues, discussions, and references
4. Keep PRDs updated as features evolve

## Contributing

When adding a new PRD:

1. Use the existing format as a template
2. Name the file descriptively: `feature-name.prd.md`
3. Include all required sections
4. Get review from stakeholders before implementation
5. Link to the PRD from related issues and PRs