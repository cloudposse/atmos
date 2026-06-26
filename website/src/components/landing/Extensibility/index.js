import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import Screengrab from '@site/src/components/Screengrab';

const EXTENSIONS = [
  { title: 'Custom commands', desc: 'Wrap any script as a first-class atmos command, with flags, args, and identity.', to: '/quick-start/advanced/add-custom-commands' },
  { title: 'Component types', desc: 'Register your own component kinds via the same registry the built-ins use.', to: '/components' },
  { title: 'YAML functions', desc: 'Resolve state, outputs, secrets, and Git metadata right inside your config.', to: '/functions/yaml' },
  { title: 'Hooks', desc: 'Run infracost, checkov, trivy, or any command on lifecycle events.', to: '/stacks/hooks' },
  { title: 'MCP & skills', desc: 'Connect Atmos to any agent, or publish reusable skills your team can install.', to: '/ai/mcp-server' },
  { title: 'Stores', desc: 'Plug in SSM, Secrets Manager, Key Vault, Vault, Redis, and more for cross-component data.', to: '/cli/configuration/stores' },
  { title: 'Validation', desc: 'Enforce your own guardrails with OPA/Rego policies and JSON Schema.', to: '/validation/validating' },
  { title: 'Templates & data sources', desc: 'Pull live data into your config with Go templates and Gomplate datasources.', to: '/templates' },
];

function Extensibility() {
  return (
    <section className="lp-section lp-extend">
      <div className="lp-section-head">
        <span className="lp-eyebrow">Extensible by design</span>
        <h2>Built by you, or your agents</h2>
        <p>Everything is pluggable. Add a command, a component type, a store, or a skill — and your agents can use it the moment it exists.</p>
      </div>
      <motion.div
        className="lp-extend-snippet"
        initial={{ opacity: 0, y: 20 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.3 }}
        transition={{ duration: 0.5, ease: 'easeOut' }}
      >
        <Screengrab title="atmos workflow run deploy-all -s prod" slug="demo-stacks/deploy-dev" />
      </motion.div>
      <div className="lp-extend-grid">
        {EXTENSIONS.map((ext, i) => (
          <motion.div
            key={ext.title}
            initial={{ opacity: 0, y: 16 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true, amount: 0.2 }}
            transition={{ duration: 0.4, delay: (i % 3) * 0.06, ease: 'easeOut' }}
          >
            <Link to={ext.to} className="lp-extend-card">
              <h3>{ext.title}</h3>
              <p>{ext.desc}</p>
            </Link>
          </motion.div>
        ))}
      </div>
    </section>
  );
}

export default Extensibility;
