---
title: Configure Repository
sidebar_position: 3
sidebar_label: Configure Repository
---

This example provides a simple filesystem layout that looks like this:

```console
   │  
   │   # Centralized components configuration
   ├── stacks/
   │   │
   │   └── $stack.yaml
   │  
   │   # Components are broken down by tool
   ├── components/
   │   │
   │   ├── terraform/   # root modules in here
   │   │   ├── infra/
   │   │   ├── mixins/
   │   │   ├── test/test-component/
   │   │   └── top-level-component1/
   │   │
   │   └── helmfile/  # helmfiles are organized by chart
   │       ├── echo-server/
   │       └── infra/infra-server
   │  
   │   # Root filesystem for the docker image (see `Dockerfile`)
   ├── rootfs/
   │
   │   # Makefile for building the CLI
   ├── Makefile
   │   # Atmos CLI configuration
   ├── atmos.yaml
   │  
   │   # Docker image for shipping the CLI and all dependencies
   └── Dockerfile (optional)
```
