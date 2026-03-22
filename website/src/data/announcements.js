/**
 * Curated product announcements displayed in the announcement bar.
 * Ordered by priority -- first non-dismissed announcement is shown.
 * Each entry needs a unique, stable ID for localStorage tracking.
 *
 * After a user dismisses an announcement, the bar stays hidden for
 * `dismissCooldownMs` before the next announcement appears.
 */

/** How long the bar stays hidden after a dismissal (default: 3 days). */
export const dismissCooldownMs = 3 * 24 * 60 * 60 * 1000;

export const announcements = [
  {
    id: 'refarch-2024',
    content:
      'Try Cloud Posse\'s <a href="https://docs.cloudposse.com">Reference Architecture</a> for AWS, Datadog & GitHub Actions using Atmos',
    backgroundColor: '#3578e5',
    textColor: '#fff',
  },
  {
    id: 'atmos-pro-launch',
    content:
      'Introducing <a href="https://atmos.tools/pro">Atmos Pro</a> \u2014 visibility and governance for your infrastructure',
    backgroundColor: '#7c3aed',
    textColor: '#fff',
  },
  {
    id: 'native-ci-2024',
    content:
      'New: <a href="/blog/native-ci-integration">Native CI/CD integration</a> for Terraform plan/apply lifecycle',
    backgroundColor: '#059669',
    textColor: '#fff',
  },
  {
    id: 'atmos-ai',
    content:
      'New: <a href="/cli/commands/ai">Native AI support</a> in Atmos \u2014 intelligent infrastructure assistance built right into the CLI',
    backgroundColor: '#0891b2',
    textColor: '#fff',
  },
  {
    id: 'atmos-toolchain',
    content:
      'Easily install Terraform, OpenTofu, and all your other tools with one command using <a href="/cli/commands/toolchain">Atmos Toolchain</a>',
    backgroundColor: '#d97706',
    textColor: '#fff',
  },
  {
    id: 'atmos-auth',
    content:
      'Simplify cloud authentication with <a href="/cli/commands/auth">Atmos Auth</a> \u2014 unified identity management across providers',
    backgroundColor: '#dc2626',
    textColor: '#fff',
  },
  {
    id: 'atmos-codegen',
    content:
      'Automatically generate Terraform or any configuration with <a href="/templates">Atmos code generation</a> \u2014 no more boilerplate',
    backgroundColor: '#4f46e5',
    textColor: '#fff',
  },
  {
    id: 'atmos-component-provisioning',
    content:
      'Deploy any Terraform root module from any source \u2014 no vendoring required. Learn about <a href="/components">Atmos component provisioning</a>',
    backgroundColor: '#0d9488',
    textColor: '#fff',
  },
  {
    id: 'atmos-locals',
    content:
      'Use <a href="/stacks/locals">locals</a> to define computed values in your stack configurations \u2014 DRY up your YAML',
    backgroundColor: '#7c3aed',
    textColor: '#fff',
  },
  {
    id: 'atmos-backends',
    content:
      'Dynamically generate and manage Terraform <a href="/stacks/components/terraform/backend">backend configurations</a> across all your environments',
    backgroundColor: '#b45309',
    textColor: '#fff',
  },
  {
    id: 'atmos-oidc',
    content:
      'Did you know Atmos handles <a href="/cli/commands/auth">GitHub OIDC</a>? Authenticate to AWS without long-lived credentials',
    backgroundColor: '#1d4ed8',
    textColor: '#fff',
  },
];
