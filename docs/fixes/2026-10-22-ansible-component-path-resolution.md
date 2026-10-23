# Fix: Resolve Ansible components from filesystem paths

**Date:** 2026-10-22

## Summary

Ansible components can be selected by absolute path, relative path, or `.` inside the component directory, using their configured Ansible base path.

## Context

The field fixture ran successfully by component name but rejected each path form. The reverse path detector tried Terraform, Helmfile, and Packer only, even though its base-path resolver already supported Ansible.

## Changes

Include Ansible in reverse path detection and in component-directory diagnostics. Existing base-path precedence, symlink handling, and configuration-directory guards remain in use.

## Validation

`TestExtractAnsibleComponentPaths` failed before the fix and passed afterward, exercising absolute, relative, and current-directory paths. Existing reverse path tests passed in the targeted run. Before/after logs are in `.context/skills-field-test/init-ansible-{before,after}.log`.

Actual Ansible playbooks succeeded by name, absolute path, relative path, and `.` with merged vars/env (`fixed-ansible-name/absolute/relative/dot`).

## Follow-ups

None.
