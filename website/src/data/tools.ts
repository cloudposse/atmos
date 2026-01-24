export type Relationship =
  | 'supported'
  | 'wrapper'
  | 'delivery'
  | 'commands'
  | 'workflows'
  | 'ecosystem'
  | 'inspiration';

export interface FeatureComparison {
  feature: string;
  atmos: boolean;
  tool: boolean;
}

export interface Tool {
  id: string;
  name: string;
  url: string;
  description: string;
  category: string;
  relationship: Relationship;
  details: string;
  atmosComparison?: string;
  featureComparison?: FeatureComparison[];
}

// =============================================================================
// FEATURE LINKS - Maps feature names to Atmos documentation URLs
// =============================================================================

export const featureLinks: Record<string, string> = {
  // Core configuration features
  'Stack Configuration': '/stacks',
  'Component Inheritance': '/cli/configuration/stacks/inherit',
  'Stack Imports': '/cli/configuration/imports',
  'Deep Merging': '/cli/configuration/stacks/inherit',
  'Stack Integration': '/stacks',
  'Remote Root Modules': '/components/remote-state',
  'Dynamic Backend Generation': '/components/terraform/backends',
  'Dynamic Provider Generation': '/components/terraform/providers',
  'Automatic Backend Provisioning': '/components/terraform/backend-provisioning',
  'Plan Diff': '/cli/commands/terraform/plan-diff',

  // Validation & Policy features
  'OPA Validation': '/validation/validating',
  'OPA Integration': '/validation/validating',
  'JSON Schema': '/validation/json-schema',
  'Custom Rules': '/validation/validating',
  'CLI Validation': '/validation/validating',

  // Workflow & Commands features
  'Native Workflows': '/workflows',
  'Custom Commands': '/cli/configuration/commands',
  'Workflow Integration': '/workflows',

  // Vendoring features
  'Vendoring': '/vendor',
  'Git Sources': '/vendor',
  'OCI Sources': '/vendor',
  'HTTP Sources': '/vendor',
  'S3 Sources': '/vendor',
  'Excludes/Includes': '/vendor',

  // Templating features
  'Go Templates': '/cli/configuration/templates',
  'Data Sources': '/cli/configuration/templates',
  'Doc Generation': '/cli/configuration/templates',

  // Toolchain features
  'Toolchain Installation': '/cli/commands/toolchain',
  'Per-project Versions': '/cli/commands/toolchain',
  'Auto-install on Demand': '/cli/commands/toolchain',
  'Cross-platform (Win/Mac/Linux)': '/cli/commands/toolchain',
  'Shell Integration': '/cli/commands/toolchain',
  'Custom Command Integration': '/cli/configuration/commands',
  'Concurrent Versions': '/cli/commands/toolchain',
  'List Installed Tools': '/cli/commands/toolchain',
  'Search for Tools': '/cli/commands/toolchain',
  'Custom Registries': '/cli/commands/toolchain',

  // Dev environment features
  'Dev Environments': '/cli/commands/devcontainer',

  // Auth features
  'Authentication (SSO/OIDC)': '/cli/commands/auth',
  'AWS SSO': '/cli/commands/auth',
  'Role Switching': '/cli/commands/auth',
  'Credential Caching': '/cli/commands/auth',
  'Multiple Config Profiles': '/cli/commands/auth',
  'Shell Execution': '/cli/commands/auth',
  'Secrets Masking': '/cli/commands/auth',
  'Multi-cloud': '/cli/commands/auth',
  'K8s Auth (EKS)': '/cli/commands/auth',
  'ECR Authentication': '/integrations/github-actions/setup-atmos',
  'EKS Authentication': '/cli/commands/auth',

  // CI/CD & Delivery features
  'GitHub Actions Native': '/integrations/github-actions',
  'PR Automation': '/integrations/github-actions',
  'Affected Stacks': '/cli/commands/describe/affected',
  'Change Detection': '/cli/commands/describe/affected',

  // Code generation
  'Code Generation': '/cli/commands/terraform/generate/files',

  // Introspection features
  'Introspection': '/cli/commands/describe',
  'Describe Affected': '/cli/commands/describe/affected',
  'Describe Dependents': '/cli/commands/describe/dependents',
  'Describe Component': '/cli/commands/describe/component',
  'Describe Stacks': '/cli/commands/describe/stacks',
  'List Components': '/cli/commands/list/components',
  'List Stacks': '/cli/commands/list/stacks',

  // Stack Dependencies features
  'Stack Dependencies': '/components/terraform/state',
  'Depends On': '/components/terraform/state',
  'Execution Order': '/components/terraform/state',

  // Hooks & Lifecycle features
  'Hooks': '/cli/configuration/hooks',
  'Pre-hooks': '/cli/configuration/hooks',
  'Post-hooks': '/cli/configuration/hooks',

  // Store Integration features
  'Store Integration': '/cli/configuration/stores',
  'Secrets Management': '/cli/configuration/stores',

  // Terminal UI features
  'Terminal UI': '/cli/commands',
  'Interactive Mode': '/cli/commands',
};

// =============================================================================
// CAPABILITY INFO - Detailed information about each capability
// =============================================================================

export interface CapabilityInfo {
  id: string;
  title: string;
  description: string;
  whyItMatters: string;
  atmosSupport: string;
  docsLink: string;
}

export const capabilityInfo: Record<string, CapabilityInfo> = {
  supported: {
    id: 'supported',
    title: 'Infrastructure as Code',
    description:
      'Infrastructure as Code (IaC) is the practice of managing and provisioning infrastructure through machine-readable configuration files rather than manual processes or interactive tools.',
    whyItMatters:
      'IaC enables version control, repeatability, and automation of your infrastructure. Teams can review changes before deployment, roll back to previous states, and ensure consistency across environments. Without IaC, infrastructure becomes a snowflake—impossible to reproduce and prone to drift.',
    atmosSupport:
      'Atmos **orchestrates Terraform, OpenTofu, Helmfile, and Packer natively**. Define components once, configure them per-environment through stacks, and deploy with confidence. Atmos generates `.tfvars` files automatically and manages state across accounts.',
    docsLink: '/components',
  },
  wrappers: {
    id: 'wrappers',
    title: 'Configuration Management',
    description:
      'Configuration management is the practice of systematically handling changes to infrastructure settings, ensuring consistency and traceability across environments like dev, staging, and production.',
    whyItMatters:
      'As infrastructure grows, managing configurations becomes exponentially complex. Without proper configuration management, teams face environment drift, duplicated code, and the nightmare of "it works on my machine." You need a way to share common settings while allowing environment-specific overrides.',
    atmosSupport:
      'Atmos provides **stack-based configuration** with imports, inheritance, and deep merging. Define base configurations once, import them into environment-specific stacks, and override only what differs. No code duplication, no templating gymnastics—just clean YAML composition.',
    docsLink: '/stacks',
  },
  auth: {
    id: 'auth',
    title: 'Cloud Authentication',
    description:
      'Cloud authentication manages identity and access to cloud providers like AWS, Azure, and GCP. It includes SSO integration, role assumption, credential caching, and secure session management.',
    whyItMatters:
      'Infrastructure teams constantly switch between accounts and roles. Manual credential management is tedious, error-prone, and a security risk. You need seamless authentication that works with your identity provider and doesn\'t interrupt your workflow.',
    atmosSupport:
      'Atmos provides **native cloud authentication** with `atmos auth`. Login via AWS SSO, assume roles across accounts, cache credentials securely, and execute commands in authenticated shells. Multi-cloud support for AWS, with Azure and GCP coming soon.',
    docsLink: '/cli/commands/auth',
  },
  workflows: {
    id: 'workflows',
    title: 'Task Automation',
    description:
      'Task automation orchestrates sequences of commands, scripts, and operations into repeatable workflows. It replaces ad-hoc scripts with structured, documented procedures.',
    whyItMatters:
      'Infrastructure operations involve complex multi-step procedures: deploying in the right order, running migrations, validating state. Manual runbooks are error-prone and hard to maintain. You need automation that\'s readable, testable, and integrated with your infrastructure.',
    atmosSupport:
      'Atmos **workflows** define tasks in YAML with typed inputs and automatic retry logic. Workflows integrate directly with stacks—run the same workflow across environments with different configurations. Resume from any step on failure. No Makefiles, no shell script spaghetti.',
    docsLink: '/workflows',
  },
  commands: {
    id: 'commands',
    title: 'Custom CLI Commands',
    description:
      'Custom commands extend your CLI with project-specific operations. They wrap existing tools, scripts, and APIs into a unified interface that your team can discover and use.',
    whyItMatters:
      'Every team has unique operational needs—custom linting, deployment helpers, debugging tools. Scattering these across scripts and READMEs leads to tribal knowledge. You need a way to integrate custom tooling into your standard workflow.',
    atmosSupport:
      'Atmos **custom commands** let you define CLI commands in `atmos.yaml`. Specify arguments, flags, and help text declaratively. Commands have full access to stack context—use them to integrate kubectl, aws-cli, or any tool into your infrastructure workflow.',
    docsLink: '/cli/configuration/commands',
  },
  toolchain: {
    id: 'toolchain',
    title: 'Toolchain Management',
    description:
      'Toolchain management handles installation, versioning, and updates of CLI tools like Terraform, kubectl, and helm. It ensures everyone uses the same tool versions.',
    whyItMatters:
      'Version mismatches cause "works on my machine" problems and subtle bugs. Installing tools manually is tedious and error-prone. You need automatic tool management that pins versions per-project and installs on demand.',
    atmosSupport:
      'Atmos **toolchain** installs tools automatically on first use. Pin Terraform versions per-stack, run multiple versions side-by-side, and let Atmos handle cross-platform binary management. No brew, no asdf, no version conflicts.',
    docsLink: '/cli/commands/toolchain',
  },
  devenv: {
    id: 'devenv',
    title: 'Dev Environments',
    description:
      'Development environments provide isolated, reproducible workspaces with all required tools, dependencies, and configurations pre-installed.',
    whyItMatters:
      'Onboarding new developers takes days when they must manually install and configure dozens of tools. Environment drift causes bugs that are impossible to reproduce. You need consistent environments that work identically for everyone.',
    atmosSupport:
      'Atmos has **native devcontainer support** with `atmos devcontainer start/stop/shell`. Define your environment once in `.devcontainer`, and Atmos manages the container lifecycle. Works with VS Code, GitHub Codespaces, and any container runtime.',
    docsLink: '/cli/commands/devcontainer',
  },
  policy: {
    id: 'policy',
    title: 'Policy & Validation',
    description:
      'Policy and validation enforce guardrails on infrastructure configurations before deployment. They catch misconfigurations, security issues, and compliance violations early.',
    whyItMatters:
      'Infrastructure mistakes in production are costly and sometimes catastrophic. Code review alone can\'t catch every issue. You need automated policy enforcement that validates configurations against your organization\'s rules before anything deploys.',
    atmosSupport:
      'Atmos provides **native OPA and JSON Schema validation**. Write policies in Rego, validate with `atmos validate stacks`, and block non-compliant deployments. Policies run locally and in CI—no external tools required.',
    docsLink: '/validation/validating',
  },
  delivery: {
    id: 'delivery',
    title: 'CI/CD & Delivery',
    description:
      'CI/CD for infrastructure automates planning, review, and deployment of infrastructure changes through pull requests and pipelines.',
    whyItMatters:
      'Manual infrastructure deployments are risky, slow, and don\'t scale. You need automated pipelines that plan changes, show diffs in PRs, and apply approved changes—all with proper audit trails and rollback capabilities.',
    atmosSupport:
      'Atmos uses **GitHub Actions natively** for CI/CD. Our official actions handle planning, applying, and drift detection. PR comments show what will change, approvals gate deployments, and everything integrates with your existing GitHub workflow.',
    docsLink: '/integrations/github-actions',
  },
  vendoring: {
    id: 'vendoring',
    title: 'Vendoring',
    description:
      'Vendoring copies external dependencies (modules, components, configurations) into your repository. It provides version control, offline access, and protection from upstream changes.',
    whyItMatters:
      'External dependencies can disappear, change unexpectedly, or introduce security issues. Vendoring gives you control over exactly what code runs in your infrastructure, with full audit trails and the ability to review changes before adopting them.',
    atmosSupport:
      'Atmos provides **native vendoring** that pulls from Git, OCI registries, HTTP, and S3. Define sources in `vendor.yaml`, run `atmos vendor pull`, and Atmos downloads and versions your dependencies. Simpler than Vendir, more powerful than git submodules.',
    docsLink: '/vendor',
  },
  templating: {
    id: 'templating',
    title: 'Templating',
    description:
      'Templating generates configuration files from templates and data. It enables dynamic values, environment-specific settings, and DRY configurations.',
    whyItMatters:
      'Static configurations don\'t scale. You need dynamic values—account IDs, region names, computed settings. Templating lets you generate configurations programmatically while keeping source files readable and maintainable.',
    atmosSupport:
      'Atmos **integrates Gomplate** for powerful templating with 200+ functions. Use templates in stack configurations for dynamic values, and in documentation for generated content. Access stack data, environment variables, and external data sources directly.',
    docsLink: '/cli/configuration/templates',
  },
  'plan-diff': {
    id: 'plan-diff',
    title: 'Plan Diff',
    description:
      'Plan Diff compares Terraform plans semantically to reveal the actual infrastructure changes, not just file-level differences.',
    whyItMatters:
      'Two different Terraform plan files can actually represent identical infrastructure changes. Traditional file diffs show noise—timestamp changes, formatting differences, resource ordering. You need to understand what will actually change in your infrastructure.',
    atmosSupport:
      'Atmos **Plan Diff** analyzes Terraform plans and shows only the semantic differences—what resources will be created, modified, or destroyed. Compare plans between branches, commits, or environments to understand the real impact of changes.',
    docsLink: '/cli/commands/terraform/plan-diff',
  },
  introspection: {
    id: 'introspection',
    title: 'Introspection',
    description:
      'Introspection provides deep visibility into your infrastructure—what exists, what changed, and what depends on what. It answers questions about your infrastructure without running Terraform.',
    whyItMatters:
      'As infrastructure grows, understanding what you have becomes increasingly difficult. Which components are affected by a change? What depends on this resource? What will be deployed? Without introspection, you\'re flying blind.',
    atmosSupport:
      'Atmos provides comprehensive **describe** and **list** commands. Use `atmos describe affected` to see what changed, `atmos describe dependents` to map dependencies, and `atmos list stacks/components` to browse your infrastructure.',
    docsLink: '/cli/commands/describe',
  },
  'stack-dependencies': {
    id: 'stack-dependencies',
    title: 'Stack Dependencies',
    description:
      'Stack dependencies manage the order of infrastructure deployment, ensuring resources are created in the correct sequence based on their relationships.',
    whyItMatters:
      'Real infrastructure has dependencies—networks before compute, databases before applications, IAM before services. Deploying in the wrong order causes failures. You need automatic dependency management.',
    atmosSupport:
      'Atmos supports **depends_on** for component dependencies with automatic execution order calculation. Define dependencies declaratively and let Atmos figure out the correct deployment sequence.',
    docsLink: '/components/terraform/state',
  },
  hooks: {
    id: 'hooks',
    title: 'Hooks & Lifecycle',
    description:
      'Hooks allow you to run custom logic before or after infrastructure operations, enabling validation, notifications, cleanup, and integration with external systems.',
    whyItMatters:
      'Infrastructure operations often need surrounding automation—pre-flight checks, post-deployment notifications, cleanup tasks. Without hooks, you resort to wrapper scripts that are hard to maintain.',
    atmosSupport:
      'Atmos provides **native hooks** that run before and after Terraform commands. Define hooks at the component or stack level to validate inputs, notify teams, or trigger downstream processes.',
    docsLink: '/cli/configuration/hooks',
  },
  stores: {
    id: 'stores',
    title: 'Store Integration',
    description:
      'Store integration provides native access to secret managers and configuration stores, allowing infrastructure to reference secrets without exposing them in configuration files.',
    whyItMatters:
      'Infrastructure needs secrets—API keys, passwords, certificates. Hardcoding them in config files is a security risk. You need native integration with secret managers that keeps secrets out of version control.',
    atmosSupport:
      'Atmos integrates with **multiple secret stores** via the `!store` YAML function. Reference secrets from AWS SSM, Secrets Manager, and other backends directly in your stack configurations.',
    docsLink: '/cli/configuration/stores',
  },
  tui: {
    id: 'tui',
    title: 'Terminal UI',
    description:
      'A Terminal UI (TUI) provides an interactive interface for navigating and managing infrastructure directly from the command line, without needing to remember complex commands.',
    whyItMatters:
      'Command-line tools can be intimidating, especially with many stacks and components. An interactive UI helps you discover what\'s available, select targets visually, and reduce typing errors.',
    atmosSupport:
      'Atmos includes an **interactive TUI** for browsing stacks and components. Navigate your infrastructure visually, select deployment targets, and execute commands—all without memorizing syntax.',
    docsLink: '/cli/commands',
  },
};

// =============================================================================
// NATIVE TOOL FEATURES - Atmos capabilities not available in native tools
// =============================================================================

// Features for Terraform BSL (has native stacks support)
const terraformBslFeatures: FeatureComparison[] = [
  { feature: 'Stack Configuration', atmos: true, tool: true },
  { feature: 'Component Inheritance', atmos: true, tool: false },
  { feature: 'Stack Imports', atmos: true, tool: false },
  { feature: 'Deep Merging', atmos: true, tool: false },
  { feature: 'Remote Root Modules', atmos: true, tool: false },
  { feature: 'Dynamic Backend Generation', atmos: true, tool: false },
  { feature: 'Dynamic Provider Generation', atmos: true, tool: false },
  { feature: 'Automatic Backend Provisioning', atmos: true, tool: false },
  { feature: 'Plan Diff', atmos: true, tool: false },
  { feature: 'OPA Validation', atmos: true, tool: false },
  { feature: 'JSON Schema', atmos: true, tool: false },
  { feature: 'Native Workflows', atmos: true, tool: false },
  { feature: 'Custom Commands', atmos: true, tool: false },
  { feature: 'Vendoring', atmos: true, tool: false },
  { feature: 'Code Generation', atmos: true, tool: false },
  { feature: 'Toolchain Installation', atmos: true, tool: false },
  { feature: 'Dev Environments', atmos: true, tool: false },
  { feature: 'Authentication (SSO/OIDC)', atmos: true, tool: false },
];

// Features for OpenTofu (no native stacks support)
const opentofuFeatures: FeatureComparison[] = [
  { feature: 'Stack Configuration', atmos: true, tool: false },
  { feature: 'Component Inheritance', atmos: true, tool: false },
  { feature: 'Stack Imports', atmos: true, tool: false },
  { feature: 'Deep Merging', atmos: true, tool: false },
  { feature: 'Remote Root Modules', atmos: true, tool: false },
  { feature: 'Dynamic Backend Generation', atmos: true, tool: false },
  { feature: 'Dynamic Provider Generation', atmos: true, tool: false },
  { feature: 'Automatic Backend Provisioning', atmos: true, tool: false },
  { feature: 'Plan Diff', atmos: true, tool: false },
  { feature: 'OPA Validation', atmos: true, tool: false },
  { feature: 'JSON Schema', atmos: true, tool: false },
  { feature: 'Native Workflows', atmos: true, tool: false },
  { feature: 'Custom Commands', atmos: true, tool: false },
  { feature: 'Vendoring', atmos: true, tool: false },
  { feature: 'Code Generation', atmos: true, tool: false },
  { feature: 'Toolchain Installation', atmos: true, tool: false },
  { feature: 'Dev Environments', atmos: true, tool: false },
  { feature: 'Authentication (SSO/OIDC)', atmos: true, tool: false },
];

// Features for Helmfile/Packer (no Remote Root Modules or Terraform-specific features)
const nonTerraformToolFeatures: FeatureComparison[] = [
  { feature: 'Stack Configuration', atmos: true, tool: false },
  { feature: 'Component Inheritance', atmos: true, tool: false },
  { feature: 'Stack Imports', atmos: true, tool: false },
  { feature: 'Deep Merging', atmos: true, tool: false },
  { feature: 'OPA Validation', atmos: true, tool: false },
  { feature: 'JSON Schema', atmos: true, tool: false },
  { feature: 'Native Workflows', atmos: true, tool: false },
  { feature: 'Custom Commands', atmos: true, tool: false },
  { feature: 'Vendoring', atmos: true, tool: false },
  { feature: 'Code Generation', atmos: true, tool: false },
  { feature: 'Toolchain Installation', atmos: true, tool: false },
  { feature: 'Dev Environments', atmos: true, tool: false },
  { feature: 'Authentication (SSO/OIDC)', atmos: true, tool: false },
];

// =============================================================================
// NATIVE INTEGRATIONS - Tools Atmos orchestrates natively
// =============================================================================

export const supportedTools: Tool[] = [
  {
    id: 'terraform',
    name: 'Terraform',
    url: 'https://github.com/hashicorp/terraform',
    description: 'Infrastructure as Code engine by HashiCorp',
    category: 'Infrastructure as Code',
    relationship: 'supported',
    details: 'Terraform is the industry-standard tool for defining and provisioning infrastructure using declarative configuration files. It supports hundreds of cloud providers and services.',
    atmosComparison: 'Atmos **orchestrates Terraform natively** with stack-based configuration, component inheritance, and automatic `.tfvars` generation. Run `atmos terraform plan` and `atmos terraform apply` for seamless execution.',
    featureComparison: terraformBslFeatures,
  },
  {
    id: 'opentofu',
    name: 'OpenTofu',
    url: 'https://github.com/opentofu/opentofu',
    description: 'Open-source Terraform fork by the Linux Foundation',
    category: 'Infrastructure as Code',
    relationship: 'supported',
    details: 'OpenTofu is an open-source fork of Terraform, maintained by the Linux Foundation. It provides the same functionality with a truly open-source license.',
    atmosComparison: 'Atmos **orchestrates OpenTofu natively** as a drop-in replacement for Terraform. Configure `command` in atmos.yaml to use `tofu` and get all the same stack-based configuration benefits.',
    featureComparison: opentofuFeatures,
  },
  {
    id: 'helmfile',
    name: 'Helmfile',
    url: 'https://github.com/helmfile/helmfile',
    description: 'Declarative Helm chart orchestration',
    category: 'Kubernetes',
    relationship: 'supported',
    details: 'Helmfile manages collections of Helm charts with declarative syntax, combining them into a "stack" for deployment to Kubernetes. It handles environmental configuration and deep merging.',
    atmosComparison: 'Atmos **orchestrates Helmfile natively**, applying the same stack-based configuration approach to Helm charts. Define Helmfile components and orchestrate Kubernetes deployments alongside your infrastructure.',
    featureComparison: nonTerraformToolFeatures,
  },
  {
    id: 'packer',
    name: 'Packer',
    url: 'https://github.com/hashicorp/packer',
    description: 'Machine image builder by HashiCorp',
    category: 'Image Building',
    relationship: 'supported',
    details: 'Packer automates the creation of machine images for multiple platforms from a single source configuration.',
    atmosComparison: 'Atmos **orchestrates Packer natively** for building machine images as part of your infrastructure workflow. Define Packer components in your stacks and build with `atmos packer build`.',
    featureComparison: nonTerraformToolFeatures,
  },
  {
    id: 'custom-commands',
    name: 'Custom Commands',
    url: '/cli/configuration/commands',
    description: 'Integrate any CLI tool with Atmos',
    category: 'Extensibility',
    relationship: 'supported',
    details: 'Custom commands let you extend Atmos with any CLI tool. Define commands in atmos.yaml and combine them with native toolchain installation to integrate tools like kubectl, aws-cli, ansible, or any other CLI into your infrastructure workflows.',
    atmosComparison: 'Use **[custom commands](/cli/configuration/commands)** to integrate any tool not natively supported. Combined with the **[toolchain](/cli/commands/toolchain)** for automatic installation, you can seamlessly add any CLI tool to your Atmos workflows.',
  },
];

// =============================================================================
// TERRAFORM WRAPPERS - Alternative approaches to Atmos
// =============================================================================

// Comprehensive feature comparison for Terraform wrappers
// Shows all Atmos capabilities that wrappers typically lack
const wrapperFeatures = {
  // Core configuration
  stackConfiguration: { feature: 'Stack Configuration', atmos: true },
  codeGeneration: { feature: 'Code Generation', atmos: true },
  componentInheritance: { feature: 'Component Inheritance', atmos: true },
  stackImports: { feature: 'Stack Imports', atmos: true },
  deepMerging: { feature: 'Deep Merging', atmos: true },
  // Remote modules
  remoteRootModules: { feature: 'Remote Root Modules', atmos: true },
  // Validation & Policy
  opaValidation: { feature: 'OPA Validation', atmos: true },
  jsonSchema: { feature: 'JSON Schema', atmos: true },
  // Automation
  nativeWorkflows: { feature: 'Native Workflows', atmos: true },
  customCommands: { feature: 'Custom Commands', atmos: true },
  // Infrastructure
  vendoring: { feature: 'Vendoring', atmos: true },
  templating: { feature: 'Go Templates', atmos: true },
  toolchainInstall: { feature: 'Toolchain Installation', atmos: true },
  devEnvironments: { feature: 'Dev Environments', atmos: true },
  authentication: { feature: 'Authentication (SSO/OIDC)', atmos: true },
};

export const terraformWrappers: Tool[] = [
  {
    id: 'terragrunt',
    name: 'Terragrunt',
    url: 'https://github.com/gruntwork-io/terragrunt',
    description: 'Thin wrapper for Terraform with DRY configurations',
    category: 'Terraform Wrapper',
    relationship: 'wrapper',
    details: 'Terragrunt is a tool built in Golang that is a thin wrapper for Terraform that provides extra tools for working with multiple Terraform modules. It focuses on keeping configurations DRY through code generation.',
    atmosComparison: 'Atmos now supports [code generation](/roadmap#feature-parity) like Terragrunt, but emphasizes modular architecture for DRY, testable root modules. Atmos encourages sufficient parameterization of components to keep them versatile and highly reusable.',
    featureComparison: [
      { ...wrapperFeatures.stackConfiguration, tool: true },
      { ...wrapperFeatures.codeGeneration, tool: true },
      { ...wrapperFeatures.componentInheritance, tool: false },
      { ...wrapperFeatures.stackImports, tool: false },
      { ...wrapperFeatures.deepMerging, tool: false },
      { ...wrapperFeatures.remoteRootModules, tool: true },
      { ...wrapperFeatures.opaValidation, tool: false },
      { ...wrapperFeatures.jsonSchema, tool: false },
      { ...wrapperFeatures.nativeWorkflows, tool: false },
      { ...wrapperFeatures.customCommands, tool: false },
      { ...wrapperFeatures.vendoring, tool: false },
      { ...wrapperFeatures.templating, tool: false },
      { ...wrapperFeatures.toolchainInstall, tool: false },
      { ...wrapperFeatures.devEnvironments, tool: false },
      { ...wrapperFeatures.authentication, tool: false },
    ],
  },
  {
    id: 'terramate',
    name: 'Terramate',
    url: 'https://github.com/terramate-io/terramate',
    description: 'Stack management with change detection and code generation',
    category: 'Terraform Wrapper',
    relationship: 'wrapper',
    details: 'Terramate is a tool built in Golang for managing multiple Terraform stacks with support for change detection and code generation.',
    atmosComparison: 'Atmos is a framework for IaC, while Terramate is a SaaS product with a CLI. Both support stack-based organization, but Atmos focuses on configuration composition through imports and inheritance, while Terramate emphasizes change detection.',
    featureComparison: [
      { ...wrapperFeatures.stackConfiguration, tool: true },
      { ...wrapperFeatures.codeGeneration, tool: true },
      { feature: 'Change Detection', atmos: true, tool: true },
      { ...wrapperFeatures.componentInheritance, tool: false },
      { ...wrapperFeatures.stackImports, tool: false },
      { ...wrapperFeatures.deepMerging, tool: false },
      { ...wrapperFeatures.remoteRootModules, tool: true },
      { ...wrapperFeatures.opaValidation, tool: false },
      { ...wrapperFeatures.jsonSchema, tool: false },
      { ...wrapperFeatures.nativeWorkflows, tool: false },
      { ...wrapperFeatures.customCommands, tool: false },
      { ...wrapperFeatures.vendoring, tool: false },
      { ...wrapperFeatures.templating, tool: false },
      { ...wrapperFeatures.toolchainInstall, tool: false },
      { ...wrapperFeatures.devEnvironments, tool: false },
      { ...wrapperFeatures.authentication, tool: false },
    ],
  },
  {
    id: 'terraspace',
    name: 'Terraspace',
    url: 'https://github.com/boltops-tools/terraspace',
    description: 'Opinionated Ruby framework for Terraform',
    category: 'Terraform Wrapper',
    relationship: 'wrapper',
    details: 'Terraspace is a Ruby-based framework for Terraform with generators, tfvars layering, Terrafile for module sourcing, and Ruby helpers for dynamic configuration.',
    atmosComparison: 'Atmos is written in Go with cross-platform binaries and provides a flexible, configuration-driven approach. Terraspace offers similar capabilities through Ruby conventions and ERB templating.',
    featureComparison: [
      { ...wrapperFeatures.stackConfiguration, tool: true },
      { ...wrapperFeatures.codeGeneration, tool: true },
      { ...wrapperFeatures.componentInheritance, tool: false },
      { ...wrapperFeatures.stackImports, tool: false },
      { ...wrapperFeatures.deepMerging, tool: false },
      { ...wrapperFeatures.remoteRootModules, tool: true },
      { ...wrapperFeatures.opaValidation, tool: false },
      { ...wrapperFeatures.jsonSchema, tool: false },
      { ...wrapperFeatures.nativeWorkflows, tool: false },
      { ...wrapperFeatures.customCommands, tool: false },
      { ...wrapperFeatures.vendoring, tool: true },
      { ...wrapperFeatures.templating, tool: true },
      { ...wrapperFeatures.toolchainInstall, tool: false },
      { ...wrapperFeatures.devEnvironments, tool: false },
      { ...wrapperFeatures.authentication, tool: false },
    ],
  },
];

// =============================================================================
// DELIVERY TOOLS - Complementary CI/CD platforms
// =============================================================================

const deliveryFeatures: FeatureComparison[] = [
  { feature: 'GitHub Actions Native', atmos: true, tool: false },
  { feature: 'PR Automation', atmos: true, tool: true },
  { feature: 'Stack Configuration', atmos: true, tool: false },
  { feature: 'Component Inheritance', atmos: true, tool: false },
  { feature: 'Multi-tool Support', atmos: true, tool: false },
  { feature: 'OPA Policies', atmos: true, tool: true },
  { feature: 'Open Source', atmos: true, tool: false },
  { feature: 'No Managed Platform', atmos: true, tool: false },
];

export const deliveryTools: Tool[] = [
  {
    id: 'atlantis',
    name: 'Atlantis',
    url: 'https://github.com/runatlantis/atlantis',
    description: 'Terraform PR automation for GitHub/GitLab/Bitbucket',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'Atlantis is a self-hosted application that listens for Terraform pull request events and automatically runs `terraform plan` and `terraform apply` commands.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** for PR automation. Atlantis is supported via config generation through the atmos-atlantis GitHub Action, but we recommend GitHub Actions for simpler architecture.',
    featureComparison: [
      ...deliveryFeatures.slice(0, 6),
      { feature: 'Open Source', atmos: true, tool: true },
      { feature: 'No Managed Platform', atmos: true, tool: true },
    ],
  },
  {
    id: 'terrateam',
    name: 'Terrateam',
    url: 'https://github.com/terrateam/terrateam',
    description: 'GitHub-native Terraform automation',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'Terrateam is a GitHub-native platform for Terraform automation with built-in plan/apply workflows, RBAC, and drift detection.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** and doesn\'t require a separate managed platform. Use what you already have with GitHub Actions and Atmos.',
    featureComparison: deliveryFeatures,
  },
  {
    id: 'spacelift',
    name: 'Spacelift',
    url: 'https://spacelift.io/',
    description: 'IaC management platform with policy enforcement',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'Spacelift is a sophisticated CI/CD platform for infrastructure as code with features like policy enforcement, drift detection, and stack dependencies.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** and doesn\'t require a managed platform. Stack configuration, policy enforcement, and component orchestration are built-in.',
    featureComparison: deliveryFeatures,
  },
  {
    id: 'env0',
    name: 'env0',
    url: 'https://www.env0.com/',
    description: 'Self-service IaC automation platform',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'env0 is a platform for automating and managing infrastructure deployments with features like cost estimation, policy enforcement, and self-service capabilities.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** and doesn\'t require a managed platform. Use your existing CI/CD with Atmos for complete control.',
    featureComparison: deliveryFeatures,
  },
  {
    id: 'digger',
    name: 'Digger',
    url: 'https://github.com/diggerhq/digger',
    description: 'Open-source Terraform CI/CD orchestration',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'Digger is an open-source tool for running Terraform plan and apply commands in your CI/CD pipeline with features like PR comments and locking.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** for CI/CD. Digger provides similar PR automation but Atmos integrates configuration management directly.',
    featureComparison: [
      ...deliveryFeatures.slice(0, 6),
      { feature: 'Open Source', atmos: true, tool: true },
      { feature: 'No Managed Platform', atmos: true, tool: true },
    ],
  },
  {
    id: 'terraform-cloud',
    name: 'Terraform Cloud',
    url: 'https://www.terraform.io/',
    description: 'HashiCorp\'s managed Terraform service',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'Terraform Cloud is HashiCorp\'s managed service for Terraform with remote state management, run history, and team collaboration features.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** and doesn\'t require a managed platform. Remote state works with any S3-compatible backend.',
    featureComparison: deliveryFeatures,
  },
  {
    id: 'scalr',
    name: 'Scalr',
    url: 'https://www.scalr.com/',
    description: 'Enterprise IaC management platform',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'Scalr is an enterprise Terraform Cloud alternative with policy-as-code, cost estimation, multi-cloud support, and self-hosted or SaaS deployment options.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** and doesn\'t require a managed platform. Native OPA policies provide enterprise-grade governance.',
    featureComparison: deliveryFeatures,
  },
  {
    id: 'otf',
    name: 'OTF',
    url: 'https://github.com/leg100/otf',
    description: 'Open-source Terraform Cloud alternative',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'OTF (Open Terraform Framework) is an open-source, self-hosted replacement for Terraform Cloud with API compatibility, team management, VCS integration, and state management.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** and doesn\'t require a separate platform. OTF is a good choice if you need Terraform Cloud API compatibility.',
    featureComparison: [
      ...deliveryFeatures.slice(0, 6),
      { feature: 'Open Source', atmos: true, tool: true },
      { feature: 'No Managed Platform', atmos: true, tool: true },
    ],
  },
  {
    id: 'terramate-cloud',
    name: 'Terramate Cloud',
    url: 'https://cloud.terramate.io/',
    description: 'GitOps automation platform with drift detection',
    category: 'Delivery',
    relationship: 'delivery',
    details: 'Terramate Cloud is the managed platform from Terramate.io that provides GitOps automation, drift detection, observability, and collaboration features for Terraform and OpenTofu workflows.',
    atmosComparison: 'Atmos uses **GitHub Actions natively** and doesn\'t require a managed platform. Drift detection and change management are handled through native Atmos commands.',
    featureComparison: deliveryFeatures,
  },
];

// =============================================================================
// WORKFLOW ALTERNATIVES - Alternatives to Atmos workflows
// =============================================================================

const workflowFeatures: FeatureComparison[] = [
  { feature: 'YAML Syntax', atmos: true, tool: false },
  { feature: 'Typed Inputs', atmos: true, tool: false },
  { feature: 'Stack Integration', atmos: true, tool: false },
  { feature: 'Parallel Execution', atmos: false, tool: true },
  { feature: 'Conditional Logic', atmos: false, tool: true },
  { feature: 'Toolchain Installation', atmos: true, tool: false },
];

export const workflowTools: Tool[] = [
  {
    id: 'make',
    name: 'Make',
    url: 'https://www.gnu.org/software/make/',
    description: 'Classic build automation tool',
    category: 'Task Runner',
    relationship: 'workflows',
    details: 'Many companies (including Cloud Posse) started by leveraging `make` with `Makefile` and targets to call `terraform`. Using `make` is a popular method of orchestrating tools, but it has trouble scaling up to support large projects. The problem is that `make` targets do not support "natural" parameterization, which leads to a proliferation of environment variables that are difficult to validate or hacks like overloading make-targets and parsing them (e.g. `make apply/prod`). Makefiles are unintuitive for newcomers because they are first evaluated as a template and then executed as a script where each line of a target runs in a separate process space.',
    atmosComparison: 'Atmos **replaces** Make with native [workflows](/workflows). Define tasks in YAML with typed inputs and stack configuration integration—no Makefile quirks.',
    featureComparison: workflowFeatures,
  },
  {
    id: 'mage',
    name: 'Mage',
    url: 'https://github.com/magefile/mage',
    description: 'Make alternative using native Go functions',
    category: 'Task Runner',
    relationship: 'workflows',
    details: 'Mage is a make/rake-like build tool using native Golang and plain-old Go functions. Mage then automatically provides a CLI to call them as Makefile-like runnable targets.',
    atmosComparison: 'Atmos **replaces** Mage with native [workflows](/workflows). Define tasks in YAML without writing Go code, with full stack configuration integration.',
    featureComparison: workflowFeatures,
  },
  {
    id: 'task',
    name: 'Task',
    url: 'https://github.com/go-task/task',
    description: 'Modern task runner with Taskfile.yml',
    category: 'Task Runner',
    relationship: 'workflows',
    details: 'Task is a task runner and build tool that aims to be simpler and easier to use than GNU Make. It uses a YAML-based Taskfile format.',
    atmosComparison: 'Atmos **replaces** Task with native [workflows](/workflows). Similar YAML syntax, but workflows integrate directly with stacks and components.',
    featureComparison: [
      { feature: 'YAML Syntax', atmos: true, tool: true },
      { feature: 'Typed Inputs', atmos: true, tool: true },
      { feature: 'Stack Integration', atmos: true, tool: false },
      { feature: 'Parallel Execution', atmos: false, tool: true },
      { feature: 'Conditional Logic', atmos: false, tool: true },
      { feature: 'Toolchain Installation', atmos: true, tool: false },
    ],
  },
  {
    id: 'just',
    name: 'Just',
    url: 'https://github.com/casey/just',
    description: 'Command runner with a simple syntax',
    category: 'Task Runner',
    relationship: 'workflows',
    details: 'Just is a handy way to save and run project-specific commands. It has a simpler syntax than Make and focuses on running commands rather than building files.',
    atmosComparison: 'Atmos **replaces** Just with native [workflows](/workflows). YAML syntax with stack configuration integration and typed inputs.',
    featureComparison: workflowFeatures,
  },
];

// =============================================================================
// CUSTOM COMMAND ALTERNATIVES - Alternatives to Atmos custom commands
// =============================================================================

const commandFeatures: FeatureComparison[] = [
  { feature: 'YAML Definition', atmos: true, tool: true },
  { feature: 'Typed Arguments', atmos: true, tool: true },
  { feature: 'Stack Integration', atmos: true, tool: false },
  { feature: 'Component Context', atmos: true, tool: false },
  { feature: 'No Separate Tool', atmos: true, tool: false },
  { feature: 'Toolchain Installation', atmos: true, tool: false },
];

export const commandTools: Tool[] = [
  {
    id: 'appbuilder',
    name: 'AppBuilder',
    url: 'https://github.com/choria-io/appbuilder',
    description: 'Golang tool for creating friendly CLI wrappers',
    category: 'CLI Framework',
    relationship: 'commands',
    details: 'AppBuilder is a tool built in Golang to create a friendly CLI command that wraps your operational tools.',
    atmosComparison: 'Atmos **replaces** AppBuilder with native [custom commands](/cli/configuration/commands). Define CLI commands in YAML with typed arguments and stack integration.',
    featureComparison: commandFeatures,
  },
  {
    id: 'variant',
    name: 'Variant',
    url: 'https://github.com/mumoshu/variant2',
    description: 'CLI wrapper for scripts and tools',
    category: 'CLI Framework',
    relationship: 'commands',
    details: 'Variant lets you wrap all your scripts and CLIs into a modern CLI and a single-executable that can run anywhere. The earliest versions of Atmos were built on top of variant2 until it was rewritten from the ground up in pure Go.',
    atmosComparison: 'Atmos **replaces** Variant2 with native [custom commands](/cli/configuration/commands). Originally built on Variant2, Atmos now provides tighter infrastructure integration.',
    featureComparison: commandFeatures,
  },
];

// =============================================================================
// ECOSYSTEM TOOLS - Tools organized by infrastructure category
// =============================================================================

const authFeatures: FeatureComparison[] = [
  { feature: 'AWS SSO', atmos: true, tool: true },
  { feature: 'Role Switching', atmos: true, tool: true },
  { feature: 'Credential Caching', atmos: true, tool: true },
  { feature: 'Multiple Config Profiles', atmos: true, tool: true },
  { feature: 'Shell Execution', atmos: true, tool: true },
  { feature: 'Secrets Masking', atmos: true, tool: false },
  { feature: 'Multi-cloud', atmos: true, tool: false },
  { feature: 'K8s Auth (EKS)', atmos: true, tool: false },
  { feature: 'ECR Authentication', atmos: true, tool: false },
];

// Custom feature comparison for EKS-focused auth tools
const eksAuthFeatures: FeatureComparison[] = [
  { feature: 'AWS SSO', atmos: true, tool: false },
  { feature: 'Role Switching', atmos: true, tool: false },
  { feature: 'Credential Caching', atmos: true, tool: false },
  { feature: 'Multiple Config Profiles', atmos: true, tool: false },
  { feature: 'Shell Execution', atmos: true, tool: false },
  { feature: 'Secrets Masking', atmos: true, tool: false },
  { feature: 'Multi-cloud', atmos: true, tool: false },
  { feature: 'K8s Auth (EKS)', atmos: true, tool: true },
  { feature: 'ECR Authentication', atmos: true, tool: false },
];

// Custom feature comparison for ECR credential helper
const ecrAuthFeatures: FeatureComparison[] = [
  { feature: 'AWS SSO', atmos: true, tool: false },
  { feature: 'Role Switching', atmos: true, tool: false },
  { feature: 'Credential Caching', atmos: true, tool: true },
  { feature: 'Multiple Config Profiles', atmos: true, tool: false },
  { feature: 'Shell Execution', atmos: true, tool: false },
  { feature: 'Secrets Masking', atmos: true, tool: false },
  { feature: 'Multi-cloud', atmos: true, tool: false },
  { feature: 'K8s Auth (EKS)', atmos: true, tool: false },
  { feature: 'ECR Authentication', atmos: true, tool: true },
];

export const authTools: Tool[] = [
  {
    id: 'granted',
    name: 'Granted',
    url: 'https://github.com/common-fate/granted',
    description: 'AWS credential switching and SSO',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'Granted is a command-line tool to assume AWS roles quickly with support for AWS SSO and credential caching.',
    atmosComparison: 'Atmos **replaces** Granted with native [auth](/cli/commands/auth). AWS SSO, role switching, credential caching, shells, exec, and console—all built-in.',
    featureComparison: authFeatures,
  },
  {
    id: 'aws-vault',
    name: 'aws-vault',
    url: 'https://github.com/99designs/aws-vault',
    description: 'AWS credential management in OS keychain',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'aws-vault securely stores and accesses AWS credentials in a development environment using the OS keychain.',
    atmosComparison: 'Atmos **replaces** aws-vault with native [auth](/cli/commands/auth). Secure credential management with SSO, role switching, and keychain integration.',
    featureComparison: authFeatures,
  },
  {
    id: 'leapp',
    name: 'Leapp',
    url: 'https://github.com/Noovolari/leapp',
    description: 'Cloud credentials manager with GUI',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'Leapp is a cross-platform application to manage cloud credentials with support for AWS, Azure, and GCP.',
    atmosComparison: 'Atmos **replaces** Leapp\'s CLI workflows with native [auth](/cli/commands/auth). Multi-cloud credential management without a separate GUI app.',
    featureComparison: authFeatures,
  },
  {
    id: 'aws-sso-cli',
    name: 'aws-sso-cli',
    url: 'https://github.com/synfinatic/aws-sso-cli',
    description: 'CLI helper to login and export AWS SSO credentials',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'aws-sso-cli makes it easy to login to AWS SSO and export credentials to your environment. Supports multiple accounts and roles with credential caching.',
    atmosComparison: 'Atmos **replaces** aws-sso-cli with native [auth](/cli/commands/auth). AWS SSO login, credential export, and caching—all built-in.',
    featureComparison: authFeatures,
  },
  {
    id: 'awsume',
    name: 'awsume',
    url: 'https://github.com/trek10inc/awsume',
    description: 'Quick role switching with shell integration',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'awsume is a convenient way to manage AWS credentials with shell integration. Super pragmatic for role switching (usually via `source`), quick to adopt, and minimal surface area.',
    atmosComparison: 'Atmos **replaces** awsume with native [auth](/cli/commands/auth). Role switching, shells, exec, and console commands—no sourcing required.',
    featureComparison: authFeatures,
  },
  {
    id: 'aws-iam-authenticator',
    name: 'aws-iam-authenticator',
    url: 'https://github.com/kubernetes-sigs/aws-iam-authenticator',
    description: 'IAM-based auth for Kubernetes (EKS)',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'aws-iam-authenticator enables IAM-based authentication for Kubernetes clusters, primarily used with Amazon EKS. Allows kubectl to authenticate using AWS IAM credentials.',
    atmosComparison: 'Atmos **replaces** aws-iam-authenticator for EKS auth with native [auth](/cli/commands/auth). Kubernetes authentication integrated with AWS SSO and role switching.',
    featureComparison: eksAuthFeatures,
  },
  {
    id: 'kubelogin',
    name: 'kubelogin',
    url: 'https://github.com/Azure/kubelogin',
    description: 'Azure AD auth for Kubernetes (AKS)',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'kubelogin is a client-go credential plugin implementing Azure AD authentication for Kubernetes. The Azure equivalent of aws-iam-authenticator for AKS clusters.',
    atmosComparison: 'Atmos will **replace** kubelogin for AKS auth when Azure support ships. Currently EKS authentication is supported via native [auth](/cli/commands/auth).',
    featureComparison: authFeatures,
  },
  {
    id: 'saml2aws',
    name: 'saml2aws',
    url: 'https://github.com/Versent/saml2aws',
    description: 'SAML identity provider to AWS credentials',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'saml2aws authenticates with SAML identity providers (Okta, OneLogin, ADFS, etc.) and returns temporary AWS credentials. Popular for organizations using SAML federation.',
    atmosComparison: 'Atmos **replaces** saml2aws with native [auth](/cli/commands/auth). SAML authentication integrated with role switching, credential caching, and shell execution.',
    featureComparison: authFeatures,
  },
  {
    id: 'amazon-ecr-credential-helper',
    name: 'amazon-ecr-credential-helper',
    url: 'https://github.com/awslabs/amazon-ecr-credential-helper',
    description: 'Docker credential helper for ECR',
    category: 'Auth Management',
    relationship: 'ecosystem',
    details: 'The Amazon ECR Docker Credential Helper automatically gets credentials for Amazon ECR when pushing or pulling images with Docker.',
    atmosComparison: 'Atmos **replaces** the credential helper with native ECR authentication through GitHub Actions. The `setup-atmos` action handles ECR login automatically.',
    featureComparison: ecrAuthFeatures,
  },
];

const toolchainFeatures: FeatureComparison[] = [
  { feature: 'Per-project Versions', atmos: true, tool: true },
  { feature: 'Auto-install on Demand', atmos: true, tool: false },
  { feature: 'Workflow Integration', atmos: true, tool: false },
  { feature: 'Custom Command Integration', atmos: true, tool: false },
  { feature: 'Shell Integration', atmos: true, tool: true },
  { feature: 'Concurrent Versions', atmos: true, tool: false },
  { feature: 'List Installed Tools', atmos: true, tool: true },
  { feature: 'Search for Tools', atmos: true, tool: true },
  { feature: 'Custom Registries', atmos: true, tool: true },
  { feature: 'Cross-platform (Win/Mac/Linux)', atmos: true, tool: false },
];

export const toolchainTools: Tool[] = [
  {
    id: 'asdf',
    name: 'asdf',
    url: 'https://github.com/asdf-vm/asdf',
    description: 'Multi-language version manager',
    category: 'Toolchain Management',
    relationship: 'ecosystem',
    details: 'asdf manages multiple runtime versions with a single CLI tool. It supports Terraform, Node.js, Python, Ruby, and many more through plugins.',
    atmosComparison: 'Atmos **replaces** asdf with native toolchain installation. Tools install cross-platform (Windows, Mac, Linux) and integrate directly into workflows.',
    featureComparison: toolchainFeatures,
  },
  {
    id: 'mise',
    name: 'mise',
    url: 'https://github.com/jdx/mise',
    description: 'Polyglot tool version manager (formerly rtx)',
    category: 'Toolchain Management',
    relationship: 'ecosystem',
    details: 'mise is a fast, modern version manager compatible with asdf plugins. It manages tool versions and environment variables.',
    atmosComparison: 'Atmos **replaces** mise with native toolchain installation. Cross-platform binary management with concurrent installation and workflow integration.',
    featureComparison: toolchainFeatures,
  },
  {
    id: 'aqua',
    name: 'aqua',
    url: 'https://github.com/aquaproj/aqua',
    description: 'Declarative CLI version manager',
    category: 'Toolchain Management',
    relationship: 'ecosystem',
    details: 'aqua is a declarative CLI version manager that manages CLI tools per project with a YAML configuration file.',
    atmosComparison: 'Atmos **replaces** aqua with native toolchain installation. YAML-based tool management with concurrent installs and workflow integration.',
    featureComparison: [
      { feature: 'Per-project Versions', atmos: true, tool: true },
      { feature: 'Auto-install on Demand', atmos: true, tool: true },
      { feature: 'Workflow Integration', atmos: true, tool: false },
      { feature: 'Custom Command Integration', atmos: true, tool: false },
      { feature: 'Shell Integration', atmos: true, tool: true },
      { feature: 'Concurrent Versions', atmos: true, tool: false },
      { feature: 'List Installed Tools', atmos: true, tool: true },
      { feature: 'Search for Tools', atmos: true, tool: true },
      { feature: 'Custom Registries', atmos: true, tool: true },
      { feature: 'Cross-platform (Win/Mac/Linux)', atmos: true, tool: true },
    ],
  },
  {
    id: 'tfenv',
    name: 'tfenv',
    url: 'https://github.com/tfutils/tfenv',
    description: 'Terraform version manager',
    category: 'Toolchain Management',
    relationship: 'ecosystem',
    details: 'tfenv is a Terraform version manager inspired by rbenv. It allows switching between multiple Terraform versions per project using a `.terraform-version` file.',
    atmosComparison: 'Atmos **replaces** tfenv with native toolchain installation. Terraform and OpenTofu versions managed alongside other tools with workflow integration.',
    featureComparison: toolchainFeatures,
  },
  {
    id: 'tofuenv',
    name: 'tofuenv',
    url: 'https://github.com/tofuutils/tofuenv',
    description: 'OpenTofu version manager',
    category: 'Toolchain Management',
    relationship: 'ecosystem',
    details: 'tofuenv is an OpenTofu version manager modeled after tfenv. It manages multiple OpenTofu versions using a `.opentofu-version` file.',
    atmosComparison: 'Atmos **replaces** tofuenv with native toolchain installation. OpenTofu versions managed alongside Terraform and other tools.',
    featureComparison: toolchainFeatures,
  },
  {
    id: 'homebrew',
    name: 'Homebrew',
    url: 'https://brew.sh/',
    description: 'Package manager for macOS and Linux',
    category: 'Toolchain Management',
    relationship: 'ecosystem',
    details: 'Homebrew is the most popular package manager for macOS, also available on Linux. It installs one global version per tool—switching versions requires manual intervention.',
    atmosComparison: 'Atmos **replaces** Homebrew for infrastructure tools. Run **multiple versions side-by-side** per project—each stack can pin its own Terraform version. Tools **auto-install on first use**, no manual setup needed.',
    featureComparison: [
      { feature: 'Per-project Versions', atmos: true, tool: false },
      { feature: 'Auto-install on Demand', atmos: true, tool: false },
      { feature: 'Multiple Versions Coexist', atmos: true, tool: false },
      { feature: 'Cross-platform (Win/Mac/Linux)', atmos: true, tool: false },
      { feature: 'Workflow Integration', atmos: true, tool: false },
    ],
  },
];

// =============================================================================
// DEV ENVIRONMENTS - Development environment tools
// =============================================================================

const devEnvFeatures: FeatureComparison[] = [
  { feature: 'Container-based', atmos: true, tool: true },
  { feature: 'VS Code Integration', atmos: true, tool: true },
  { feature: 'GitHub Codespaces', atmos: true, tool: true },
  { feature: 'Native CLI Commands', atmos: true, tool: false },
];

export const devEnvironmentTools: Tool[] = [
  {
    id: 'devcontainers',
    name: 'devcontainers',
    url: 'https://containers.dev/supporting',
    description: 'Development containers specification',
    category: 'Dev Environments',
    relationship: 'supported',
    details: 'Development containers provide full-featured development environments that can be used with VS Code, GitHub Codespaces, and other tools.',
    atmosComparison: 'Atmos has **native devcontainer support** with `atmos devcontainer start/stop/shell`. Run containers as your development environment with full CLI control.',
    featureComparison: devEnvFeatures,
  },
  {
    id: 'devcontainer-cli',
    name: 'Dev Container CLI',
    url: 'https://github.com/devcontainers/cli',
    description: 'Reference CLI implementation for dev containers',
    category: 'Dev Environments',
    relationship: 'ecosystem',
    details: 'The Dev Container CLI is the reference implementation for the Dev Container Spec. It can create and configure dev containers, prebuild configurations, and run lifecycle scripts.',
    atmosComparison: 'Atmos **wraps** the Dev Container CLI with native `atmos devcontainer` commands. Get the same capabilities with stack integration and simpler commands.',
    featureComparison: devEnvFeatures,
  },
  {
    id: 'devpod',
    name: 'DevPod',
    url: 'https://github.com/loft-sh/devpod',
    description: 'Client-only dev environments on any backend',
    category: 'Dev Environments',
    relationship: 'ecosystem',
    details: 'DevPod is a client-only tool to create reproducible developer environments based on devcontainer.json on any backend—local, Kubernetes, remote machines, or cloud VMs.',
    atmosComparison: 'Atmos devcontainers provide a **native alternative** with `atmos devcontainer` commands. Stack integration and simpler CLI for container-based environments.',
    featureComparison: [
      { feature: 'Container-based', atmos: true, tool: true },
      { feature: 'Multi-backend Support', atmos: false, tool: true },
      { feature: 'VS Code Integration', atmos: true, tool: true },
      { feature: 'Native CLI Commands', atmos: true, tool: true },
    ],
  },
  {
    id: 'devenv',
    name: 'devenv',
    url: 'https://github.com/cachix/devenv',
    description: 'Nix-based dev environments with devcontainer support',
    category: 'Dev Environments',
    relationship: 'ecosystem',
    details: 'Cachix devenv uses Nix to create development environments and can automatically generate .devcontainer.json files for use with any dev container supporting tool.',
    atmosComparison: 'Atmos devcontainers provide a **container-based alternative** without Nix complexity. Native `atmos devcontainer` commands for simpler setup.',
    featureComparison: [
      { feature: 'Isolated Environments', atmos: true, tool: true },
      { feature: 'Reproducible', atmos: true, tool: true },
      { feature: 'Devcontainer Generation', atmos: false, tool: true },
      { feature: 'Native CLI Commands', atmos: true, tool: true },
      { feature: 'No Nix Required', atmos: true, tool: false },
    ],
  },
  {
    id: 'devbox',
    name: 'Devbox',
    url: 'https://github.com/jetify-com/devbox',
    description: 'Nix-powered isolated dev environments',
    category: 'Dev Environments',
    relationship: 'ecosystem',
    details: 'Jetify Devbox creates isolated, reproducible development environments using Nix without requiring Nix knowledge. Has VS Code integration for devcontainer generation.',
    atmosComparison: 'Atmos devcontainers provide a **container-based alternative** to Devbox. Native `atmos devcontainer` commands manage your development environment.',
    featureComparison: [
      { feature: 'Isolated Environments', atmos: true, tool: true },
      { feature: 'Reproducible', atmos: true, tool: true },
      { feature: 'VS Code Integration', atmos: true, tool: true },
      { feature: 'Native CLI Commands', atmos: true, tool: true },
      { feature: 'No Nix Required', atmos: true, tool: false },
    ],
  },
];

const policyFeatures: FeatureComparison[] = [
  { feature: 'OPA/Rego Policies', atmos: true, tool: true },
  { feature: 'Stack Validation', atmos: true, tool: false },
  { feature: 'Component Validation', atmos: true, tool: false },
  { feature: 'JSON Schema', atmos: true, tool: false },
];

export const policyTools: Tool[] = [
  {
    id: 'opa',
    name: 'OPA',
    url: 'https://github.com/open-policy-agent/opa',
    description: 'Open Policy Agent for policy-as-code',
    category: 'Policy & Validation',
    relationship: 'ecosystem',
    details: 'OPA is a general-purpose policy engine that enables unified, context-aware policy enforcement across the stack.',
    atmosComparison: 'Atmos has **native OPA integration**. Define policies in Rego and validate with `atmos validate stacks`. No separate OPA installation required.',
    featureComparison: policyFeatures,
  },
  {
    id: 'conftest',
    name: 'Conftest',
    url: 'https://github.com/open-policy-agent/conftest',
    description: 'Policy testing for structured data',
    category: 'Policy & Validation',
    relationship: 'ecosystem',
    details: 'Conftest helps write tests against structured configuration data using OPA\'s Rego language.',
    atmosComparison: 'Atmos **replaces** Conftest with native OPA validation via `atmos validate stacks`. Same Rego policies, integrated directly.',
    featureComparison: policyFeatures,
  },
  {
    id: 'checkov',
    name: 'Checkov',
    url: 'https://github.com/bridgecrewio/checkov',
    description: 'Static analysis for IaC security',
    category: 'Policy & Validation',
    relationship: 'ecosystem',
    details: 'Checkov scans cloud infrastructure configurations for misconfigurations and security issues.',
    atmosComparison: 'Checkov complements Atmos OPA policies. Run through [custom commands](/cli/configuration/commands) or [workflows](/workflows) for security scanning.',
    featureComparison: [
      { feature: 'Security Scanning', atmos: false, tool: true },
      { feature: 'OPA/Rego Policies', atmos: true, tool: false },
      { feature: 'Workflow Integration', atmos: true, tool: false },
    ],
  },
  {
    id: 'tfsec',
    name: 'tfsec',
    url: 'https://github.com/aquasecurity/tfsec',
    description: 'Terraform security scanner',
    category: 'Policy & Validation',
    relationship: 'ecosystem',
    details: 'tfsec uses static analysis of Terraform code to spot potential misconfigurations and security issues.',
    atmosComparison: 'tfsec complements Atmos OPA policies. Integrate into [workflows](/workflows) for Terraform security scanning before deployment.',
    featureComparison: [
      { feature: 'Security Scanning', atmos: false, tool: true },
      { feature: 'OPA/Rego Policies', atmos: true, tool: false },
      { feature: 'Workflow Integration', atmos: true, tool: false },
    ],
  },
];

// =============================================================================
// VENDORING - Module vendoring tools
// =============================================================================

const vendoringFeatures: FeatureComparison[] = [
  { feature: 'Git Sources', atmos: true, tool: true },
  { feature: 'OCI Sources', atmos: true, tool: false },
  { feature: 'HTTP Sources', atmos: true, tool: true },
  { feature: 'S3 Sources', atmos: true, tool: false },
  { feature: 'Excludes/Includes', atmos: true, tool: true },
  { feature: 'Stack Integration', atmos: true, tool: false },
];

// Vendir-specific features (supports more source types than generic vendoring tools).
const vendirFeatures: FeatureComparison[] = [
  { feature: 'Git Sources', atmos: true, tool: true },
  { feature: 'HTTP Sources', atmos: true, tool: true },
  { feature: 'OCI/Docker Images', atmos: true, tool: true },
  { feature: 'GitHub Releases', atmos: false, tool: true },
  { feature: 'Helm Charts', atmos: false, tool: true },
  { feature: 'S3 Sources', atmos: true, tool: false },
  { feature: 'Excludes/Includes', atmos: true, tool: true },
  { feature: 'Stack Integration', atmos: true, tool: false },
];

export const vendoringTools: Tool[] = [
  {
    id: 'vendir',
    name: 'Vendir',
    url: 'https://github.com/carvel-dev/vendir',
    description: 'Declarative directory sync from multiple sources',
    category: 'Vendoring',
    relationship: 'ecosystem',
    details:
      'Vendir allows you to declaratively state what should be in a directory and sync data from various sources like git repos, helm charts, and OCI images.',
    atmosComparison:
      "Atmos provides native [vendoring](/vendor/) using HashiCorp's GoGetter. Vendir supports GitHub releases and Helm charts that Atmos doesn't, while Atmos offers S3 sources and direct stack integration that Vendir lacks.",
    featureComparison: vendirFeatures,
  },
  {
    id: 'terrafile',
    name: 'Terrafile',
    url: 'https://github.com/coretech/terrafile',
    description: 'Terraform module vendoring tool',
    category: 'Vendoring',
    relationship: 'ecosystem',
    details: 'Terrafile manages Terraform module dependencies using a simple YAML configuration, downloading modules from git or registry sources.',
    atmosComparison: 'Atmos **replaces** Terrafile with native [vendoring](/vendor/). More source types (OCI, S3, HTTP) and tighter stack integration.',
    featureComparison: vendoringFeatures,
  },
  {
    id: 'git-submodules',
    name: 'Git Submodules',
    url: 'https://git-scm.com/book/en/v2/Git-Tools-Submodules',
    description: 'Native Git repository embedding',
    category: 'Vendoring',
    relationship: 'ecosystem',
    details: 'Git Submodules allow you to keep a Git repository as a subdirectory of another Git repository. This lets you clone another repository into your project and keep your commits separate.',
    atmosComparison: 'Atmos **replaces** Git Submodules with native [vendoring](/vendor/). Supports selective file inclusion/exclusion, multiple source types, and stack configuration integration.',
    featureComparison: [
      { feature: 'Git Sources', atmos: true, tool: true },
      { feature: 'OCI Sources', atmos: true, tool: false },
      { feature: 'HTTP Sources', atmos: true, tool: false },
      { feature: 'S3 Sources', atmos: true, tool: false },
      { feature: 'Excludes/Includes', atmos: true, tool: false },
      { feature: 'Stack Integration', atmos: true, tool: false },
        ],
  },
];

// =============================================================================
// TEMPLATING - Configuration templating tools
// =============================================================================

const templatingFeatures: FeatureComparison[] = [
  { feature: 'Go Templates', atmos: true, tool: true },
  { feature: 'Data Sources', atmos: true, tool: true },
  { feature: 'Stack Integration', atmos: true, tool: false },
  { feature: 'Doc Generation', atmos: true, tool: false },
];

export const templatingTools: Tool[] = [
  {
    id: 'gomplate',
    name: 'Gomplate',
    url: 'https://github.com/hairyhenderson/gomplate',
    description: 'Template rendering with data sources',
    category: 'Templating',
    relationship: 'ecosystem',
    details: 'Gomplate is a flexible template renderer that supports various data sources and over 200 functions.',
    atmosComparison: 'Atmos **integrates** Gomplate for configuration templating AND documentation generation. Use Gomplate functions directly in stack configurations.',
    featureComparison: templatingFeatures,
  },
  {
    id: 'envsubst',
    name: 'envsubst',
    url: 'https://www.gnu.org/software/gettext/manual/html_node/envsubst-Invocation.html',
    description: 'Environment variable substitution',
    category: 'Templating',
    relationship: 'ecosystem',
    details: 'envsubst substitutes environment variable values in shell format strings. Simple but limited to environment variables only.',
    atmosComparison: 'Atmos **replaces** envsubst with Gomplate templating. More powerful with data sources, functions, and stack integration.',
    featureComparison: [
      { feature: 'Env Var Substitution', atmos: true, tool: true },
      { feature: 'Go Templates', atmos: true, tool: false },
      { feature: 'Data Sources', atmos: true, tool: false },
      { feature: 'Stack Integration', atmos: true, tool: false },
    ],
  },
  {
    id: 'j2cli',
    name: 'j2cli',
    url: 'https://github.com/kolypto/j2cli',
    description: 'Jinja2 templating from command line',
    category: 'Templating',
    relationship: 'ecosystem',
    details: 'j2cli renders Jinja2 templates from the command line with support for YAML, JSON, and environment variable data sources.',
    atmosComparison: 'Atmos **replaces** j2cli with Gomplate templating. Go templates with 200+ functions and native stack configuration integration.',
    featureComparison: templatingFeatures,
  },
  {
    id: 'consul-template',
    name: 'consul-template',
    url: 'https://github.com/hashicorp/consul-template',
    description: 'Template rendering with Consul/Vault data',
    category: 'Templating',
    relationship: 'ecosystem',
    details: 'consul-template populates templates with data from Consul KV, Vault secrets, and other sources. Commonly used for service discovery and secret injection.',
    atmosComparison: 'Atmos **replaces** consul-template for config templating. Use Gomplate with store functions for secrets and stack-aware configuration.',
    featureComparison: templatingFeatures,
  },
  {
    id: 'confd',
    name: 'confd',
    url: 'https://github.com/kelseyhightower/confd',
    description: 'Lightweight config management with templates',
    category: 'Templating',
    relationship: 'ecosystem',
    details: 'confd manages local application configuration files using templates and data from etcd, Consul, or environment variables.',
    atmosComparison: 'Atmos **replaces** confd for infrastructure config templating. Gomplate templates with native stack configuration and data source integration.',
    featureComparison: templatingFeatures,
  },
];

// =============================================================================
// CONCEPTUAL INSPIRATIONS
// =============================================================================

export const inspirationTools: Tool[] = [
  {
    id: 'react',
    name: 'React',
    url: 'https://react.dev/',
    description: 'Component-based UI architecture',
    category: 'Conceptual Inspiration',
    relationship: 'inspiration',
    details: 'React\'s component-based architecture serves as a key inspiration for Atmos. By breaking down UIs into reusable components, React simplifies the development of complex applications.',
    atmosComparison: 'Similarly, Atmos promotes modularity in infrastructure as code, allowing components to be reused across different environments and projects. In Atmos, any Terraform "root module" may be used as a component.',
  },
  {
    id: 'kustomize',
    name: 'Kustomize',
    url: 'https://kustomize.io/',
    description: 'Template-free Kubernetes configuration',
    category: 'Conceptual Inspiration',
    relationship: 'inspiration',
    details: 'Kustomize introduces a template-free way to customize Kubernetes configurations, focusing on overlays and inheritance to manage configuration variations.',
    atmosComparison: 'Atmos adopts a similar approach, enabling users to import, overlay, and override configurations efficiently, thereby simplifying the management of complex infrastructure setups, all without relying on templating. However, Atmos now also supports advanced templating as an escape hatch.',
  },
  {
    id: 'helmfile-inspiration',
    name: 'Helmfile',
    url: 'https://helmfile.com',
    description: 'Stack orchestration for Helm charts',
    category: 'Conceptual Inspiration',
    relationship: 'inspiration',
    details: 'Helmfile manages collections of Helm charts with declarative syntax, combining them into a "stack" for deployment to Kubernetes. It handles environmental configuration, deep merging it, and evaluating templates with a Go template engine.',
    atmosComparison: 'Atmos draws from Helmfile\'s ability to orchestrate multiple tools, applying the concept to Terraform root modules to manage interdependencies and deployment order. Atmos generates .tfvar files like Helmfile generates Helm values files.',
  },
  {
    id: 'helm',
    name: 'Helm Charts',
    url: 'https://helm.sh/',
    description: 'Parameterized Kubernetes packaging',
    category: 'Conceptual Inspiration',
    relationship: 'inspiration',
    details: 'Helm Charts provide a packaging format for deploying applications on Kubernetes, simplifying the processes of defining, installing, and upgrading applications.',
    atmosComparison: 'The concept is that if your root modules are sufficiently parameterized, they function much like Helm charts. You only need to supply the appropriate values to achieve the desired configuration.',
  },
  {
    id: 'vendir-inspiration',
    name: 'Vendir',
    url: 'https://github.com/carvel-dev/vendir',
    description: 'Declarative vendoring approach',
    category: 'Conceptual Inspiration',
    relationship: 'inspiration',
    details: 'Vendir by VMWare Tanzu served as the basis for Atmos\'s initial vendoring implementation.',
    atmosComparison: 'After using Vendir, we realized we only needed a simpler subset of its functionality. We implemented our own version using HashiCorp\'s GoGetter library. Additionally, we added OCI support, allowing vendoring to pull configurations from anywhere.',
  },
];

// =============================================================================
// ALL TOOLS - Combined export
// =============================================================================

export const allTools: Tool[] = [
  ...supportedTools,
  ...terraformWrappers,
  ...deliveryTools,
  ...workflowTools,
  ...commandTools,
  ...authTools,
  ...toolchainTools,
  ...devEnvironmentTools,
  ...policyTools,
  ...vendoringTools,
  ...templatingTools,
  ...inspirationTools,
];

// =============================================================================
// TOOL CATEGORIES - Structured category metadata for ToolCategory component
// =============================================================================

export interface ToolCategory {
  id: string;
  icon: string;
  title: string;
  tagline: string;
  tools: Tool[];
}

export const toolCategories: ToolCategory[] = [
  {
    id: 'supported',
    icon: 'RiCodeBoxLine',
    title: 'Native Integrations',
    tagline:
      'Atmos enhances the tools you already use—adding powerful conventions while keeping you in full control. Comprehensive enough to be complete, flexible enough to never limit you.',
    tools: supportedTools,
  },
  {
    id: 'wrappers',
    icon: 'RiStackLine',
    title: 'Terraform Wrappers',
    tagline:
      'Atmos is the alternative to stitching together single-purpose wrapper tools—one unified CLI for stack configuration, component inheritance, and deep merging',
    tools: terraformWrappers,
  },
  {
    id: 'auth',
    icon: 'RiShieldKeyholeLine',
    title: 'Cloud Authentication',
    tagline:
      'Atmos is the alternative to juggling multiple CLI credential helpers—providing unified authentication across AWS, Azure, GCP, and GitHub OIDC with automatic session management',
    tools: authTools,
  },
  {
    id: 'workflows',
    icon: 'RiFlowChart',
    title: 'Workflows & Task Runners',
    tagline:
      'Atmos is the alternative to maintaining Makefiles and task runners—providing native YAML workflows with typed inputs and automatic retry logic',
    tools: workflowTools,
  },
  {
    id: 'commands',
    icon: 'RiTerminalBoxLine',
    title: 'Custom Commands',
    tagline:
      'Atmos is the alternative to building custom CLI tools—providing extensible commands with full access to stack context and component configuration',
    tools: commandTools,
  },
  {
    id: 'toolchain',
    icon: 'RiToolsLine',
    title: 'Toolchain Management',
    tagline:
      'Atmos is the alternative to managing tool versions separately—with access to 1,500+ packages via Aqua, automatically install and pin versions for Terraform, OpenTofu, and any CLI tool',
    tools: toolchainTools,
  },
  {
    id: 'devenv',
    icon: 'RiTerminalWindowLine',
    title: 'Dev Environments',
    tagline:
      'Atmos is the alternative to running dev containers manually from the command line or inside your IDE—providing native devcontainer lifecycle management',
    tools: devEnvironmentTools,
  },
  {
    id: 'policy',
    icon: 'RiShieldCheckLine',
    title: 'Policy & Validation',
    tagline:
      'Atmos OPA policies provide high-level architectural guardrails, while tools like TFSEC, Checkov, and Conftest give you low-level controls—bring your own tools and integrate them via custom commands',
    tools: policyTools,
  },
  {
    id: 'delivery',
    icon: 'RiRocketLine',
    title: 'CI/CD & Delivery',
    tagline:
      'Atmos is the alternative to paying for another platform to automate Terraform—with native GitHub Actions support, you control where and how your workflows run',
    tools: deliveryTools,
  },
  {
    id: 'vendoring',
    icon: 'RiDownloadLine',
    title: 'Vendoring',
    tagline:
      'Atmos is the alternative to external vendoring tools—providing native module vendoring with Git, OCI, HTTP, and S3 sources integrated directly into your stack configuration',
    tools: vendoringTools,
  },
  {
    id: 'templating',
    icon: 'RiFileCodeLine',
    title: 'Templating',
    tagline:
      'Atmos is the alternative to general-purpose templating tools—providing powerful Go template support with multiple data sources for ultimate flexibility',
    tools: templatingTools,
  },
];
