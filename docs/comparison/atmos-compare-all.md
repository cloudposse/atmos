# Atmos Feature Comparison

This document provides a comprehensive comparison of Atmos with similar infrastructure orchestration tools.

---

## Table of Contents

1. [Atmos Features](#atmos-features)
2. [HashiCorp Native Tools](#hashicorp-native-tools)

- [Terraform Stacks](#terraform-stacks-features)

3. [CLI Orchestration Tools](#cli-orchestration-tools)

- [Terragrunt](#terragrunt-features)
- [Terramate](#terramate-features)
- [Terraspace](#terraspace-features)

4. [Programmatic IaC Tools](#programmatic-iac-tools)

- [Pulumi](#pulumi-features)
- [CDKTF](#cdktf-features-deprecated)

5. [CI/CD Platforms](#cicd-platforms)

- [Spacelift](#spacelift-features)
- [env0](#env0-features)
- [Scalr](#scalr-features)
- [Digger](#digger-features)

6. [Feature Comparison Matrix](#feature-comparison-matrix)
7. [Summary](#summary)

---

## Tool Categories

| Category              | Tools                                    | Description                                         |
|-----------------------|------------------------------------------|-----------------------------------------------------|
| **HashiCorp Native**  | Terraform Stacks                         | Native HCP Terraform multi-deployment orchestration |
| **CLI Orchestration** | Atmos, Terragrunt, Terramate, Terraspace | Local CLI tools for Terraform orchestration         |
| **Programmatic IaC**  | Pulumi, CDKTF                            | Use programming languages instead of HCL/YAML       |
| **CI/CD Platforms**   | Spacelift, env0, Scalr, Digger           | SaaS platforms for IaC automation                   |

---

## Atmos Features

### Core Architecture

| Category                         | Features                                                    |
|----------------------------------|-------------------------------------------------------------|
| **Infrastructure Orchestration** | Terraform, Helmfile, Packer support with unified CLI        |
| **Configuration Model**          | YAML-based stacks with deep merge inheritance               |
| **Component Types**              | Terraform root modules, Helmfile releases, Packer templates |
| **Zero Lock-in**                 | Works with vanilla Terraform - no modifications required    |
| **Language**                     | Go CLI, YAML configuration                                  |

### Configuration Management

| Feature                   | Description                                                                                                                                        |
|---------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| **Stack Inheritance**     | Multi-level imports, base components, mixins                                                                                                       |
| **Template Functions**    | Go templates, Gomplate (100+ functions), Sprig                                                                                                     |
| **YAML Functions**        | `!terraform.output`, `!terraform.state`, `!store`, `!exec`, `!include`, `!env`, `!template`, `!random`, `!aws.*`, `!repo-root`, `!cwd`, `!literal` |
| **Deep Merge**            | Automatic configuration composition across layers                                                                                                  |
| **Locals**                | File-scoped, stack-level, component-level variables                                                                                                |
| **Variables**             | Component vars with inheritance and overrides                                                                                                      |
| **Settings**              | Component settings, integration settings, metadata                                                                                                 |
| **Environment Variables** | Component and stack-level env vars with inheritance                                                                                                |
| **Overrides**             | Terraform and Helmfile overrides (inline and imports)                                                                                              |
| **Providers**             | Provider configuration with inheritance                                                                                                            |

### Multi-Cloud Authentication

| Provider     | Capabilities                                                              |
|--------------|---------------------------------------------------------------------------|
| **AWS**      | IAM Identity Center, SSO, STS assume-role, ECR login, console access      |
| **Azure**    | Native authentication support, Key Vault integration                      |
| **GCP**      | Native authentication support, Secret Manager integration                 |
| **OIDC**     | Custom provider support                                                   |
| **Commands** | `auth login/logout/whoami/list/exec/env/shell/console/ecr-login/validate` |

### Validation & Governance

| Type                 | Details                                    |
|----------------------|--------------------------------------------|
| **OPA Policies**     | Rego-based policy-as-code enforcement      |
| **JSON Schema**      | Stack and component validation             |
| **EditorConfig**     | File formatting validation                 |
| **Stack Validation** | Configuration completeness and correctness |

### State & Remote Data Access

| Feature                  | Description                                                                              |
|--------------------------|------------------------------------------------------------------------------------------|
| **Store Providers**      | AWS SSM, Secrets Manager, Azure Key Vault, GCP Secret Manager, Redis, Artifactory, Vault |
| **Remote State**         | `!terraform.output`, `!terraform.state` YAML functions                                   |
| **State Sharing**        | Cross-component output references                                                        |
| **Lifecycle Hooks**      | Store outputs after terraform apply                                                      |
| **Backend Provisioning** | Automatic S3/DynamoDB backend creation                                                   |

### Vendoring & Dependencies

| Source Type         | Support                                |
|---------------------|----------------------------------------|
| **Git**             | Full support with refs, tags, branches |
| **OCI Registries**  | Docker/container registries            |
| **S3/GCS**          | Cloud storage                          |
| **HTTP/HTTPS**      | Remote URLs                            |
| **Local**           | Filesystem paths                       |
| **Mercurial**       | Version control                        |
| **Version Pinning** | Strict version control with manifests  |

### Workflow Automation

| Feature                   | Description                                   |
|---------------------------|-----------------------------------------------|
| **Workflows**             | Multi-step orchestration with retry logic     |
| **Custom Commands**       | User-defined CLI commands with templates      |
| **Lifecycle Hooks**       | Pre/post events (after-terraform-apply, etc.) |
| **Resume Capability**     | Resume from specific steps                    |
| **Step Dependencies**     | Automatic dependency handling                 |
| **Identity per Step**     | Different auth contexts per workflow step     |
| **Toolchain Integration** | Auto-detect and use tools from .tool-versions |

### CLI Commands

| Command            | Description                                                                                                                                       |
|--------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| **terraform** (tf) | Full Terraform orchestration (plan, apply, destroy, init, validate, fmt, state, workspace, output, console, import, deploy, clean, backend, etc.) |
| **helmfile** (hf)  | Kubernetes deployments (sync, apply, destroy, diff)                                                                                               |
| **packer** (pk)    | Machine image building (build, init, validate, inspect)                                                                                           |
| **workflow**       | Multi-step automation with TUI                                                                                                                    |
| **describe**       | Stack/component introspection (component, config, stacks, affected, dependents, locals, workflows)                                                |
| **list**           | Resource enumeration (components, stacks, instances, affected, workflows, themes, metadata, settings, vars, vendor)                               |
| **validate**       | Policy/schema validation (component, stacks, schema, editorconfig)                                                                                |
| **vendor**         | Dependency management (pull, diff)                                                                                                                |
| **auth**           | Multi-cloud authentication                                                                                                                        |
| **toolchain**      | Tool version management                                                                                                                           |
| **devcontainer**   | Development environments                                                                                                                          |
| **atlantis**       | Atlantis config generation                                                                                                                        |
| **aws**            | AWS-specific commands (eks update-kubeconfig)                                                                                                     |
| **pro**            | Commercial features (lock/unlock)                                                                                                                 |
| **profile**        | AWS profile management                                                                                                                            |
| **theme**          | Terminal theme management                                                                                                                         |
| **env**            | Environment variable export                                                                                                                       |
| **docs**           | Documentation access                                                                                                                              |
| **version**        | Version management                                                                                                                                |
| **completion**     | Shell completion (bash, zsh, fish, powershell)                                                                                                    |

### CI/CD Integrations

| Platform           | Actions/Features                                                                                                     |
|--------------------|----------------------------------------------------------------------------------------------------------------------|
| **GitHub Actions** | setup-atmos, terraform-plan, terraform-apply, drift-detection, drift-remediation, affected-stacks, component-updater |
| **Atlantis**       | Repository config generation, workflow integration                                                                   |
| **Spacelift**      | Admin stack detection, lock state management                                                                         |
| **GitOps**         | PR-based workflows, plan file storage (S3), metadata storage (DynamoDB)                                              |

### Developer Experience

| Feature                | Description                                          |
|------------------------|------------------------------------------------------|
| **Terminal UI**        | Interactive stack/component navigation with preview  |
| **Themes**             | Configurable terminal color themes                   |
| **Shell Completion**   | Bash, Zsh, Fish, PowerShell                          |
| **Affected Detection** | Git-based change detection for selective deployments |
| **Secret Masking**     | Gitleaks integration (120+ patterns)                 |
| **Markdown Rendering** | Rich terminal output                                 |
| **Pager Support**      | Automatic output pagination                          |

### Advanced Features

| Feature                  | Description                                    |
|--------------------------|------------------------------------------------|
| **Service Catalogs**     | Curated component collections with governance  |
| **Multiple Instances**   | Same component deployed with different configs |
| **Abstract Components**  | Base components for inheritance only           |
| **Provenance Tracking**  | Configuration source tracking                  |
| **Toolchain Management** | Tool version management (like mise/asdf)       |
| **Devcontainers**        | Docker/Podman development environments         |
| **Backend Provisioning** | Automatic S3/DynamoDB backend creation         |
| **Plan Diff Analysis**   | Detailed change comparison                     |

### Design Patterns

| Pattern                   | Description                            |
|---------------------------|----------------------------------------|
| **Component Catalog**     | Curated, versioned component libraries |
| **Mixins**                | Composable configuration fragments     |
| **Multiple Inheritance**  | Components inherit from multiple bases |
| **Layered Configuration** | Environment-specific overrides         |
| **Defaults Pattern**      | Centralized default values             |
| **Multi-Region**          | Region-aware stack organization        |
| **Multi-Account**         | Account hierarchy support              |
| **Multi-Tenant**          | Tenant isolation patterns              |

---

## HashiCorp Native Tools

### Terraform Stacks Features

**Overview**: Native HCP Terraform feature for multi-deployment orchestration with component-based architecture.

**Status**: Public Beta (as of 2024)

**Documentation**:

- [Terraform Stacks Explained](https://www.hashicorp.com/en/blog/terraform-stacks-explained)
- [Stacks Language Reference](https://developer.hashicorp.com/terraform/language/stacks)
- [Stacks in HCP Terraform](https://developer.hashicorp.com/terraform/cloud-docs/stacks)
- [Creating Stacks](https://developer.hashicorp.com/terraform/cloud-docs/stacks/create)
- [Deploy with Stacks Tutorial](https://developer.hashicorp.com/terraform/tutorials/cloud/stacks-deploy)

#### Core Architecture

| Feature                 | Description                                                        |
|-------------------------|--------------------------------------------------------------------|
| **Component-based**     | Reusable Terraform modules packaged as named components            |
| **Deployments**         | Infrastructure instances across environments, regions, or accounts |
| **Configuration Files** | `.tfstack.hcl` (stack config), `.tfdeploy.hcl` (deployment config) |
| **Language**            | HCL (HashiCorp Configuration Language)                             |
| **Platform**            | HCP Terraform only (no local execution)                            |

#### Configuration Files

| File Type             | Description                                                               |
|-----------------------|---------------------------------------------------------------------------|
| **`.tfstack.hcl`**    | Component definitions, provider configurations, variables, outputs        |
| **`.tfdeploy.hcl`**   | Deployment definitions, identity tokens, store blocks, auto-approve rules |
| **Component Blocks**  | Define individual Terraform modules with inputs                           |
| **Deployment Blocks** | Define infrastructure instances per environment                           |

#### Stack Configuration (`.tfstack.hcl`)

| Feature                   | Description                           |
|---------------------------|---------------------------------------|
| **Component Blocks**      | Define Terraform modules with inputs  |
| **Provider Declarations** | Centralized authentication workflows  |
| **Variables**             | Input values for stack customization  |
| **Outputs**               | Values exposed from the stack         |
| **Locals**                | Temporary computed values             |
| **Stack References**      | Link to other stacks for data sharing |

#### Deployment Configuration (`.tfdeploy.hcl`)

| Feature                      | Description                                  |
|------------------------------|----------------------------------------------|
| **Deployment Blocks**        | Define instances across environments/regions |
| **Deployment Groups**        | Organize and coordinate deployments          |
| **Identity Tokens**          | Secure provider authentication               |
| **Store Blocks**             | Access HCP Terraform variable sets           |
| **Auto-approve Rules**       | Conditional automatic approvals              |
| **Publish Output Blocks**    | Expose values to downstream stacks           |
| **Upstream Input Blocks**    | Consume data from other stacks               |
| **`for_each` Meta-argument** | Multi-region/multi-environment patterns      |

#### Orchestration Features

| Feature                   | Description                                   |
|---------------------------|-----------------------------------------------|
| **Sequential Rollout**    | Changes roll out one deployment at a time     |
| **Independent Lifecycle** | Changes to one deployment don't affect others |
| **Deployment Groups**     | Coordinate rollouts across environments       |
| **Staged Approach**       | Reduce risk during infrastructure updates     |
| **Change Tracking**       | Visibility across all deployments             |

#### Cross-Stack Integration

| Feature                | Description                              |
|------------------------|------------------------------------------|
| **Upstream Stacks**    | Link up to 20 stacks for data import     |
| **Downstream Stacks**  | Expose values to 25 stacks               |
| **Automatic Triggers** | Dependent stack runs when outputs change |
| **Stack References**   | Reference outputs from other stacks      |

#### State Management

| Feature              | Description                         |
|----------------------|-------------------------------------|
| **Isolated State**   | Separate state file per deployment  |
| **HCP Managed**      | State managed by HCP Terraform      |
| **Version Tracking** | Configuration version management    |
| **Plan Review**      | Deployment plan review capabilities |

#### Resource Limits (Beta)

| Limit                         | Value       |
|-------------------------------|-------------|
| **Deployments per Stack**     | 20          |
| **Components per Stack**      | 100         |
| **Resources per Stack**       | 10,000      |
| **Upstream Stack Links**      | 20          |
| **Downstream Stack Exposure** | 25          |
| **Deployments per Group**     | 1 (current) |

#### HCP Terraform Integration

| Feature                        | Description                          |
|--------------------------------|--------------------------------------|
| **Project-scoped Permissions** | Access controls at project level     |
| **Variable Sets**              | Access via store blocks              |
| **Concurrent Operations**      | Same agent pool as workspaces        |
| **Deployment Runs**            | Monitoring and management            |
| **Remote Execution**           | HCP Terraform or custom agents       |
| **API Access**                 | Programmatic creation and management |

#### Creation Methods

| Method                | Description                 |
|-----------------------|-----------------------------|
| **HCP Terraform UI**  | Web interface               |
| **Terraform CLI**     | `terraform stacks` commands |
| **HCP Terraform API** | Programmatic creation       |

#### Key Limitations

| Limitation                 | Description                             |
|----------------------------|-----------------------------------------|
| **HCP Terraform Required** | No local/open-source execution          |
| **Beta Status**            | Not production-ready                    |
| **No OpenTofu**            | HashiCorp Terraform only                |
| **Limited Deployments**    | Max 20 per stack                        |
| **No Helmfile/Packer**     | Terraform-only                          |
| **No Inheritance**         | No configuration composition like Atmos |
| **No Vendoring**           | No component versioning/vendoring       |
| **No CLI Orchestration**   | Remote execution only                   |

---

## CLI Orchestration Tools

### Terragrunt Features

**Overview**: HCL-based Terraform wrapper focused on DRY configuration and dependency management.

#### Core Architecture

| Feature                 | Description                                                                      |
|-------------------------|----------------------------------------------------------------------------------|
| **Units**               | Individual directories containing `terragrunt.hcl` as smallest deployable entity |
| **Stacks**              | Collections of units managed together as environments                            |
| **Configuration Files** | `terragrunt.hcl`, `terragrunt.hcl.json`, `terragrunt.stack.hcl`                  |
| **Language**            | HCL configuration                                                                |
| **IaC Support**         | Terraform, OpenTofu                                                              |

#### Configuration & Composition (DRY)

| Feature                         | Description                                                                  |
|---------------------------------|------------------------------------------------------------------------------|
| **Include Blocks**              | Configuration inheritance from parent folders via `find_in_parent_folders()` |
| **Multiple Includes**           | Multiple parent configurations with `expose = true`                          |
| **Locals**                      | Reusable local variables                                                     |
| **Dynamic Configuration**       | `read_terragrunt_config()` for environment-specific files                    |
| **Extra Arguments**             | Automatic CLI flag and env var injection                                     |
| **Required/Optional Var Files** | `required_var_files`, `optional_var_files`                                   |

#### Dependency Management

| Feature                      | Description                                     |
|------------------------------|-------------------------------------------------|
| **Dependency Blocks**        | Express relationships between units             |
| **Mock Outputs**             | Placeholder values for testing/planning         |
| **DAG Ordering**             | Automatic execution order based on dependencies |
| **Dependency Visualization** | `dag graph` outputs DOT format graphs           |

#### State & Backend Management

| Feature                  | Description                                            |
|--------------------------|--------------------------------------------------------|
| **Remote State Backend** | Automatic backend configuration generation             |
| **Backend Bootstrap**    | `backend bootstrap` initializes backend infrastructure |
| **Backend Migration**    | `backend migrate` transfers state between units        |
| **Backend Deletion**     | `backend delete` removes backend state                 |
| **Lock Files**           | `.terraform.lock.hcl` generation                       |
| **Smart Caching**        | `.terragrunt-cache` with `--source-update` control     |

#### Hooks & Custom Actions

| Feature                   | Description                                            |
|---------------------------|--------------------------------------------------------|
| **Before Hooks**          | Execute before Terraform commands                      |
| **After Hooks**           | Execute after commands (even on errors)                |
| **Error Hooks**           | Trigger on specific failures                           |
| **Context Variables**     | `TG_CTX_TF_PATH`, `TG_CTX_COMMAND`, `TG_CTX_HOOK_NAME` |
| **Conditional Execution** | `run_on_error` controls                                |

#### Code Generation & Scaffolding

| Feature                 | Description                                      |
|-------------------------|--------------------------------------------------|
| **Generate Blocks**     | Dynamic file generation before execution         |
| **Provider Generation** | Inject provider blocks without modifying modules |
| **Scaffold Command**    | Boilerplate-based code generation                |
| **Catalog Command**     | Interactive TUI for browsing modules             |

#### Advanced Features

| Feature                       | Description                        |
|-------------------------------|------------------------------------|
| **Custom IaC Engines**        | RPC-based plugin system            |
| **Content Addressable Store** | Infrastructure artifact management |
| **Run Reports**               | Reporting and tracking             |
| **Provider Cache Server**     | Caching for concurrent runs        |

---

### Terramate Features

**Overview**: HCL-based stack management with cloud platform for governance and drift detection.

#### Core Architecture

| Feature                 | Description                                                                                                   |
|-------------------------|---------------------------------------------------------------------------------------------------------------|
| **Configuration Files** | HCL-based (`terramate.tm.hcl`)                                                                                |
| **Blocks**              | `terramate`, `stack`, `generate_hcl`, `generate_file`, `globals`, `lets`, `map`, `script`, `assert`, `import` |
| **IaC Support**         | Terraform, OpenTofu, Terragrunt, Kubernetes                                                                   |
| **IDE Support**         | VSCode, JetBrains, Vim with LSP                                                                               |

#### Stack Management

| Feature                   | Description                                      |
|---------------------------|--------------------------------------------------|
| **Stack Operations**      | Create, nest, clone, manage, delete              |
| **Stack Metadata**        | Path, name, description                          |
| **Multiple Environments** | Workspaces, directories, TFVars, partial backend |
| **Output Sharing**        | Between environments                             |

#### Code Generation

| Feature               | Description                              |
|-----------------------|------------------------------------------|
| **Generate HCL**      | Programmatic backend/provider generation |
| **Generate Files**    | HCL, JSON, YAML generation               |
| **Ad-hoc Generation** | Dynamic file creation                    |
| **Template-based**    | Infrastructure instantiation             |

#### Change Detection

| Feature                            | Description                       |
|------------------------------------|-----------------------------------|
| **Git-based Detection**            | Identifies changes in modules     |
| **Terragrunt Dependency Tracking** | Monitors dependency modifications |
| **Terraform State Analysis**       | State-based change detection      |
| **Partial Evaluation**             | Selective change detection        |

#### Terramate Cloud Platform

| Feature                 | Description                                    |
|-------------------------|------------------------------------------------|
| **Dashboard**           | Infrastructure state visualization             |
| **Stack Inventory**     | Status tracking and listing                    |
| **Collaboration**       | PR previews, deployment tracking               |
| **Drift Management**    | Automated detection and reconciliation         |
| **Governance**          | 500+ pre-configured policies, CIS Benchmarks   |
| **Incident Management** | Failed deployment detection, Slack integration |

#### Terramate Catalyst

| Feature             | Description                              |
|---------------------|------------------------------------------|
| **Components**      | Reusable infrastructure templates        |
| **Bundles**         | Collections of components                |
| **Scaffolding**     | Self-service infrastructure provisioning |
| **Remote Catalogs** | External component sources               |

---

### Terraspace Features

**Overview**: Ruby-based Terraform framework with convention-over-configuration approach.

#### Core Architecture

| Feature           | Description                              |
|-------------------|------------------------------------------|
| **Language**      | Ruby DSL (97.2% Ruby)                    |
| **Configuration** | `.rb` (Ruby DSL), `.tf`, `.tfvars` files |
| **IaC Support**   | Terraform only                           |
| **License**       | Apache 2.0                               |

#### Stack Management

| Feature                     | Description                                    |
|-----------------------------|------------------------------------------------|
| **Multi-Stack Deployment**  | `terraspace all up` with dependency resolution |
| **Automatic DAG**           | Determines optimal deployment order            |
| **Subgraph/Subtree**        | Deploy stack subsets                           |
| **Coordinated Deployments** | Complex relationship handling                  |

#### Layering System

| Feature                   | Description                        |
|---------------------------|------------------------------------|
| **Seed Layering**         | Basic layer configuration          |
| **Full Layering**         | Identical code across environments |
| **App-specific Layering** | Application-level separation       |
| **Multi-region Layering** | Region-aware configuration         |
| **Custom Layering**       | User-defined layers                |

#### Hooks System

| Feature              | Description                |
|----------------------|----------------------------|
| **Terraform Hooks**  | Pre/post-terraform hooks   |
| **Terraspace Hooks** | Framework-specific hooks   |
| **Boot Hooks**       | Initialization hooks       |
| **Ruby Hooks**       | Ruby-based implementations |
| **Generator Hooks**  | Code generation hooks      |

#### Plugin System

| Feature                   | Description                                                                |
|---------------------------|----------------------------------------------------------------------------|
| **Cloud Plugins**         | terraspace_plugin_aws, terraspace_plugin_azurerm, terraspace_plugin_google |
| **Auto Backend Creation** | S3, Azure Storage, GCS bucket provisioning                                 |
| **Extensibility**         | Custom plugin development                                                  |

#### Testing Framework

| Feature                   | Description                          |
|---------------------------|--------------------------------------|
| **Module-level Testing**  | Reusable module validation           |
| **Stack-level Testing**   | Stack code validation                |
| **Project-level Testing** | Full project validation              |
| **Test Harness**          | Automated provision-validate-destroy |

#### Advanced Features

| Feature                 | Description                                            |
|-------------------------|--------------------------------------------------------|
| **Terrafile**           | Unified module sourcing (Git, S3, GCS, SSH)            |
| **Secrets Integration** | AWS Secrets Manager, SSM, Azure Key Vault, GCP Secrets |
| **Terraform Cloud**     | Workspace management integration                       |
| **Scaffolding**         | `terraspace new project/module` generators             |

**Note**: Terraspace Cloud has been sunset; users directed to alternatives.

---

## Programmatic IaC Tools

### Pulumi Features

**Overview**: Multi-language Infrastructure as Code platform using general-purpose programming languages.

#### Language Support

| Language                  | Status                   |
|---------------------------|--------------------------|
| **TypeScript/JavaScript** | Stable (Node.js runtime) |
| **Python**                | Stable                   |
| **Go**                    | Stable                   |
| **.NET** (C#, F#, VB.NET) | Stable                   |
| **Java**                  | Stable (JDK 11+)         |
| **YAML**                  | Stable                   |

#### Provider Ecosystem

| Feature                | Description                                                           |
|------------------------|-----------------------------------------------------------------------|
| **Registry Packages**  | 294+ packages available                                               |
| **Cloud Providers**    | AWS, Azure, GCP, Kubernetes, Docker                                   |
| **Extended Providers** | 120+ including Datadog, Cloudflare, Auth0, Vault, PostgreSQL, MongoDB |

#### State Management

| Feature                | Description                   |
|------------------------|-------------------------------|
| **Managed Backend**    | Fully-managed state storage   |
| **Concurrent Locking** | Parallel deployment support   |
| **Encryption**         | In transit (TLS) and at rest  |
| **Rollback**           | Complete history access       |
| **Self-hosted**        | On-premises deployment option |
| **SOC 2 Type II**      | Compliance certification      |

#### Secrets Management

| Feature                  | Description                                            |
|--------------------------|--------------------------------------------------------|
| **Automatic Encryption** | Secrets never in plaintext                             |
| **Transitive Tracking**  | Derived data marked as secret                          |
| **Multiple Providers**   | Pulumi Cloud, AWS KMS, Azure Key Vault, GCP KMS, Vault |
| **Pulumi ESC**           | Centralized secrets and configuration                  |

#### Policy & Governance

| Feature                    | Description                            |
|----------------------------|----------------------------------------|
| **CrossGuard**             | Policy-as-code in TypeScript/Python    |
| **150+ Built-in Policies** | Compliance and security guardrails     |
| **Continuous Auditing**    | CIS, NIST, HITRUST, PCI DSS frameworks |
| **Pulumi Neo**             | AI-powered remediation                 |

#### Advanced Features

| Feature                    | Description                              |
|----------------------------|------------------------------------------|
| **Cross-Stack References** | `StackReference` for output consumption  |
| **Automation API**         | Embed provisioning in applications       |
| **Drift Detection**        | Enterprise feature                       |
| **Kubernetes Operator**    | Native K8s API resources                 |
| **Templates**              | Container, serverless, VM, K8s templates |

---

### CDKTF Features (DEPRECATED)

**⚠️ DEPRECATED**: As of December 10, 2025, CDKTF has been archived and is no longer maintained.

#### Language Support

| Language       | Status     |
|----------------|------------|
| **TypeScript** | Deprecated |
| **Python**     | Deprecated |
| **Java**       | Deprecated |
| **C#**         | Deprecated |
| **Go**         | Deprecated |

#### Key Features (Historical)

| Feature                    | Description                                    |
|----------------------------|------------------------------------------------|
| **Provider Support**       | All Terraform providers via generated bindings |
| **Module Support**         | Terraform Registry module integration          |
| **Constructs**             | Reusable infrastructure patterns               |
| **Cross-Stack References** | Multi-stack architectures                      |
| **Testing**                | Unit testing with Jest, pytest, JUnit          |
| **HCL Interop**            | Bidirectional HCL conversion                   |

**Migration Path**: Use `cdktf synth --hcl` to transition to standard Terraform.

---

## CI/CD Platforms

### Spacelift Features

**Overview**: Enterprise CI/CD platform for Infrastructure as Code with policy enforcement.

#### IaC Support

| Tool                   | Support                              |
|------------------------|--------------------------------------|
| **Terraform/OpenTofu** | Full support with modules, providers |
| **CloudFormation**     | Full support                         |
| **Pulumi**             | C#, Go, TypeScript, Python           |
| **Terragrunt**         | Full support                         |
| **Kubernetes**         | Helm, Kustomize                      |
| **Ansible**            | Playbook automation                  |

#### Stack Management

| Feature                  | Description                                      |
|--------------------------|--------------------------------------------------|
| **Stack Dependencies**   | Coordinated multi-stack deployments              |
| **Run Types**            | Tasks, Proposed Runs, Tracked Runs, Module Tests |
| **Scheduled Automation** | Timed actions, drift detection, deletion         |
| **Worker Pools**         | Public (managed) or Private (self-hosted)        |

#### Policy as Code (OPA)

| Policy Type                 | Description                    |
|-----------------------------|--------------------------------|
| **Login Policies**          | Account access control         |
| **Access Policies**         | Stack-level permissions        |
| **Approval Policies**       | Approval/rejection authorities |
| **Plan Policies**           | Block problematic changes      |
| **Push Policies**           | Git event interpretation       |
| **Notification Policies**   | Alert routing                  |
| **Initialization Policies** | Pre-execution blocking         |
| **Task Policies**           | Task execution control         |
| **Trigger Policies**        | Automatic run selection        |

#### Governance & Security

| Feature             | Description                         |
|---------------------|-------------------------------------|
| **Blueprints**      | Standardized stack templates        |
| **Drift Detection** | Automated divergence identification |
| **RBAC**            | Role-based access control           |
| **SAML 2.0 SSO**    | Enterprise authentication           |
| **Audit Trails**    | Immutable change tracking           |
| **SOC 2 Type II**   | Compliance certification            |
| **FedRAMP**         | First IaC platform authorized       |

#### Cloud Integration

| Feature                 | Description                   |
|-------------------------|-------------------------------|
| **AWS/Azure/GCP**       | Native cloud provider support |
| **OIDC**                | Short-lived token generation  |
| **Vault Integration**   | HashiCorp Vault via OIDC      |
| **Dynamic Credentials** | Eliminate static credentials  |

#### AI Features

| Feature                     | Description                            |
|-----------------------------|----------------------------------------|
| **Spacelift Intent**        | Natural language infrastructure (Beta) |
| **Auto Provider Discovery** | Terraform schema learning              |
| **OPA Enforcement**         | Policy checks before execution         |

---

### env0 Features

**Overview**: Environment as a Service platform with FinOps and governance capabilities.

#### IaC Support

| Tool                   | Support      |
|------------------------|--------------|
| **Terraform/OpenTofu** | Full support |
| **Pulumi**             | Full support |
| **CloudFormation**     | Full support |
| **Kubernetes/Helm**    | Full support |
| **Ansible**            | Full support |
| **Terragrunt**         | Full support |

#### Deployment & Orchestration

| Feature                  | Description                         |
|--------------------------|-------------------------------------|
| **PR-Based Deployments** | Speculative plans for pull requests |
| **Custom Workflows**     | Multi-environment orchestration     |
| **Custom Flows**         | Task automation                     |
| **Parallel Deployments** | Concurrent infrastructure changes   |
| **GitOps Integration**   | Atlantis-style automation           |

#### Governance & Compliance

| Feature                    | Description                      |
|----------------------------|----------------------------------|
| **Policy Enforcement**     | Automated guardrails             |
| **RBAC**                   | Dynamic, scoped roles            |
| **Approval Workflows**     | Multi-layer approval processes   |
| **Environment Quotas/TTL** | Resource lifecycle management    |
| **Audit Logging**          | Complete activity tracking       |
| **SSO/SAML**               | Okta, Azure AD, Google Workspace |
| **SOC 2**                  | Compliance certification         |

#### Cost Management (FinOps)

| Feature                  | Description                        |
|--------------------------|------------------------------------|
| **Cost Estimation**      | Pre-deployment impact analysis     |
| **Cost Monitoring**      | Real-time tracking by team/project |
| **Budget Notifications** | Threshold alerts                   |
| **Automatic Tagging**    | Spend tracking                     |
| **Multi-Cloud**          | AWS, Azure, GCP support            |

#### Drift Management

| Feature                 | Description                    |
|-------------------------|--------------------------------|
| **Automatic Detection** | Real-time drift identification |
| **Drift Analysis**      | Root cause identification      |
| **Drift Remediation**   | Automated correction           |

#### AI Features

| Feature             | Description                             |
|---------------------|-----------------------------------------|
| **Cloud Compass**   | AI-powered IaC discovery and generation |
| **Cloud Analyst**   | AI-powered infrastructure intelligence  |
| **AI PR Summaries** | Automated change summaries              |
| **Code Optimizer**  | Infrastructure code optimization (Beta) |

---

### Scalr Features

**Overview**: Terraform Cloud alternative with unlimited concurrent runs.

#### Core Features

| Feature                       | Description                                 |
|-------------------------------|---------------------------------------------|
| **IaC Support**               | Terraform, OpenTofu, Terragrunt             |
| **Workspace Types**           | VCS-driven, CLI-driven, No Code, API-driven |
| **Remote Execution**          | Plan/apply without local dependencies       |
| **Unlimited Concurrent Runs** | No additional cost                          |

#### State Management

| Feature                   | Description                      |
|---------------------------|----------------------------------|
| **Remote Backend**        | Scalr or any TF/OpenTofu backend |
| **Storage Profiles**      | Flexible state storage           |
| **State File Management** | Configuration options            |

#### Policy & Governance

| Feature              | Description                |
|----------------------|----------------------------|
| **OPA Integration**  | Policy-as-code enforcement |
| **Checkov Scanning** | Security best practices    |
| **Policy Cascade**   | Organizational hierarchy   |
| **RBAC**             | Custom role creation       |

#### Observability

| Feature                    | Description                                |
|----------------------------|--------------------------------------------|
| **Drift Detection**        | With notifications and remediation         |
| **Centralized Reporting**  | TF versions, providers, modules, resources |
| **Pipeline Observability** | Monitoring and dashboards                  |
| **Datadog Integration**    | Event-driven workflows                     |

#### Pricing Highlights

| Feature                | Description            |
|------------------------|------------------------|
| **Unlimited Runs**     | No concurrency charges |
| **Self-hosted Agents** | No additional cost     |
| **All Features**       | Available on all tiers |
| **Free Tier**          | 600 runs/month         |

---

### Digger Features

**Overview**: CI-native Terraform automation (runs in your existing CI, also known as OpenTaco).

#### Core Architecture

| Feature                        | Description                                     |
|--------------------------------|-------------------------------------------------|
| **CI-Native**                  | Runs in GitHub Actions, GitLab CI, Azure DevOps |
| **No Separate Infrastructure** | Uses existing CI compute                        |
| **Secrets Stay Local**         | No third-party credential sharing               |
| **Open Source**                | Community-driven                                |

#### PR Automation

| Feature              | Description                       |
|----------------------|-----------------------------------|
| **Auto Plan**        | `terraform plan` on pull requests |
| **PR Comments**      | Results posted as comments        |
| **CommentOps**       | Control via PR comments           |
| **Draft PR Support** | Handle draft PRs                  |
| **Auto-merge**       | On approval workflows             |

#### State & Locking

| Feature                 | Description                         |
|-------------------------|-------------------------------------|
| **Centralized State**   | Built-in state management with RBAC |
| **Versioning**          | Rollback capabilities               |
| **PR-level Locks**      | Prevent race conditions             |
| **Distributed Locking** | DynamoDB (AWS), Cloud Storage (GCP) |

#### Policy & Security

| Feature             | Description          |
|---------------------|----------------------|
| **OPA Integration** | Policy-as-code       |
| **Checkov**         | Static code analysis |
| **Infracost**       | Cost estimation      |
| **FIPS 140**        | Compliance support   |

#### Advanced Features

| Feature                   | Description                          |
|---------------------------|--------------------------------------|
| **Terragrunt Support**    | Multi-environment orchestration      |
| **OpenTofu Support**      | Full compatibility                   |
| **Dependencies/Layering** | Complex multi-module infrastructures |
| **AI Summaries**          | Automated plan analysis              |
| **Private Runners**       | Self-hosted execution                |
| **OIDC**                  | Cloud-native authentication          |

---

## Feature Comparison Matrix

### Tool Overview

| Tool           | Type        | Language   | License     | Status         |
|----------------|-------------|------------|-------------|----------------|
| **Atmos**      | CLI         | YAML       | Apache 2.0  | Active         |
| **Terragrunt** | CLI         | HCL        | MIT         | Active         |
| **Terramate**  | CLI + Cloud | HCL        | MPL 2.0     | Active         |
| **TF Stacks**  | HCP SaaS    | HCL        | BSL         | **Beta**       |
| **Terraspace** | CLI         | Ruby       | Apache 2.0  | Active         |
| **Pulumi**     | CLI + Cloud | Multi-lang | Apache 2.0  | Active         |
| **CDKTF**      | CLI         | Multi-lang | MPL 2.0     | **Deprecated** |
| **Spacelift**  | SaaS        | N/A        | Proprietary | Active         |
| **env0**       | SaaS        | N/A        | Proprietary | Active         |
| **Scalr**      | SaaS        | N/A        | Proprietary | Active         |
| **Digger**     | CI-Native   | N/A        | Apache 2.0  | Active         |

### IaC Tool Support

| Feature            | Atmos | Terragrunt | Terramate | TF Stacks | Terraspace | Pulumi | Spacelift | env0 | Scalr | Digger |
|--------------------|:-----:|:----------:|:---------:|:---------:|:----------:|:------:|:---------:|:----:|:-----:|:------:|
| **Terraform**      |   ✅   |     ✅      |     ✅     |     ✅     |     ✅      |   ❌    |     ✅     |  ✅   |   ✅   |   ✅    |
| **OpenTofu**       |   ✅   |     ✅      |     ✅     |     ❌     |     ✅      |   ❌    |     ✅     |  ✅   |   ✅   |   ✅    |
| **Helmfile**       |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |     ❌     |  ❌   |   ❌   |   ❌    |
| **Packer**         |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |     ❌     |  ❌   |   ❌   |   ❌    |
| **Kubernetes**     |   ✅   |     ❌      |     ✅     |     ❌     |     ❌      |   ✅    |     ✅     |  ✅   |   ❌   |   ❌    |
| **Pulumi**         |   ❌   |     ❌      |     ❌     |     ❌     |     ❌      |   ✅    |     ✅     |  ✅   |   ❌   |   ❌    |
| **CloudFormation** |   ❌   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |     ✅     |  ✅   |   ❌   |   ❌    |
| **Ansible**        |   ❌   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |     ✅     |  ✅   |   ❌   |   ❌    |
| **Terragrunt**     |   ❌   |    N/A     |     ✅     |     ❌     |     ❌      |   ❌    |     ✅     |  ✅   |   ✅   |   ✅    |

### Configuration & Composition

| Feature                  |    Atmos     | Terragrunt | Terramate | TF Stacks | Terraspace |   Pulumi   |
|--------------------------|:------------:|:----------:|:---------:|:---------:|:----------:|:----------:|
| **Config Language**      |     YAML     |    HCL     |    HCL    |    HCL    |    Ruby    | Multi-lang |
| **Inheritance**          | ✅ Deep merge | ✅ Include  | ✅ Import  |     ❌     | ✅ Layering |     ❌      |
| **Multiple Inheritance** |      ✅       |     ✅      |     ❌     |     ❌     |     ❌      |     ❌      |
| **Mixins**               |      ✅       |     ❌      |     ❌     |     ❌     |     ❌      |     ❌      |
| **Template Functions**   |    ✅ 100+    | ✅ Limited  |  ✅ 150+   |     ❌     |   ✅ ERB    |    N/A     |
| **YAML Functions**       |    ✅ 17+     |     ❌      |     ❌     |     ❌     |     ❌      |     ❌      |
| **Abstract Components**  |      ✅       |     ❌      |     ❌     |     ✅     |     ❌      |     ✅      |

### State & Backend

| Feature                  | Atmos | Terragrunt | Terramate | TF Stacks | Terraspace | Pulumi | Spacelift | env0 | Scalr | Digger |
|--------------------------|:-----:|:----------:|:---------:|:---------:|:----------:|:------:|:---------:|:----:|:-----:|:------:|
| **Backend Generation**   |   ✅   |     ✅      |     ✅     |    N/A    |     ✅      |  N/A   |     ✅     |  ✅   |   ✅   |   ❌    |
| **Backend Provisioning** |   ✅   |     ✅      |     ❌     |    N/A    |     ✅      |  N/A   |     ❌     |  ❌   |   ❌   |   ❌    |
| **Remote State Access**  |   ✅   |     ✅      |     ❌     |     ✅     |     ✅      |   ✅    |     ✅     |  ✅   |   ✅   |   ✅    |
| **Backend Migration**    |   ❌   |     ✅      |     ❌     |    N/A    |     ❌      |   ❌    |     ❌     |  ❌   |   ❌   |   ❌    |
| **Managed State**        |   ❌   |     ❌      |     ❌     |     ✅     |     ❌      |   ✅    |     ✅     |  ✅   |   ✅   |   ✅    |

### Hooks & Lifecycle

| Feature             | Atmos | Terragrunt | Terramate | TF Stacks | Terraspace |
|---------------------|:-----:|:----------:|:---------:|:---------:|:----------:|
| **Before Hooks**    |   ❌   |     ✅      |     ❌     |     ❌     |     ✅      |
| **After Hooks**     |   ✅   |     ✅      |     ❌     |     ❌     |     ✅      |
| **Error Hooks**     |   ❌   |     ✅      |     ❌     |     ❌     |     ❌      |
| **Store to Remote** |   ✅   |     ❌      |     ❌     |    N/A    |     ✅      |

### Vendoring & Module Management

| Feature                 | Atmos | Terragrunt | Terramate | TF Stacks | Terraspace |
|-------------------------|:-----:|:----------:|:---------:|:---------:|:----------:|
| **Component Vendoring** |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |
| **Git Sources**         |   ✅   |     ✅      |     ❌     |     ❌     |     ✅      |
| **OCI Registry**        |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |
| **S3/GCS Sources**      |   ✅   |     ✅      |     ❌     |     ❌     |     ✅      |
| **Module Caching**      |   ❌   |     ✅      |     ❌     |     ❌     |     ✅      |
| **Module Catalog**      |   ❌   |     ✅      |     ✅     |     ❌     |     ❌      |
| **Scaffolding**         |   ❌   |     ✅      |     ✅     |     ❌     |     ✅      |

### Validation & Policy

| Feature               | Atmos | Terragrunt | Terramate | TF Stacks |  Spacelift  | env0 | Scalr | Digger |
|-----------------------|:-----:|:----------:|:---------:|:---------:|:-----------:|:----:|:-----:|:------:|
| **OPA Policies**      |   ✅   |     ❌      |     ❌     |     ❌     | ✅ (9 types) |  ✅   |   ✅   |   ✅    |
| **JSON Schema**       |   ✅   |     ❌      |     ❌     |     ❌     |      ❌      |  ❌   |   ❌   |   ❌    |
| **Built-in Policies** |   ❌   |     ❌      | ✅ (500+)  |     ❌     |      ❌      |  ❌   |   ❌   |   ❌    |
| **Checkov**           |   ❌   |     ❌      |     ❌     |     ❌     |      ❌      |  ❌   |   ✅   |   ✅    |
| **Infracost**         |   ❌   |     ❌      |     ❌     |     ❌     |      ✅      |  ✅   |   ❌   |   ✅    |

### Authentication

| Feature                 | Atmos | Terragrunt | Terramate | TF Stacks | Spacelift | env0 | Scalr |
|-------------------------|:-----:|:----------:|:---------:|:---------:|:---------:|:----:|:-----:|
| **AWS IAM/SSO**         |   ✅   |     ✅      |     ❌     |     ✅     |     ✅     |  ✅   |   ✅   |
| **Azure**               |   ✅   |     ❌      |     ❌     |     ✅     |     ✅     |  ✅   |   ✅   |
| **GCP**                 |   ✅   |     ❌      |     ❌     |     ✅     |     ✅     |  ✅   |   ✅   |
| **OIDC**                |   ✅   |     ✅      |     ❌     |     ✅     |     ✅     |  ✅   |   ✅   |
| **Auth CLI**            |   ✅   |     ❌      |     ❌     |     ❌     |     ❌     |  ❌   |   ❌   |
| **Multiple Identities** |   ✅   |     ❌      |     ❌     |     ✅     |     ❌     |  ❌   |   ❌   |

### Change Detection & Drift

| Feature                 | Atmos | Terragrunt | Terramate | TF Stacks | Spacelift | env0 | Scalr | Digger |
|-------------------------|:-----:|:----------:|:---------:|:---------:|:---------:|:----:|:-----:|:------:|
| **Git-based Detection** |   ✅   |     ✅      |     ✅     |     ✅     |     ✅     |  ✅   |   ❌   |   ✅    |
| **Affected Components** |   ✅   |     ✅      |     ✅     |     ✅     |     ✅     |  ✅   |   ❌   |   ✅    |
| **Drift Detection**     |   ✅   |     ❌      |     ✅     |     ✅     |     ✅     |  ✅   |   ✅   |   ✅    |
| **Drift Remediation**   |   ✅   |     ❌      |     ✅     |     ✅     |     ✅     |  ✅   |   ✅   |   ❌    |
| **Drift Dashboard**     |   ❌   |     ❌      |     ✅     |     ✅     |     ✅     |  ✅   |   ✅   |   ❌    |

### CI/CD Integration

| Feature            | Atmos | Terragrunt | Terramate | TF Stacks | Spacelift | env0 | Scalr | Digger |
|--------------------|:-----:|:----------:|:---------:|:---------:|:---------:|:----:|:-----:|:------:|
| **GitHub Actions** |   ✅   |     ❌      |     ✅     |     ✅     |     ✅     |  ✅   |   ✅   |   ✅    |
| **GitLab CI**      |   ❌   |     ❌      |     ✅     |     ✅     |     ✅     |  ✅   |   ✅   |   ✅    |
| **Bitbucket**      |   ❌   |     ❌      |     ✅     |     ✅     |     ✅     |  ✅   |   ✅   |   ❌    |
| **Azure DevOps**   |   ❌   |     ❌      |     ❌     |     ❌     |     ✅     |  ✅   |   ✅   |   ✅    |
| **Atlantis**       |   ✅   |     ❌      |     ⏳     |     ❌     |     ❌     |  ❌   |   ❌   |   ❌    |
| **Spacelift**      |   ✅   |     ❌      |     ❌     |     ❌     |    N/A    |  ❌   |   ❌   |   ❌    |

### Developer Experience

| Feature                | Atmos | Terragrunt | Terramate | TF Stacks | Terraspace | Pulumi |
|------------------------|:-----:|:----------:|:---------:|:---------:|:----------:|:------:|
| **Terminal UI**        |   ✅   |     ✅      |     ❌     |     ❌     |     ❌      |   ❌    |
| **Shell Completion**   |   ✅   |     ✅      |     ✅     |    N/A    |     ✅      |   ✅    |
| **IDE/LSP Support**    |   ❌   |     ❌      |     ✅     |     ✅     |     ❌      |   ✅    |
| **Themes**             |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |
| **Secret Masking**     |   ✅   |     ❌      |     ❌     |     ✅     |     ❌      |   ✅    |
| **Markdown Rendering** |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |

### Cloud Platform Features

| Feature                 | Atmos Pro | Terramate Cloud | HCP TF Stacks | Spacelift | env0 | Scalr |
|-------------------------|:---------:|:---------------:|:-------------:|:---------:|:----:|:-----:|
| **Dashboard**           |     ✅     |        ✅        |       ✅       |     ✅     |  ✅   |   ✅   |
| **Stack Inventory**     |     ❌     |        ✅        |       ✅       |     ✅     |  ✅   |   ✅   |
| **PR Previews**         |     ✅     |        ✅        |       ❌       |     ✅     |  ✅   |   ✅   |
| **Deployment Tracking** |     ❌     |        ✅        |       ✅       |     ✅     |  ✅   |   ✅   |
| **Audit Trails**        |     ❌     |        ✅        |       ✅       |     ✅     |  ✅   |   ✅   |
| **Cost Management**     |     ❌     |        ❌        |       ❌       |     ✅     |  ✅   |   ❌   |
| **Slack Integration**   |     ❌     |        ✅        |       ❌       |     ✅     |  ✅   |   ✅   |
| **Locking**             |     ✅     |        ❌        |       ✅       |     ✅     |  ✅   |   ✅   |
| **AI Features**         |     ❌     |        ❌        |       ❌       |     ✅     |  ✅   |   ✅   |

### Advanced Features

| Feature                  | Atmos | Terragrunt | Terramate | TF Stacks | Terraspace | Pulumi |
|--------------------------|:-----:|:----------:|:---------:|:---------:|:----------:|:------:|
| **Toolchain Management** |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |
| **Devcontainers**        |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |
| **Service Catalogs**     |   ✅   |     ❌      |     ✅     |     ❌     |     ❌      |   ❌    |
| **Component Updater**    |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |
| **Provenance Tracking**  |   ✅   |     ❌      |     ❌     |     ❌     |     ❌      |   ❌    |
| **Testing Framework**    |   ❌   |     ❌      |     ❌     |     ❌     |     ✅      |   ✅    |
| **Custom IaC Engines**   |   ❌   |     ✅      |     ❌     |     ❌     |     ❌      |   ❌    |

---

## Summary

### Atmos Unique Strengths

1. **Multi-tool support**: Only CLI tool supporting Terraform + Helmfile + Packer
2. **YAML-based configuration**: Human-readable with deep merge inheritance
3. **YAML Functions**: Unique `!terraform.output`, `!store`, `!exec` functions
4. **Multi-cloud authentication CLI**: Comprehensive auth for AWS, Azure, GCP
5. **Component vendoring**: Full vendoring with OCI, Git, S3/GCS, HTTP support
6. **OPA + JSON Schema validation**: Native policy-as-code
7. **Toolchain management**: Built-in tool version management (like mise)
8. **Terminal UI**: Interactive stack/component navigation
9. **Secret masking**: Gitleaks integration (120+ patterns)
10. **Provenance tracking**: Configuration source tracking

### Atmos Feature Gaps

| Gap                    | Available In                      |
|------------------------|-----------------------------------|
| Before hooks           | Terragrunt, Terraspace            |
| Error hooks            | Terragrunt                        |
| IDE/LSP support        | Terramate, Pulumi                 |
| Scaffolding command    | Terragrunt, Terramate, Terraspace |
| Provider generation    | Terragrunt, Terramate             |
| DAG visualization      | Terragrunt                        |
| Mock outputs           | Terragrunt                        |
| Backend migration      | Terragrunt                        |
| Module change tracking | Terramate                         |
| GitLab/Bitbucket CI    | Terramate, Spacelift, env0, Scalr |
| Testing framework      | Terraspace, Pulumi                |
| Cost management        | Spacelift, env0                   |
| AI features            | Spacelift, env0, Scalr            |

### Tool Selection Guide

| Use Case                                              | Recommended Tools          |
|-------------------------------------------------------|----------------------------|
| **Multi-tool orchestration (TF + Helmfile + Packer)** | Atmos                      |
| **HCL-native Terraform wrapper**                      | Terragrunt                 |
| **Cloud platform with governance**                    | Terramate, Spacelift, env0 |
| **HashiCorp-native multi-deployment (HCP only)**      | TF Stacks                  |
| **Programming language IaC**                          | Pulumi                     |
| **Enterprise compliance (FedRAMP)**                   | Spacelift                  |
| **FinOps/Cost management**                            | env0, Spacelift            |
| **CI-native (no extra infrastructure)**               | Digger                     |
| **Unlimited concurrent runs**                         | Scalr                      |
| **Ruby-based framework**                              | Terraspace                 |

---

## Feature Implementation Priorities

Based on this analysis, potential features to consider implementing in Atmos:

### High Priority

1. **Before hooks** - Pre-command execution (Terragrunt, Terraspace have)
2. **Error hooks** - Error-specific handlers (Terragrunt has)
3. **Scaffolding command** - Boilerplate code generation (Terragrunt, Terramate, Terraspace have)
4. **DAG visualization** - Dependency graph output in DOT format (Terragrunt has)

### Medium Priority

5. **IDE/LSP support** - Editor integration (Terramate, Pulumi have)
6. **Provider generation** - Dynamic provider blocks (Terragrunt, Terramate have)
7. **Mock outputs** - Placeholder values for testing (Terragrunt has)
8. **GitLab CI templates** - Expand CI/CD support (Terramate, all SaaS platforms have)
9. **Bitbucket Pipeline templates** - Expand CI/CD support
10. **Testing framework** - Infrastructure testing (Terraspace, Pulumi have)

### Lower Priority

11. **Backend migration** - State transfer between components (Terragrunt has)
12. **Module change tracking** - Track changes in referenced modules (Terramate has)
13. **HCL generation** - Generate HCL files from YAML (Terragrunt, Terramate have)
14. **Cost estimation integration** - Infracost integration (Spacelift, env0, Digger have)
15. **AI features** - Plan summaries, remediation (Spacelift, env0, Scalr have)
