---
slug: /features
title: Atmos Features
sidebar_label: Features
sidebar_position: 1
---

Atmos streamlines Terraform orchestration, environment, and configuration management, offering developers and DevOps a set of
powerful tools to tackle deployment challenges. Designed to be cloud agnostic, it enables you to operate consistently across
various cloud platforms. These features boost efficiency, clarity, and control across various environments, making it an
indispensable asset for managing complex infrastructures with confidence.

### Core Features
- **Terminal UI** Polished interface for easier interaction with Terraform, workflows, and commands.
- **Native Terraform Support:** Orchestration, backend generation, varfile generation, ensuring compatibility with vanilla Terraform.
- **Stacks:** Powerful abstraction layer defined in YAML for orchestrating and deploying components.
- **Components:** A generic abstraction for deployable units, such as Terraform "root" modules.
- **Vendoring:** Pulls dependencies from remote sources, supporting immutable infrastructure practices.
- **Custom Commands:** Extends Atmos's functionality, allowing integration of any command with stack configurations.
- **Workflow Orchestration:** Comprehensive support for managing the lifecycle of cloud infrastructure from initiation to maintenance.

### Configuration and Management
- **Service Catalogs and Component Libraries:** Collections of reusable components for efficient infrastructure management.
- **Schema Validation:** Ensures configurations are correct against a JSON schema.
- **Deep-merged Imports & Inheritance:** Streamlines configuration reuse and simplifies managing complex setups.
- **Mixins:** Reusable snippets of configuration to avoid repetition across stack configurations.
- **OPA Policy Controls:** Integrates Open Policy Agent for configuration-level policy enforcement, suitable for varied stack policies.
- **Automatic Component Updates** Using the [Atmos Component Updater GitHub Action](/integrations/github-actions/component-updater),
   Pull Requests are automatically generated to update components to their latest versions, ensuring the infrastructure remains continually up-to-date.

### Integrations
- **GitHub Actions Support:** Facilitates GitOps workflows with Terraform Plan, Apply, and Drift Detection.
- **Helmfile Support:** Provides declarative specifications for deploying helm charts, supporting Kubernetes management.
- **Works with Atlantis, and Spacelift:** Ensures compatibility with popular Terraform Automation & Collaboration Software (TACOS).
- **Terraform Provider** Atmos has a Terraform provider that can be used to manage Atmos configurations and stacks natively from HCL.
