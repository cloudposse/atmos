---
title: Packer + Docker
tags: [Automation]
cast:
  file: /casts/examples/packer-docker/build.cast
  title: atmos Packer Docker build
---

# Example: Packer Docker Image

Minimal Atmos setup demonstrating the [Packer component type](https://atmos.tools/components/packer) using Packer's Docker builder — no cloud credentials required.

## What You'll See

- Packer [component](https://atmos.tools/components/packer) configuration in an Atmos stack
- Packer's `docker` builder pulling `alpine:3.19`, running a shell provisioner, and committing the result as a local image — no AMIs, no cloud credentials, and nothing pushed anywhere (there's no post-processor)
- The Atmos [toolchain](https://atmos.tools/cli/configuration/toolchain) installing Packer automatically, pinned as a component-level tool dependency

## Prerequisites

- Docker or Podman running locally (the `docker` builder talks to the local container runtime; `atmos packer init` only needs network access to download the plugin)

## Try It

```shell
cd examples/packer-docker

# Download the Docker plugin (network access only — no daemon required)
atmos packer init alpine -s alpine

# Build the image (requires Docker or Podman running locally)
atmos packer build alpine -s alpine
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Atmos configuration: Packer component path + toolchain-managed Packer install |
| `stacks/alpine.yaml` | Single flat stack declaring the `alpine` Packer component |
| `components/packer/alpine/image.pkr.hcl` | Packer template using the Docker builder |
