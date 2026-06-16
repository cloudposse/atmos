/**
 * Curated product announcements displayed in the announcement bar.
 * Ordered by priority -- first non-dismissed announcement is shown.
 * Each entry needs a unique, stable ID for localStorage tracking.
 *
 * After a user dismisses an announcement, the bar stays hidden for
 * `dismissCooldownMs` before the next announcement appears.
 *
 * Styling: announcements intentionally do NOT set per-entry colors. Every
 * announcement inherits the consistent brand-blue bar styling from the
 * `--announcement-bar-background` / `--announcement-bar-text-color` CSS
 * variables (see website/src/css/custom.css). Keep it that way -- a single
 * on-brand color reads as sophisticated on the dark site, whereas per-entry
 * colors turn the bar into a rainbow.
 */

/** How long the bar stays hidden after a dismissal (default: 3 days). */
export const dismissCooldownMs = 3 * 24 * 60 * 60 * 1000;

export const announcements = [
  {
    id: 'refarch-2024',
    content:
      'Try Cloud Posse\'s <a href="https://docs.cloudposse.com">Reference Architecture</a> for AWS, Datadog & GitHub Actions using Atmos',
  },
  {
    id: 'atmos-pro-launch',
    content:
      'Introducing <a href="https://atmos.tools/pro">Atmos Pro</a> — visibility and governance for your infrastructure',
  },
  {
    id: 'native-ci-2024',
    content:
      'New: <a href="/blog/native-ci-integration">Native CI/CD integration</a> for Terraform plan/apply lifecycle',
  },
  {
    id: 'atmos-ai',
    content:
      'New: <a href="/cli/commands/ai">Native AI support</a> in Atmos — intelligent infrastructure assistance built right into the CLI',
  },
  {
    id: 'atmos-toolchain',
    content:
      'Easily install Terraform, OpenTofu, and all your other tools with one command using <a href="/cli/commands/toolchain">Atmos Toolchain</a>',
  },
  {
    id: 'atmos-auth',
    content:
      'Simplify cloud authentication with <a href="/cli/commands/auth">Atmos Auth</a> — unified identity management across providers',
  },
  {
    id: 'atmos-codegen',
    content:
      'Automatically generate Terraform or any configuration with <a href="/templates">Atmos code generation</a> — no more boilerplate',
  },
  {
    id: 'atmos-component-provisioning',
    content:
      'Deploy any Terraform root module from any source — no vendoring required. Learn about <a href="/components">Atmos component provisioning</a>',
  },
  {
    id: 'atmos-locals',
    content:
      'Use <a href="/stacks/locals">locals</a> to define computed values in your stack configurations — DRY up your YAML',
  },
  {
    id: 'atmos-backends',
    content:
      'Dynamically generate and manage Terraform <a href="/stacks/components/terraform/backend">backend configurations</a> across all your environments',
  },
  {
    id: 'atmos-oidc',
    content:
      'Did you know Atmos handles <a href="/cli/commands/auth">GitHub OIDC</a>? Authenticate to AWS without long-lived credentials',
  },
];
