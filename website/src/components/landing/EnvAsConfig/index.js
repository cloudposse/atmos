import React, { useEffect, useState } from 'react';
import { motion, useReducedMotion } from 'framer-motion';
import CodeBlock from '@theme/CodeBlock';
import Terminal from '@site/src/components/Terminal';

// The configuration-as-code thesis: environments are just YAML, inherited and
// kept DRY — the setup for the "Batteries" (one tool, not 25) payoff that follows.
const FEATURES = [
  'Reusable root modules',
  'Inherit a shared baseline',
  'Override only what changes',
  'No glue scripts',
  'DRY by default',
];

// Everything in Atmos is configuration — not just environments. The carousel
// rotates through real config domains (each snippet is lifted from an actual
// example/doc in this repo) so the "just config" claim is shown, not asserted.
const EXAMPLES = [
  {
    id: 'environments',
    label: 'Environments',
    file: 'stacks/orgs/acme/plat/prod.yaml',
    // A real stack manifest: import a baseline, then vary one value per environment.
    yaml: `import:
  - catalog/vpc          # shared baseline, defined once

vars:
  stage: prod
  region: us-east-2

components:
  terraform:
    vpc:
      vars:
        cidr_block: 10.100.0.0/16   # the only thing that changes
`,
  },
  {
    id: 'auth',
    label: 'Auth',
    file: 'atmos.yaml',
    // Identity is config too: declare a provider and the role it assumes.
    yaml: `auth:
  providers:
    acme-sso:
      kind: aws/iam-identity-center
      start_url: https://acme.awsapps.com/start
      region: us-east-1

  identities:
    prod-admin:
      kind: aws/assume-role
      via:
        provider: acme-sso
      principal:
        assume_role: arn:aws:iam::111111111111:role/ProdAdmin
`,
  },
  {
    id: 'secrets',
    label: 'Secrets',
    file: 'stacks/catalog/app.yaml',
    // Secrets are config too: declare what you need and name the backend —
    // Vault, SOPS, SSM — then consume with !secret.
    yaml: `components:
  terraform:
    app:
      secrets:
        vars:
          DB_PASSWORD:
            store: prod/vault        # HashiCorp Vault
            required: true
          DATADOG_API_KEY:
            sops: dev-sops           # SOPS-encrypted, committed to Git
      vars:
        db_password:     !secret DB_PASSWORD
        datadog_api_key: !secret DATADOG_API_KEY
`,
  },
  {
    id: 'workflows',
    label: 'Workflows',
    file: 'stacks/workflows/deploy.yaml',
    // Orchestration is config: gather input interactively, then act on it —
    // something a flag can't do.
    yaml: `workflows:
  deploy:
    description: Pick an environment, add a note, then ship it
    steps:
      - name: env
        type: choose                 # interactive prompt
        prompt: Which environment?
        options: [dev, staging, prod]
      - name: note
        type: input
        prompt: Deployment note
        placeholder: e.g. JIRA-123
      - command: terraform apply vpc -s plat-ue2-{{ .steps.env.value }}
      - type: toast
        level: success
        content: "Deployed to {{ .steps.env.value }} — {{ .steps.note.value }}"
`,
  },
  {
    id: 'vendoring',
    label: 'Vendoring',
    file: 'vendor.yaml',
    // Dependencies are config: pin a source and version, vendor it locally.
    yaml: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: vendor

spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc
      version: 1.398.0
      targets:
        - components/terraform/vpc
`,
  },
  {
    id: 'toolchain',
    label: 'Toolchain',
    file: 'stacks/catalog/vpc/defaults.yaml',
    // Tooling is config too: pin the exact tool versions a component depends on,
    // right in the stack — Atmos resolves and installs them before it runs.
    yaml: `components:
  terraform:
    vpc:
      dependencies:
        tools:
          terraform: 1.10.3       # exact version
          tflint: ^0.54.0         # semver constraint
          checkov: latest         # always newest
      vars:
        cidr_block: 10.0.0.0/16
`,
  },
  {
    id: 'dependencies',
    label: 'Deps',
    file: 'stacks/catalog/app/defaults.yaml',
    // Ordering is config: declare what a component depends on; Atmos builds the DAG.
    yaml: `components:
  terraform:
    app:
      dependencies:
        components:
          - name: vpc                 # deploy after vpc, same stack
          - name: rds
            stack: acme-ue1-prod      # or a component in another stack
        files:
          - configs/app.json          # re-deploy when this file changes
`,
  },
];

const ROTATE_MS = 5000;

function ConfigCarousel() {
  const reduce = useReducedMotion();
  const [active, setActive] = useState(0);
  const [paused, setPaused] = useState(false);

  // Auto-advance through the domains, but only when motion is allowed and the
  // user isn't actively reading (hover/focus pauses). Re-armed whenever `active`
  // changes so each slide gets a full dwell after a manual tab click.
  useEffect(() => {
    if (reduce || paused) return undefined;

    const timer = setTimeout(() => {
      setActive((i) => (i + 1) % EXAMPLES.length);
    }, ROTATE_MS);

    return () => clearTimeout(timer);
  }, [active, paused, reduce]);

  const current = EXAMPLES[active];

  return (
    <div
      className="lp-config-carousel"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <div className="lp-config-tabs" role="tablist" aria-label="Configuration domains">
        {EXAMPLES.map((ex, i) => (
          <button
            key={ex.id}
            type="button"
            role="tab"
            aria-selected={i === active}
            className={i === active ? 'is-active' : ''}
            onClick={() => setActive(i)}
          >
            {ex.label}
          </button>
        ))}
      </div>
      <Terminal title={current.file}>
        {/* All snippets are stacked in one grid cell (see .lp-config-stack) so the
            terminal is always as tall as the tallest snippet. Switching tabs
            crossfades in place instead of resizing the window — a height change
            re-centered the row and made the tabs jump. */}
        <div className="lp-config-stack">
          {EXAMPLES.map((ex, i) => (
            <motion.div
              key={ex.id}
              className="lp-config-slide"
              aria-hidden={i !== active}
              animate={{ opacity: i === active ? 1 : 0 }}
              transition={{ duration: reduce ? 0 : 0.25, ease: 'easeOut' }}
            >
              <CodeBlock language="yaml">{ex.yaml}</CodeBlock>
            </motion.div>
          ))}
        </div>
      </Terminal>
    </div>
  );
}

function EnvAsConfig() {
  return (
    <section className="lp-workload lp-config">
      <motion.div
        className="lp-workload-copy"
        initial={{ opacity: 0, y: 24 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.25 }}
        transition={{ duration: 0.5, ease: 'easeOut' }}
      >
        <span className="lp-eyebrow">Everything is configuration</span>
        <h2>Your environments are just configuration.</h2>
        <p>
          Point every environment at the same reusable Terraform root modules and
          treat the rest as configuration — eliminating code duplication, custom
          bash scripts, and complicated tooling with one tool to rule them all.
        </p>
        <ul className="lp-mono-list">
          {FEATURES.map((f) => (
            <li key={f}>{f}</li>
          ))}
        </ul>
        <p className="lp-workload-proof">
          And it isn't just environments — auth, secrets, workflows, and vendored
          dependencies are all declared the same way, so the entire platform stays
          readable, reviewable, and in sync.
        </p>
      </motion.div>
      <motion.div
        className="lp-workload-visual"
        initial={{ opacity: 0, y: 24 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.25 }}
        transition={{ duration: 0.6, delay: 0.1, ease: 'easeOut' }}
      >
        <ConfigCarousel />
      </motion.div>
    </section>
  );
}

export default EnvAsConfig;
