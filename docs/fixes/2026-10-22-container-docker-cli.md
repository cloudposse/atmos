# Fix: Include the Docker CLI in the Atmos image

**Date:** 2026-10-22

## Summary

The Atmos container image includes the `docker` command needed by container workflows and commands that connect to a Docker daemon.

## Context

Debian trixie packages the Docker daemon and client separately. Installing `docker.io` with recommended packages disabled installs `dockerd` but omits the `docker` executable. Building the current Dockerfile reproduced `docker: not found` twice, contradicting the documented container capabilities.

## Changes

Install `docker-cli` explicitly alongside `docker.io`. Keep the existing package recommendation policy and runtime dependencies.

## Validation

Built the actual Dockerfile for `linux/arm64`, using the published Atmos 1.230.1 binary as its release input. Before the fix, `docker --version` exited 127 while `dockerd` existed. The rebuilt image reports Docker CLI 26.1.5, connects to the local daemon through its socket, and launches a disposable cached Alpine container that prints the expected marker. No remote account was used.

## Follow-ups

The fixed package selection applies to newly built images; existing published image tags remain unchanged.
