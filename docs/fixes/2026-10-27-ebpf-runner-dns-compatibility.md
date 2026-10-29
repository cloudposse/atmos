# Fix: Preserve DNS for the initialized StepSecurity eBPF agent

**Date:** 2026-10-27

## Summary

Recognize the initialized eBPF agent without requiring the community agent's DNS
listener. Preserve its host resolver and existing block policy.

## Context

Two CI jobs failed before running their workloads because the DNS repair waited
for `127.0.0.1:53`. The pinned Harden Runner action had selected AgentAlpha after
its tier lookup timed out. Its logs confirmed eBPF mode and an applied block
policy; this mode does not run the community DNS proxy.

## Changes

Require the active service, block configuration, initialized status, and agent
records for AgentAlpha, eBPF mode, the current job correlation ID, and the applied
block policy before preserving DNS. Keep the bounded listener wait and resolver
repair for other modes. Do not modify the endpoint policy, firewall, or Docker.

## Validation

All 111 hardening action and diagnostic tests pass. Tests cover delayed
initialization, stale job evidence, unknown agents, other network modes, missing
policy evidence, and existing community-agent repair and cleanup behavior.
Replaying both failed jobs' captured initialization logs reproduces the listener
failure with the original code and succeeds with the updated code without DNS
writes. Hosted CI verification remains pending publication.

## Follow-ups

Verify the updated action on hosted CI after publication.
