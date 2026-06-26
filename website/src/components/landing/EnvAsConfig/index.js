import React from 'react';
import { motion } from 'framer-motion';
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

// A real stack manifest: import a baseline, then vary one value per environment.
const STACK_YAML = `import:
  - catalog/vpc          # shared baseline, defined once

vars:
  stage: prod
  region: us-east-2

components:
  terraform:
    vpc:
      vars:
        cidr_block: 10.100.0.0/16   # the only thing that changes
`;

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
        <span className="lp-eyebrow">Environments as configuration</span>
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
          Reuse one root module across dev, staging, and prod; vary only what
          changes per environment, and they stay in sync because they share the
          same source.
        </p>
      </motion.div>
      <motion.div
        className="lp-workload-visual"
        initial={{ opacity: 0, y: 24 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.25 }}
        transition={{ duration: 0.6, delay: 0.1, ease: 'easeOut' }}
      >
        <Terminal title="stacks/orgs/acme/plat/prod.yaml">
          <CodeBlock language="yaml">{STACK_YAML}</CodeBlock>
        </Terminal>
      </motion.div>
    </section>
  );
}

export default EnvAsConfig;
