---
title: Atmos Versioning
sidebar_label: Versioning
sidebar_position: 4
---

Atmos follows the <a href="https://semver.org/" target="_blank">Semantic Versioning (SemVer)</a> convention: <code>major.minor.patch.</code>

Incompatible changes increment the <code>major</code> version, adding backwards-compatible functionality increments the <code>minor</code> version,
and backwards-compatible bug fixes increment the <code>patch</code> version.

## Release Schedule

### Major Release

A major release will be published when there is a breaking change introduced in `atmos`.
Several release candidates will be published prior to a major release in order to get feedback before the final release.
An outline of what is changing and why will be included with the release candidates.

### Minor Release

A minor release will be published when a new feature is added or changes that are non-breaking are introduced.
We will heavily test any changes so that we are confident with the release, but with new code comes the potential for new issues.

### Patch Release

A patch release will be published when bug fixes were included, but no breaking changes were introduced.
To ensure patch releases can fix existing code without introducing new issues from the new features, patch releases will always be published prior to
a minor release.

## Changelog

To see a list of all notable changes to `atmos` please refer to
the <a href="https://github.com/cloudposse/atmos/blob/master/CHANGELOG.md" target="_blank">changelog</a>.
It contains an ordered list of all bug fixes and new features under each release.
