# Fix: Document verified unattended Ansible execution

**Date:** 2026-10-22

## Summary

Correct the skill's blanket claim that Atmos Ansible requires an interactive terminal, and align the playbook command documentation.

## Context

The skill directed CI users away from `atmos ansible playbook`, although the command executes playbooks with piped streams successfully when no prompt is required.

## Changes

Describe both interactive and unattended execution, with prompt-free authentication and vault input for CI. Document `--command` and its compatibility alias on the playbook command page.

## Validation

Actual local playbooks passed with piped streams, including inherited variables, inventory selection, check mode and encrypted vault inputs. Separate PTY cases answered the native step and vault-password prompts successfully. SSH and privilege escalation used an isolated local container and asserted the effective remote user.

## Follow-ups

None.
