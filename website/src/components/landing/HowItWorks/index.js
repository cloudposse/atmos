import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import Screengrab from '@site/src/components/Screengrab';

const STEPS = [
  {
    title: 'Install Atmos',
    desc: 'One binary, no dependencies. It even installs the right Terraform, OpenTofu, and Helmfile versions for you.',
  },
  {
    title: 'Configure with YAML',
    desc: 'Describe your environments as configuration. Inherit a common baseline and keep everything DRY.',
  },
  {
    title: 'Run it anywhere',
    desc: 'The same command builds, authenticates, and ships — on your laptop and in CI, identically.',
  },
];

const LINKS = [
  { label: 'Quickstart →', to: '/intro' },
  { label: 'Browse commands →', to: '/cli/commands/terraform/usage' },
  { label: 'Design patterns →', to: '/design-patterns' },
];

function HowItWorks() {
  return (
    <section className="lp-section">
      <div className="lp-section-head">
        <span className="lp-eyebrow">From zero to deployed</span>
        <h2>Ship anything with Atmos</h2>
        <p>Three steps from an empty directory to infrastructure running in production.</p>
      </div>
      <div className="lp-how-grid">
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.3 }}
          transition={{ duration: 0.5, ease: 'easeOut' }}
        >
          <ol className="lp-steps">
            {STEPS.map((step, i) => (
              <li className="lp-step" key={step.title}>
                <span className="lp-step-num">0{i + 1}</span>
                <div>
                  <h3>{step.title}</h3>
                  <p>{step.desc}</p>
                </div>
              </li>
            ))}
          </ol>
          <div className="lp-how-cta">
            {LINKS.map((link) => (
              <Link key={link.to} to={link.to}>{link.label}</Link>
            ))}
          </div>
        </motion.div>
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.3 }}
          transition={{ duration: 0.6, delay: 0.1, ease: 'easeOut' }}
        >
          <Screengrab title="atmos terraform deploy vpc -s dev" slug="demo-stacks/deploy-dev" />
        </motion.div>
      </div>
    </section>
  );
}

export default HowItWorks;
