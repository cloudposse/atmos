import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import DemoVideo from '@site/src/components/landing/DemoVideo';

const STEPS = [
  {
    title: 'Model your platform',
    desc: 'Stacks, components, identities, secrets, and stores live in one declarative graph.',
  },
  {
    title: 'Authenticate once',
    desc: 'Atmos identities feed Terraform, stores, emulators, and CI without bespoke wrapper scripts.',
  },
  {
    title: 'Run it anywhere',
    desc: 'The same commands build, authenticate, and ship on your laptop and in CI, identically.',
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
        <p>Three steps from config to authenticated infrastructure workflows.</p>
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
          <DemoVideo title="Config in, infrastructure out" slug="how-it-works" />
        </motion.div>
      </div>
    </section>
  );
}

export default HowItWorks;
