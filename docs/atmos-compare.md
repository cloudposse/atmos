# Atmos Feature Comparison

This document provides a comprehensive comparison of Atmos with similar infrastructure orchestration tools: Terragrunt
and Terramate.

---

## Table of Contents

1. [Atmos Features](#atmos-features)
2. [Terragrunt Features](#terragrunt-features)
3. [Terramate Features](#terramate-features)
4. [Feature Comparison Matrix](#feature-comparison-matrix)
5. [Summary](#summary)

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

## Terragrunt Features

### Core Architecture

| Feature                 | Description                                                                      |
|-------------------------|----------------------------------------------------------------------------------|
| **Units**               | Individual directories containing `terragrunt.hcl` as smallest deployable entity |
| **Stacks**              | Collections of units managed together as environments                            |
| **Configuration Files** | `terragrunt.hcl`, `terragrunt.hcl.json`, `terragrunt.stack.hcl`                  |
| **Language**            | HCL configuration                                                                |
| **IaC Support**         | Terraform, OpenTofu                                                              |

### Configuration & Composition (DRY)

| Feature                         | Description                                                                  |
|---------------------------------|------------------------------------------------------------------------------|
| **Include Blocks**              | Configuration inheritance from parent folders via `find_in_parent_folders()` |
| **Multiple Includes**           | Multiple parent configurations with `expose = true`                          |
| **Locals**                      | Reusable local variables                                                     |
| **Dynamic Configuration**       | `read_terragrunt_config()` for environment-specific files                    |
| **Extra Arguments**             | Automatic CLI flag and env var injection                                     |
| **Required/Optional Var Files** | `required_var_files`, `optional_var_files`                                   |

### Dependency Management

| Feature                      | Description                                     |
|------------------------------|-------------------------------------------------|
| **Dependency Blocks**        | Express relationships between units             |
| **Mock Outputs**             | Placeholder values for testing/planning         |
| **DAG Ordering**             | Automatic execution order based on dependencies |
| **Dependency Visualization** | `dag graph` outputs DOT format graphs           |

### State & Backend Management

| Feature                  | Description                                            |
|--------------------------|--------------------------------------------------------|
| **Remote State Backend** | Automatic backend configuration generation             |
| **Backend Bootstrap**    | `backend bootstrap` initializes backend infrastructure |
| **Backend Migration**    | `backend migrate` transfers state between units        |
| **Backend Deletion**     | `backend delete` removes backend state                 |
| **Lock Files**           | `.terraform.lock.hcl` generation                       |
| **Smart Caching**        | `.terragrunt-cache` with `--source-update` control     |

### Hooks & Custom Actions

| Feature                   | Description                                            |
|---------------------------|--------------------------------------------------------|
| **Before Hooks**          | Execute before Terraform commands                      |
| **After Hooks**           | Execute after commands (even on errors)                |
| **Error Hooks**           | Trigger on specific failures                           |
| **Context Variables**     | `TG_CTX_TF_PATH`, `TG_CTX_COMMAND`, `TG_CTX_HOOK_NAME` |
| **Conditional Execution** | `run_on_error` controls                                |

### Execution & Run Queue

| Feature                  | Description                                                        |
|--------------------------|--------------------------------------------------------------------|
| **Run All**              | `run --all` across entire stacks                                   |
| **Queue Filtering**      | Include/exclude with positive/negative filters                     |
| **Parallelism Control**  | Concurrent runs with limits                                        |
| **Queue Flags**          | `--queue-ignore-dag-order`, `--queue-ignore-errors`, `--fail-fast` |
| **Non-interactive Mode** | `--non-interactive` for automation                                 |

### Remote Module Management

| Feature                    | Description                              |
|----------------------------|------------------------------------------|
| **Go-Getter Support**      | Git, local paths, HTTP sources           |
| **Git Version References** | `ref` parameter for version pinning      |
| **Local Development**      | `--source` parameter for local iteration |
| **Module Caching**         | Automatic temporary directory caching    |

### Provider & Authentication

| Feature                   | Description                                   |
|---------------------------|-----------------------------------------------|
| **IAM Role Assumption**   | `--iam-assume-role` flag                      |
| **OIDC Web Identity**     | `--iam-assume-role-web-identity-token`        |
| **Auth Provider Command** | `--auth-provider-cmd` for dynamic credentials |
| **SSH Authentication**    | Git authentication for private repos          |
| **Provider Cache Server** | Caching for concurrent runs                   |

### Code Generation & Scaffolding

| Feature                   | Description                                      |
|---------------------------|--------------------------------------------------|
| **Generate Blocks**       | Dynamic file generation before execution         |
| **Provider Generation**   | Inject provider blocks without modifying modules |
| **Scaffold Command**      | Boilerplate-based code generation                |
| **Auto-populated Inputs** | Generated files with variable types/descriptions |

### Module Discovery & Catalog

| Feature             | Description                           |
|---------------------|---------------------------------------|
| **Catalog Command** | Interactive TUI for browsing modules  |
| **Module Search**   | Search and filter modules             |
| **Module Browser**  | View docs and generate configurations |

### CLI Commands

| Command                              | Description                                         |
|--------------------------------------|-----------------------------------------------------|
| **run**                              | Execute Terraform commands with dependency handling |
| **exec**                             | Wrap arbitrary commands                             |
| **stack run/output/clean/generate**  | Stack-level operations                              |
| **backend bootstrap/delete/migrate** | Backend management                                  |
| **find, list**                       | Discover configurations                             |
| **render**                           | Process configuration with includes merged          |
| **hcl fmt/validate**                 | Format and validate HCL                             |
| **scaffold**                         | Generate boilerplate code                           |
| **catalog**                          | Browse module catalog                               |
| **dag graph**                        | Visualize dependencies                              |

### Advanced Features

| Feature                       | Description                        |
|-------------------------------|------------------------------------|
| **Custom IaC Engines**        | RPC-based plugin system            |
| **Content Addressable Store** | Infrastructure artifact management |
| **Run Reports**               | Reporting and tracking             |
| **Feature Flags**             | Runtime control                    |

---

## Terramate Features

### Core Architecture

| Feature                 | Description                                                                                                   |
|-------------------------|---------------------------------------------------------------------------------------------------------------|
| **Configuration Files** | HCL-based (`terramate.tm.hcl`)                                                                                |
| **Blocks**              | `terramate`, `stack`, `generate_hcl`, `generate_file`, `globals`, `lets`, `map`, `script`, `assert`, `import` |
| **IaC Support**         | Terraform, OpenTofu, Terragrunt, Kubernetes                                                                   |
| **IDE Support**         | VSCode, JetBrains, Vim with LSP                                                                               |

### Stack Management

| Feature                   | Description                                      |
|---------------------------|--------------------------------------------------|
| **Stack Operations**      | Create, nest, clone, manage, delete              |
| **Stack Metadata**        | Path, name, description                          |
| **Multiple Environments** | Workspaces, directories, TFVars, partial backend |
| **Output Sharing**        | Between environments                             |

### Code Generation

| Feature               | Description                              |
|-----------------------|------------------------------------------|
| **Generate HCL**      | Programmatic backend/provider generation |
| **Generate Files**    | HCL, JSON, YAML generation               |
| **Ad-hoc Generation** | Dynamic file creation                    |
| **Template-based**    | Infrastructure instantiation             |

### Orchestration & Execution

| Feature                       | Description                |
|-------------------------------|----------------------------|
| **Graph-based Orchestration** | Dependency-aware execution |
| **Parallel Execution**        | Unlimited concurrency      |
| **Tag-based Filtering**       | Stack selection by tags    |
| **Custom Scripts**            | Multi-command combinations |

### Change Detection

| Feature                            | Description                       |
|------------------------------------|-----------------------------------|
| **Git-based Detection**            | Identifies changes in modules     |
| **Terragrunt Dependency Tracking** | Monitors dependency modifications |
| **Terraform State Analysis**       | State-based change detection      |
| **Partial Evaluation**             | Selective change detection        |

### CI/CD Integration

| Platform                | Support                                                          |
|-------------------------|------------------------------------------------------------------|
| **GitHub Actions**      | Preview, deployment, drift-check workflows                       |
| **GitLab CI/CD**        | Preview, deployment, drift-check workflows                       |
| **Bitbucket Pipelines** | Preview, deployment, drift-check workflows                       |
| **Planned**             | Atlantis, Digger, Azure DevOps, AWS CodeBuild, CircleCI, Jenkins |

### Terramate Cloud Platform

| Feature                 | Description                                    |
|-------------------------|------------------------------------------------|
| **Dashboard**           | Infrastructure state visualization             |
| **Stack Inventory**     | Status tracking and listing                    |
| **Collaboration**       | PR previews, deployment tracking               |
| **Drift Management**    | Automated detection and reconciliation         |
| **Governance**          | 500+ pre-configured policies, CIS Benchmarks   |
| **Incident Management** | Failed deployment detection, Slack integration |

### Terramate Catalyst

| Feature             | Description                              |
|---------------------|------------------------------------------|
| **Components**      | Reusable infrastructure templates        |
| **Bundles**         | Collections of components                |
| **Scaffolding**     | Self-service infrastructure provisioning |
| **Remote Catalogs** | External component sources               |

### CLI Commands

| Command                         | Description                    |
|---------------------------------|--------------------------------|
| **create/list/manage/delete**   | Stack operations               |
| **run/trigger**                 | Orchestration commands         |
| **script run/info/list/tree**   | Script management              |
| **fmt/generate**                | Code formatting and generation |
| **scaffold**                    | Component scaffolding          |
| **cloud login/info/drift show** | Cloud platform integration     |
| **eval/partial-eval**           | Configuration inspection       |

### Built-in Functions

| Category                 | Count                            |
|--------------------------|----------------------------------|
| **String Functions**     | format, join, split, trim, regex |
| **Collection Functions** | merge, flatten, sort, distinct   |
| **Encoding**             | JSON, YAML, base64, TOML         |
| **Crypto**               | Hashing, UUID generation         |
| **Date/Time**            | Time operations                  |
| **Total**                | 150+ functions                   |

---

## Feature Comparison Matrix

### Core Architecture

| Feature                            |      Atmos       |  Terragrunt  | Terramate |
|------------------------------------|:----------------:|:------------:|:---------:|
| **Terraform Support**              |        ✅         |      ✅       |     ✅     |
| **OpenTofu Support**               |        ✅         |      ✅       |     ✅     |
| **Helmfile Support**               |        ✅         |      ❌       |     ❌     |
| **Packer Support**                 |        ✅         |      ❌       |     ❌     |
| **Kubernetes Support**             | ✅ (via Helmfile) |      ❌       |     ✅     |
| **Native Terraform (no wrappers)** |        ✅         | ❌ (wraps TF) |     ✅     |
| **Configuration Language**         |       YAML       |     HCL      |    HCL    |

### Configuration & Composition

| Feature                    |       Atmos       |    Terragrunt    |     Terramate     |
|----------------------------|:-----------------:|:----------------:|:-----------------:|
| **Inheritance/Imports**    |   ✅ Deep merge    | ✅ Include blocks |  ✅ Import blocks  |
| **Multiple Inheritance**   |         ✅         |        ✅         |         ❌         |
| **Mixins**                 |         ✅         |        ❌         |         ❌         |
| **Locals**                 |         ✅         |        ✅         | ✅ (globals, lets) |
| **Template Functions**     | ✅ 100+ (Gomplate) |    ✅ Limited     |      ✅ 150+       |
| **YAML Functions**         |  ✅ 17+ functions  |        ❌         |         ❌         |
| **Dynamic Variable Files** |         ✅         |        ✅         |         ✅         |
| **Abstract Components**    |         ✅         |        ❌         |         ❌         |
| **Component Overrides**    |         ✅         |        ❌         |         ❌         |

### State & Backend Management

| Feature                  |      Atmos       |     Terragrunt      |     Terramate      |
|--------------------------|:----------------:|:-------------------:|:------------------:|
| **Backend Generation**   |        ✅         |          ✅          |         ✅          |
| **Backend Provisioning** | ✅ (S3/DynamoDB)  |    ✅ (bootstrap)    |         ❌          |
| **Remote State Access**  | ✅ YAML functions | ✅ dependency blocks |         ❌          |
| **State Sharing**        |        ✅         |          ✅          | ✅ (output sharing) |
| **Backend Migration**    |        ❌         |          ✅          |         ❌          |

### Dependency Management

| Feature                      | Atmos |   Terragrunt   | Terramate |
|------------------------------|:-----:|:--------------:|:---------:|
| **Dependency Declaration**   |   ✅   |       ✅        |     ✅     |
| **DAG-based Execution**      |   ✅   |       ✅        |     ✅     |
| **Dependency Visualization** |   ❌   | ✅ (DOT format) |     ❌     |
| **Mock Outputs**             |   ❌   |       ✅        |     ❌     |

### Vendoring & Module Management

| Feature                    | Atmos |  Terragrunt   |  Terramate   |
|----------------------------|:-----:|:-------------:|:------------:|
| **Component Vendoring**    |   ✅   |       ❌       |      ❌       |
| **Git Sources**            |   ✅   | ✅ (go-getter) |      ❌       |
| **OCI Registry**           |   ✅   |       ❌       |      ❌       |
| **S3/GCS Sources**         |   ✅   |       ✅       |      ❌       |
| **HTTP Sources**           |   ✅   |       ✅       |      ❌       |
| **Version Pinning**        |   ✅   | ✅ (git refs)  |      ❌       |
| **Module Caching**         |   ❌   |       ✅       |      ❌       |
| **Module Catalog/Browser** |   ❌   |    ✅ (TUI)    | ✅ (Catalyst) |

### Hooks & Lifecycle

| Feature                    | Atmos | Terragrunt | Terramate |
|----------------------------|:-----:|:----------:|:---------:|
| **Before Hooks**           |   ❌   |     ✅      |     ❌     |
| **After Hooks**            |   ✅   |     ✅      |     ❌     |
| **Error Hooks**            |   ❌   |     ✅      |     ❌     |
| **Store Output to Remote** |   ✅   |     ❌      |     ❌     |

### Workflow & Orchestration

| Feature                  | Atmos | Terragrunt |  Terramate  |
|--------------------------|:-----:|:----------:|:-----------:|
| **Workflow Definitions** |   ✅   |     ❌      | ✅ (scripts) |
| **Multi-step Workflows** |   ✅   |     ❌      |      ✅      |
| **Resume from Step**     |   ✅   |     ❌      |      ❌      |
| **Custom Commands**      |   ✅   |     ❌      |      ✅      |
| **Parallel Execution**   |   ✅   |     ✅      |      ✅      |
| **Run-all Operations**   |   ✅   |     ✅      |      ✅      |

### Validation & Governance

| Feature                     | Atmos | Terragrunt |      Terramate      |
|-----------------------------|:-----:|:----------:|:-------------------:|
| **OPA Policy Validation**   |   ✅   |     ❌      | ❌ (via third-party) |
| **JSON Schema Validation**  |   ✅   |     ❌      |          ❌          |
| **Built-in Policies**       |   ✅   |     ❌      |  ✅ (500+ in Cloud)  |
| **EditorConfig Validation** |   ✅   |     ❌      |          ❌          |
| **Assertion Blocks**        |   ❌   |     ❌      |          ✅          |

### Authentication

| Feature                 | Atmos | Terragrunt | Terramate |
|-------------------------|:-----:|:----------:|:---------:|
| **AWS IAM/SSO**         |   ✅   |     ✅      |     ❌     |
| **Azure**               |   ✅   |     ❌      |     ❌     |
| **GCP**                 |   ✅   |     ❌      |     ❌     |
| **OIDC**                |   ✅   |     ✅      |     ❌     |
| **Auth Shell/Exec**     |   ✅   |     ❌      |     ❌     |
| **ECR Login**           |   ✅   |     ❌      |     ❌     |
| **Console Access**      |   ✅   |     ❌      |     ❌     |
| **Multiple Identities** |   ✅   |     ❌      |     ❌     |

### Change Detection & Affected

| Feature                        | Atmos |    Terragrunt     | Terramate |
|--------------------------------|:-----:|:-----------------:|:---------:|
| **Git-based Change Detection** |   ✅   | ✅ (queue-include) |     ✅     |
| **Affected Components**        |   ✅   |         ✅         |     ✅     |
| **Dependent Detection**        |   ✅   |         ❌         |     ❌     |
| **Module Change Tracking**     |   ❌   |         ❌         |     ✅     |
| **State-based Detection**      |   ❌   |         ❌         |     ✅     |

### Drift Management

| Feature                 |       Atmos       | Terragrunt | Terramate |
|-------------------------|:-----------------:|:----------:|:---------:|
| **Drift Detection**     | ✅ (GitHub Action) |     ❌      |     ✅     |
| **Drift Remediation**   | ✅ (GitHub Action) |     ❌      |     ✅     |
| **Scheduled Detection** |         ✅         |     ❌      | ✅ (Cloud) |
| **Drift Dashboard**     |         ❌         |     ❌      | ✅ (Cloud) |

### CI/CD Integration

| Feature                   |    Atmos     |  Terragrunt   |  Terramate   |
|---------------------------|:------------:|:-------------:|:------------:|
| **GitHub Actions**        | ✅ (official) | ❌ (community) | ✅ (official) |
| **GitLab CI**             |      ❌       |       ❌       |      ✅       |
| **Bitbucket Pipelines**   |      ❌       |       ❌       |      ✅       |
| **Atlantis Integration**  |      ✅       |       ❌       | ⏳ (planned)  |
| **Spacelift Integration** |      ✅       |       ❌       |      ❌       |

### Developer Experience

| Feature                |       Atmos       | Terragrunt  | Terramate |
|------------------------|:-----------------:|:-----------:|:---------:|
| **Terminal UI**        |         ✅         | ✅ (catalog) |     ❌     |
| **Shell Completion**   |   ✅ (4 shells)    |      ✅      |     ✅     |
| **Themes**             |         ✅         |      ❌      |     ❌     |
| **Secret Masking**     | ✅ (120+ patterns) |      ❌      |     ❌     |
| **Markdown Rendering** |         ✅         |      ❌      |     ❌     |
| **IDE/LSP Support**    |         ❌         |      ❌      |     ✅     |
| **Scaffolding**        |         ❌         |      ✅      |     ✅     |

### Code Generation

| Feature                  | Atmos | Terragrunt | Terramate |
|--------------------------|:-----:|:----------:|:---------:|
| **Backend Generation**   |   ✅   |     ✅      |     ✅     |
| **Provider Generation**  |   ❌   |     ✅      |     ✅     |
| **Varfile Generation**   |   ✅   |     ✅      |     ❌     |
| **HCL Generation**       |   ❌   |     ✅      |     ✅     |
| **JSON/YAML Generation** |   ❌   |     ❌      |     ✅     |

### Cloud Platform

| Feature                 |  Atmos  | Terragrunt | Terramate |
|-------------------------|:-------:|:----------:|:---------:|
| **SaaS Dashboard**      | ✅ (Pro) |     ❌      | ✅ (Cloud) |
| **Stack Inventory**     |    ❌    |     ❌      |     ✅     |
| **PR Previews**         |    ✅    |     ❌      |     ✅     |
| **Deployment Tracking** |    ❌    |     ❌      |     ✅     |
| **Governance Policies** |    ❌    |     ❌      | ✅ (500+)  |
| **Audit Trails**        |    ❌    |     ❌      |     ✅     |
| **Slack Integration**   |    ❌    |     ❌      |     ✅     |
| **Locking**             | ✅ (Pro) |     ❌      |     ❌     |

### Advanced Features

| Feature                  |       Atmos       |   Terragrunt    |  Terramate   |
|--------------------------|:-----------------:|:---------------:|:------------:|
| **Toolchain Management** |   ✅ (like mise)   |        ❌        |      ❌       |
| **Devcontainers**        |         ✅         |        ❌        |      ❌       |
| **Service Catalogs**     |         ✅         |        ❌        | ✅ (Catalyst) |
| **Component Updater**    | ✅ (GitHub Action) |        ❌        |      ❌       |
| **Provenance Tracking**  |         ✅         |        ❌        |      ❌       |
| **Plan Diff Analysis**   |         ✅         |        ❌        |      ❌       |
| **Custom IaC Engines**   |         ❌         | ✅ (RPC plugins) |      ❌       |

---

## Summary

### Atmos Strengths

- **Multi-tool support**: Only tool supporting Terraform + Helmfile + Packer in unified CLI
- **YAML-based configuration**: Human-readable, deep merge inheritance
- **YAML Functions**: Unique `!terraform.output`, `!store`, `!exec` functions
- **Multi-cloud authentication**: Comprehensive auth for AWS, Azure, GCP with console access
- **Component vendoring**: Full vendoring with OCI, Git, S3/GCS, HTTP support
- **OPA validation**: Native policy-as-code enforcement
- **Toolchain management**: Built-in tool version management
- **Terminal UI**: Interactive stack/component navigation
- **Secret masking**: Gitleaks integration

### Atmos Gaps (vs Competitors)

- No before hooks (Terragrunt has)
- No error hooks (Terragrunt has)
- No IDE/LSP support (Terramate has)
- No scaffolding command (Terragrunt, Terramate have)
- No HCL/provider generation (Terragrunt, Terramate have)
- No dependency visualization/DAG graph (Terragrunt has)
- No mock outputs for dependencies (Terragrunt has)
- No backend migration command (Terragrunt has)
- No module change tracking (Terramate has)
- No GitLab/Bitbucket CI templates (Terramate has)
- Limited cloud platform features vs Terramate Cloud

### Terragrunt Strengths

- Mature ecosystem with extensive documentation
- HCL-native configuration
- Module caching and catalog browser
- Scaffolding with Boilerplate integration
- Custom IaC engine plugins
- Backend migration tooling
- Mock outputs for dependencies

### Terragrunt Gaps

- Terraform-only (no Helmfile, Packer, K8s)
- No built-in policy validation
- No multi-cloud authentication
- Limited CI/CD integrations
- No workflow definitions
- No cloud platform/dashboard

### Terramate Strengths

- Comprehensive cloud platform (dashboard, drift, governance)
- 500+ built-in governance policies
- State-based change detection
- Multi-CI support (GitHub, GitLab, Bitbucket)
- IDE/LSP support
- Catalyst for component catalogs
- Modern, well-documented

### Terramate Gaps

- No component vendoring
- No multi-cloud authentication CLI
- No OPA policy validation (relies on third-party)
- No Helmfile/Packer support
- No workflow resume capability
- Cloud platform required for many features

---

## Feature Implementation Priorities

Based on this analysis, potential features to consider implementing in Atmos:

### High Priority

1. **Before hooks** - Pre-command execution
2. **Error hooks** - Error-specific handlers
3. **Scaffolding command** - Boilerplate code generation
4. **Dependency visualization** - DAG graph output (DOT format)

### Medium Priority

5. **IDE/LSP support** - Editor integration
6. **Provider generation** - Dynamic provider blocks
7. **Mock outputs** - Placeholder values for testing
8. **GitLab CI templates** - Expand CI/CD support
9. **Bitbucket Pipeline templates** - Expand CI/CD support

### Lower Priority

10. **Backend migration** - State transfer between components
11. **Module change tracking** - Track changes in referenced modules
12. **HCL generation** - Generate HCL files from YAML
