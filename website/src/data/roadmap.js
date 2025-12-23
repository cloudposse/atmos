/**
 * Atmos Roadmap Configuration
 *
 * This file contains all roadmap data including initiatives, milestones,
 * progress percentages, and GitHub issue associations.
 *
 * To update the roadmap:
 * 1. Adjust progress percentages as milestones complete
 * 2. Move milestones from "planned" to "in-progress" to "shipped"
 * 3. Update quarters as time progresses
 * 4. Add new GitHub issue numbers as they're created
 */

export const roadmapConfig = {
  vision:
    'One tool to orchestrate your entire infrastructure lifecycle — as convenient as PaaS, but as flexible as Terraform, sitting on top of the tools you know and love.',

  theme: {
    title: 'From Fragmented to Unified',
    description:
      'Atmos is consolidating the sprawl of infrastructure tooling into a cohesive, discoverable, zero-config experience that works identically locally and in CI.',
  },

  quarters: [
    { id: 'q1-2025', label: 'Q1 2025', status: 'completed' },
    { id: 'q2-2025', label: 'Q2 2025', status: 'completed' },
    { id: 'q3-2025', label: 'Q3 2025', status: 'completed' },
    { id: 'q4-2025', label: 'Q4 2025', status: 'current' },
    { id: 'q1-2026', label: 'Q1 2026', status: 'planned' },
    { id: 'q2-2026', label: 'Q2 2026', status: 'planned' },
  ],

  initiatives: [
    {
      id: 'auth',
      icon: 'RiLockLine',
      title: 'Unified Authentication',
      tagline: 'Replace a dozen auth tools with one identity layer',
      description:
        'The way humans login with SSO is different from how automation systems authenticate with OIDC. Yet most teams implement this with fragmented approaches. Atmos brings authentication into the core with native support for identity profiles configurable by runtime.',
      progress: 80,
      status: 'in-progress',
      milestones: [
        { label: 'Added `atmos auth` command framework', status: 'shipped', quarter: 'q2-2025', docs: '/cli/commands/auth/usage', changelog: 'introducing-atmos-auth', description: 'A unified command for managing authentication across all cloud providers and CI systems. One command to rule them all.' },
        { label: 'AWS IAM Identity Center (SSO)', status: 'shipped', quarter: 'q2-2025', docs: '/cli/configuration/auth/providers', changelog: 'introducing-atmos-auth', description: 'Native support for AWS SSO login with automatic credential caching and session refresh.' },
        { label: 'AWS IAM Users', status: 'shipped', quarter: 'q4-2025', docs: '/cli/configuration/auth/providers', description: 'Support for traditional IAM user credentials with secure credential management.' },
        { label: 'Assume Root capability', status: 'shipped', quarter: 'q4-2025', docs: '/cli/configuration/auth/providers', changelog: 'aws-assume-root-identity', description: 'Centralized root access management for organizations that need controlled root-level operations.' },
        { label: 'GitHub OIDC', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/providers', changelog: 'introducing-atmos-auth', description: 'Native GitHub Actions OIDC integration for secure, secretless CI/CD authentication to AWS.' },
        { label: 'Azure AD / Workload Identity Federation', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/providers', changelog: 'azure-authentication-support', description: 'Authenticate to Azure using Entra ID (Azure AD) with support for Workload Identity Federation for CI/CD.' },
        { label: 'SAML Provider', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/providers', changelog: 'introducing-atmos-auth', description: 'Enterprise SAML-based authentication for organizations using identity providers like Okta or OneLogin.' },
        { label: 'Keyring backends (system, file, memory)', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/keyring', changelog: 'flexible-keyring-backends', description: 'Flexible credential storage with system keychain integration, encrypted file storage, or in-memory sessions.' },
        { label: 'Component-level auth identities', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/identities', changelog: 'authentication-for-workflows-and-custom-commands', description: 'Define different AWS identities per component, enabling multi-account deployments from a single workflow.' },
        { label: 'Per-step workflow authentication', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/identities', changelog: 'authentication-for-workflows-and-custom-commands', description: 'Each workflow step can assume a different identity, enabling cross-account orchestration.' },
        { label: 'EKS Kubeconfig integration', status: 'in-progress', quarter: 'q4-2025', description: 'Automatic kubeconfig generation for EKS clusters using Atmos-managed AWS credentials.' },
        { label: 'ECR Authentication', status: 'planned', quarter: 'q1-2026', description: 'Native ECR login for container image operations without external tooling.' },
        { label: 'GCP Workload Identity', status: 'planned', quarter: 'q1-2026', description: 'Google Cloud authentication using Workload Identity Federation for secretless CI/CD.' },
        { label: 'GitHub Apps', status: 'planned', quarter: 'q1-2026', description: 'GitHub App authentication for fine-grained repository access and elevated rate limits.' },
      ],
      issues: [],
      prs: [
        { number: 1894, title: 'Add Azure OIDC/Workload Identity Federation provider' },
        { number: 1859, title: 'Add ECR authentication' },
        { number: 1884, title: 'Add EKS kubeconfig authentication integration PRD' },
        { number: 1887, title: 'Add PRD for aws/login provider (native SDK auth)' },
      ],
    },
    {
      id: 'dx',
      icon: 'RiFlashlightLine',
      title: 'Developer Experience & Zero-Config',
      tagline: 'Sane defaults, full configurability',
      description:
        'Too many parameters, too much configuration. Everything should just work out of the box while remaining fully customizable.',
      progress: 75,
      status: 'in-progress',
      milestones: [
        { label: 'Zero-config terminal output (auto TTY/color)', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/settings/terminal', changelog: 'zero-config-terminal-output', description: 'Automatic detection of terminal capabilities with smart color and formatting. No configuration needed.' },
        { label: 'Force flags (`--force-tty`, `--force-color`)', status: 'shipped', quarter: 'q3-2025', docs: '/cli/global-flags', changelog: 'zero-config-terminal-output', description: 'Override auto-detection for CI environments or screenshot generation with explicit TTY and color control.' },
        { label: 'Auto-degradation (TrueColor→256→16→None)', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/settings/terminal', changelog: 'zero-config-terminal-output', description: 'Graceful color fallback from 16 million colors to 256 to 16 to plain text based on terminal support.' },
        { label: 'Secret masking (built-in patterns + custom expressions)', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/settings/mask', changelog: 'zero-config-terminal-output', description: 'Automatic detection and masking of 120+ secret patterns (AWS keys, tokens, passwords) in terminal output.' },
        { label: 'Added `--chdir` flag for multi-repo workflows', status: 'shipped', quarter: 'q3-2025', docs: '/cli/global-flags', changelog: 'introducing-chdir-flag', description: 'Run Atmos commands from any directory by specifying the project root with --chdir.' },
        { label: 'Parent directory search & git root discovery', status: 'shipped', quarter: 'q3-2025', changelog: 'parent-directory-search-and-git-root-discovery', description: 'Automatic discovery of atmos.yaml by searching parent directories up to the git repository root.' },
        { label: 'Native Dev Containers', status: 'shipped', quarter: 'q4-2025', docs: '/cli/commands/devcontainer/devcontainer', changelog: 'native-devcontainer-support', description: 'First-class Dev Container support with automatic container lifecycle management and seamless development.' },
        { label: 'Interactive prompts for missing flags', status: 'shipped', quarter: 'q4-2025', changelog: 'interactive-flag-prompts', description: 'Smart prompts that ask for missing required flags with tab completion and validation.' },
        { label: 'Atmos Profiles', status: 'shipped', quarter: 'q4-2025', docs: '/cli/configuration/profiles', changelog: 'atmos-profiles', description: 'Named configuration profiles for different environments or projects, switchable on the fly.' },
        { label: 'Backend provisioning', status: 'shipped', quarter: 'q4-2025', docs: '/components/terraform/backend-provisioning', description: 'Automatic backend.tf generation—no more manually managing backend configuration files.' },
        { label: 'Streaming Terraform UI', status: 'in-progress', quarter: 'q4-2025', description: 'Real-time Terraform plan/apply visualization with resource-level progress tracking.' },
        { label: 'Native CI integration with summary templates', status: 'in-progress', quarter: 'q4-2025', description: 'GitHub/GitLab-native summaries with formatted plan output, cost estimates, and approval workflows.' },
        { label: 'Component-aware tab completion', status: 'shipped', quarter: 'q4-2025', docs: '/cli/commands/completion', changelog: 'component-aware-stack-completion', description: 'Shell completion that understands your stacks and components—type less, discover more.' },
      ],
      issues: [],
      prs: [
        { number: 1908, title: 'Add Terraform streaming UI with real-time visualization' },
        { number: 1891, title: 'Native CI Integration with Summary Templates and Terraform Command Registry' },
      ],
    },
    {
      id: 'discoverability',
      icon: 'RiSearchLine',
      title: 'Discoverability & List Commands',
      tagline: 'Everything should be discoverable',
      description:
        'As infrastructure grows, teams need to explore what exists—stacks, components, workflows—and newcomers need a way to orient themselves. Intuitive list commands make your entire infrastructure discoverable and queryable at a glance.',
      progress: 95,
      status: 'in-progress',
      milestones: [
        { label: 'Added `atmos list stacks`', status: 'shipped', quarter: 'q1-2025', docs: '/cli/commands/list/stacks', description: 'List all stacks in your infrastructure with filtering and formatting options.', codeExample: 'atmos list stacks --format json' },
        { label: 'Added `atmos list components`', status: 'shipped', quarter: 'q1-2025', docs: '/cli/commands/list/components', description: 'Discover all available components across your infrastructure catalog.', codeExample: 'atmos list components' },
        { label: 'Added `atmos list workflows`', status: 'shipped', quarter: 'q3-2025', docs: '/cli/commands/list/workflows', description: 'Browse available workflows with their descriptions and step counts.', codeExample: 'atmos list workflows' },
        { label: 'Added `atmos list affected` with spinner UI', status: 'shipped', quarter: 'q4-2025', docs: '/cli/commands/list/affected', changelog: 'list-affected-command', description: 'Identify which stacks and components are affected by your changes—perfect for targeted CI/CD.' },
        { label: 'Customizable list columns', status: 'shipped', quarter: 'q4-2025', changelog: 'customizable-list-command-output', description: 'Create custom views of your configuration with customizable columns—display exactly the data you need.' },
        { label: 'JMESPath queries (`--query`)', status: 'shipped', quarter: 'q4-2025', docs: '/cli/commands/list/stacks', description: 'Filter and transform list output with JMESPath queries for precise data extraction.', codeExample: 'atmos list stacks --query "[?components.terraform.vpc]"' },
      ],
      issues: [],
      prs: [
        { number: 1874, title: 'Add list affected command with spinner UI improvements' },
      ],
    },
    {
      id: 'workflows',
      icon: 'RiFlowChart',
      title: 'Workflows Overhaul',
      tagline: 'Bootstrap systems and create reusable patterns',
      description:
        'Bootstrapping new environments and repeating complex multi-step operations manually is error-prone and time-consuming. Workflows encode these patterns once and execute them reliably across teams and environments.',
      progress: 60,
      status: 'in-progress',
      milestones: [
        { label: 'Workflow step types (show, sleep, stage, alert, etc.)', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/workflows', description: 'Rich step types including message display, timed delays, user prompts, and alert notifications.' },
        { label: 'Input types for workflow steps', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/workflows', description: 'Typed inputs for workflow steps with validation—strings, numbers, booleans, and selections.' },
        { label: 'Working directory support', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/workflows', changelog: 'working-directory-support', description: 'Execute workflow steps in specific directories, enabling multi-repo orchestration.' },
        { label: 'Per-step authentication', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/identities', changelog: 'authentication-for-workflows-and-custom-commands', description: 'Each workflow step can use different credentials for cross-account deployments.' },
        { label: 'Unified task execution (`pkg/runner`)', status: 'shipped', quarter: 'q4-2025', description: 'A single execution engine for all task types—Terraform, Helmfile, shell, and custom commands.' },
        { label: 'New workflow step types', status: 'in-progress', quarter: 'q4-2025', docs: '/cli/configuration/workflows', description: 'Additional step types including parallel execution, conditional branching, and error handlers.' },
        { label: 'Workflow templating', status: 'in-progress', quarter: 'q4-2025', docs: '/cli/configuration/templates', description: 'Go template support in workflows for dynamic step generation and parameterization.' },
        { label: 'Workflow composition (reusable workflows)', status: 'planned', quarter: 'q1-2026', description: 'Import and compose workflows from other files to build complex pipelines from simple building blocks.' },
      ],
      issues: [],
      prs: [
        { number: 1899, title: 'Implement workflow step types with registry pattern' },
        { number: 1901, title: 'Create pkg/runner with unified task execution' },
      ],
    },
    {
      id: 'extensibility',
      icon: 'RiPlugLine',
      title: 'Extensibility & Custom Components',
      tagline: 'Truly extensible architecture',
      description:
        'Modern infrastructure spans Terraform, Kubernetes, serverless, and custom tooling. A single orchestration layer should manage all of it consistently, not just a subset.',
      progress: 50,
      status: 'in-progress',
      milestones: [
        { label: 'Custom command types', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/commands', description: 'Define your own command types beyond Terraform and Helmfile—shell scripts, Ansible, Pulumi, and more.' },
        { label: 'Added `!terraform.state` YAML function', status: 'shipped', quarter: 'q3-2025', docs: '/functions/yaml/terraform.state', description: 'The fastest way to retrieve state from Atmos, natively querying the configured state backend.', codeExample: 'vpc_id: !terraform.state vpc.outputs.vpc_id' },
        { label: 'Added `!terraform.output` YAML function', status: 'shipped', quarter: 'q3-2025', docs: '/functions/yaml/terraform.output', description: 'Reference outputs from other components without data sources.', codeExample: 'subnet_ids: !terraform.output vpc.private_subnet_ids' },
        { label: 'Added `!store` YAML function', status: 'shipped', quarter: 'q3-2025', docs: '/functions/yaml/store', description: 'Access secrets from configured stores (SSM, Secrets Manager, etc.).', codeExample: 'api_key: !store ssm:/myapp/api-key' },
        { label: 'Added `!include` YAML function', status: 'shipped', quarter: 'q3-2025', docs: '/functions/yaml/include', description: 'Include external YAML files for reusable configuration blocks.', codeExample: 'settings: !include common/settings.yaml' },
        { label: 'Added `!literal` YAML function', status: 'shipped', quarter: 'q4-2025', docs: '/functions/yaml/literal', changelog: 'literal-yaml-function', description: 'Pass raw HCL/JSON values without YAML interpretation for complex Terraform expressions.', codeExample: 'policy: !literal \'jsonencode({...})\'' },
        { label: '`metadata.name` for component workspace keys', status: 'shipped', quarter: 'q4-2025', docs: '/stacks/components/metadata', changelog: 'metadata-name-workspace-keys', description: 'Use custom names for Terraform workspaces instead of auto-generated ones.' },
        { label: 'Custom component types with registry', status: 'shipped', quarter: 'q4-2025', docs: '/cli/configuration/commands', changelog: 'introducing-command-registry-pattern', description: 'Register custom component types that integrate seamlessly with Atmos commands and workflows.' },
        { label: 'Semantic type completion', status: 'shipped', quarter: 'q4-2025', docs: '/cli/commands/completion', description: 'Context-aware shell completion that understands your component types and suggests valid options.' },
        { label: 'Auth support for custom commands', status: 'shipped', quarter: 'q4-2025', docs: '/cli/configuration/auth/identities', changelog: 'authentication-for-workflows-and-custom-commands', description: 'Custom commands can leverage Atmos authentication for cloud provider access.' },
        { label: 'Custom workflows for built-in component types', status: 'planned', quarter: 'q1-2026', description: 'Override default Terraform/Helmfile workflows with custom step sequences.' },
        { label: 'Plugin architecture', status: 'planned', quarter: 'q2-2026', description: 'Load external plugins for new component types, YAML functions, and integrations.' },
      ],
      issues: [],
      prs: [
        { number: 1904, title: 'Add custom component types for custom commands' },
      ],
    },
    {
      id: 'vendoring',
      icon: 'RiBox3Line',
      title: 'Vendoring & Resilience',
      tagline: 'Purpose-built engine with retry and resilience',
      description:
        'Terraform users expect to declare module sources inline. The source provisioner brings this pattern to stack configuration—declare where components come from and let vendoring handle the rest with retries, concurrency, and graceful failure recovery.',
      progress: 40,
      status: 'in-progress',
      milestones: [
        { label: 'Retry with exponential backoff', status: 'shipped', quarter: 'q3-2025', docs: '/cli/commands/vendor/vendor-pull', description: 'Automatic retries with increasing delays for transient network failures and rate limits.' },
        { label: 'Version constraints for vendor updates', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/vendor', changelog: 'version-constraint-validation', description: 'Semantic versioning constraints to control which versions are pulled during vendor updates.' },
        { label: 'Vendor registry pattern migration', status: 'in-progress', quarter: 'q4-2025', description: 'Refactoring vendoring to use a pluggable registry pattern for different source types.' },
        { label: 'Concurrent vendoring', status: 'planned', quarter: 'q1-2026', description: 'Parallel downloads for faster vendoring of large component catalogs.' },
        { label: 'Just-in-time vendoring', status: 'planned', quarter: 'q1-2026', description: 'Automatically vendor components on first use—no separate vendor step needed.' },
        { label: 'Component workdir provisioning', status: 'planned', quarter: 'q1-2026', description: 'Automatic working directory setup for components with dependencies and generated files.' },
      ],
      issues: [],
      prs: [
        { number: 1889, title: 'Migrate vendor to registry pattern + implement --stack flag' },
        { number: 1877, title: 'Implement source provisioner for JIT component vendoring' },
        { number: 1876, title: 'Implement component workdir provisioning and CRUD commands' },
      ],
    },
    {
      id: 'ci-cd',
      icon: 'RiGitBranchLine',
      title: 'CI/CD Simplification',
      tagline: 'Native CI/CD support — local = CI',
      description:
        'CI pipelines shouldn\'t require complicated workflows, custom actions, and shell commands just to run what should be a one liner. They should just work. What works locally should work identically in CI with minimal configuration.',
      progress: 35,
      status: 'in-progress',
      milestones: [
        { label: 'Native GitHub OIDC (via `atmos auth`)', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/auth/providers', changelog: 'introducing-atmos-auth', description: 'Secretless CI/CD with native OIDC—no AWS access keys stored in GitHub secrets.' },
        { label: 'CI Summary Templates (`--ci` flag)', status: 'in-progress', quarter: 'q4-2025', description: 'Beautiful PR comments with formatted Terraform plans, cost estimates, and approval buttons.' },
        { label: 'Terraform command registry', status: 'in-progress', quarter: 'q4-2025', changelog: 'terraform-command-registry-pattern', description: 'Centralized Terraform command configuration for consistent behavior across CI and local.' },
        { label: 'Easily sharing outputs between steps', status: 'planned', quarter: 'q1-2026', description: 'Pass Terraform outputs between GitHub Actions steps without manual JSON parsing.' },
        { label: 'Simplified GitHub Actions', status: 'planned', quarter: 'q1-2026', docs: '/integrations/github-actions/github-actions', description: 'Pre-built GitHub Actions that handle auth, planning, and applying with minimal configuration.' },
        { label: 'Native CI mode', status: 'planned', quarter: 'q1-2026', description: 'Automatic detection of CI environment with optimized output formatting and caching.' },
      ],
      issues: [],
      prs: [
        { number: 1891, title: 'Native CI Integration with Summary Templates and Terraform Command Registry' },
      ],
    },
    {
      id: 'migration',
      icon: 'RiExchangeLine',
      title: 'Feature Parity with Terragrunt',
      tagline: 'Familiar concepts for Terragrunt users',
      description:
        'Users migrating from Terragrunt expect code generation, backend generation, and other familiar patterns.',
      progress: 60,
      status: 'in-progress',
      milestones: [
        { label: 'File-scoped locals', status: 'shipped', quarter: 'q4-2025', docs: '/stacks/locals', changelog: 'file-scoped-locals', description: 'Define local variables at the file level for DRY configuration—familiar to Terragrunt users.' },
        { label: 'Imperative stack names', status: 'shipped', quarter: 'q4-2025', docs: '/stacks/name', changelog: 'stack-manifest-name-override', description: 'Simple stack naming with a direct name field instead of complex name templates.' },
        { label: 'File generation (`generate` blocks)', status: 'shipped', quarter: 'q4-2025', description: 'Generate files like backend.tf and provider.tf from stack configuration with inheritance support.' },
        { label: 'Automatic backend provisioning', status: 'shipped', quarter: 'q4-2025', docs: '/components/terraform/backend-provisioning', description: 'Automatic backend.tf generation from stack configuration—no manual backend files needed.' },
        { label: 'AWS context YAML functions', status: 'shipped', quarter: 'q4-2025', docs: '/functions/yaml/aws.account-id', description: 'Access AWS caller identity in stack configuration.', codeExample: 'account_id: !aws.account-id' },
        { label: 'Provider auto-generation', status: 'planned', quarter: 'q1-2026', docs: '/components/terraform/providers', description: 'Generate provider.tf with proper credentials and region configuration from stack metadata.' },
        { label: 'Multi-stack formats', status: 'planned', quarter: 'q2-2026', description: 'Support for alternative stack formats including single-file stacks and Terragrunt-style layouts.' },
      ],
      issues: [],
      prs: [
        { number: 1878, title: 'Add generate section inheritance and auto-generation support' },
        { number: 1893, title: 'Add Terragrunt Support PRD' },
      ],
    },
    {
      id: 'quality',
      icon: 'RiShieldCheckLine',
      title: 'Code Quality and Community',
      tagline: 'Rigorous testing, open contribution',
      description:
        '2025 started at <20% test coverage and ended at ~74% — a 54% improvement. Embracing AI-assisted development while maintaining high standards.',
      progress: 85,
      status: 'in-progress',
      milestones: [
        { label: 'Test coverage from <20% to 74%', status: 'shipped', quarter: 'q1-2025', description: 'Massive test coverage improvement from less than 20% to 74%—a 54% increase in one year.' },
        { label: 'Changelog introduction', status: 'shipped', quarter: 'q1-2025', description: 'Detailed changelogs for every release with feature announcements and migration guides.' },
        { label: 'Weekly release cadence', status: 'shipped', quarter: 'q2-2025', description: 'Predictable weekly releases every Tuesday with semantic versioning.' },
        { label: 'Claude Code + Claude skills', status: 'shipped', quarter: 'q3-2025', description: 'AI-assisted development with custom Claude Code skills for Atmos-specific patterns.' },
        { label: 'CodeRabbit review integration', status: 'shipped', quarter: 'q3-2025', description: 'Automated code reviews with AI-powered suggestions and security analysis.' },
        { label: '80%+ test coverage', status: 'in-progress', quarter: 'q1-2026', description: 'Targeting 80%+ test coverage with focus on critical paths and edge cases.' },
      ],
      issues: [],
      prs: [],
    },
    {
      id: 'docs',
      icon: 'RiBookOpenLine',
      title: 'Documentation Overhaul',
      tagline: 'Every command, every config, cross-linked',
      description:
        'Comprehensive documentation of every `atmos.yaml` section, every CLI command, with cross-linking between commands and their configurations.',
      progress: 95,
      status: 'in-progress',
      milestones: [
        { label: 'Every `atmos.yaml` section documented', status: 'shipped', quarter: 'q3-2025', docs: '/cli/configuration/configuration', description: 'Complete reference for every configuration option in atmos.yaml with examples and defaults.' },
        { label: 'Every CLI command documented', status: 'shipped', quarter: 'q3-2025', docs: '/cli/commands/commands', description: 'Comprehensive documentation for every CLI command with usage examples and screenshots.' },
        { label: 'Cross-linking commands to configs', status: 'shipped', quarter: 'q3-2025', docs: '/cli/commands/commands', description: 'Navigate from any command to its related configuration and vice versa.' },
        { label: 'Design patterns refresh', status: 'shipped', quarter: 'q3-2025', docs: '/design-patterns', description: 'Updated design patterns with real-world examples for common infrastructure scenarios.' },
        { label: 'Versioning strategy docs', status: 'shipped', quarter: 'q3-2025', docs: '/design-patterns/version-management', changelog: 'comprehensive-version-management-documentation', description: 'Complete guide to version management including component versioning and upgrade strategies.' },
        { label: 'New learning section', status: 'shipped', quarter: 'q4-2025', docs: '/learn/concepts-overview', changelog: 'documentation-reorganization', description: 'Step-by-step tutorials for getting started with Atmos from scratch.' },
        { label: 'Migration guides (Terragrunt, Workspaces, Native Terraform)', status: 'shipped', quarter: 'q4-2025', docs: '/migration/terragrunt', changelog: 'migration-guides', description: 'Detailed guides for migrating from Terragrunt, Terraform workspaces, and native Terraform to Atmos.' },
        { label: 'Roadmap', status: 'shipped', quarter: 'q4-2025', docs: '/roadmap', description: 'Public roadmap showing past accomplishments and future plans.' },
      ],
      issues: [],
      prs: [],
    },
  ],

  highlights: [
    {
      id: 'test-coverage',
      label: 'Test Coverage',
      before: '<20%',
      after: '74%',
      icon: 'RiTestTubeLine',
      description: '54% improvement in 2025',
    },
    {
      id: 'release-cadence',
      label: 'Release Cadence',
      before: 'Per PR',
      after: 'Weekly',
      icon: 'RiCalendarLine',
      description: 'Predictable release schedule',
    },
    {
      id: 'tools-replaced',
      label: 'Third-party Tools',
      before: 'Dozens',
      after: 'Minimal',
      icon: 'RiStackLine',
      description: 'Consolidated toolchain',
    },
  ],
};

export default roadmapConfig;
