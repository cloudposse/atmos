import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import {
  RiShieldKeyholeLine,
  RiKey2Line,
  RiBox3Line,
  RiDatabase2Line,
  RiToolsLine,
  RiTerminalBoxLine,
  RiGitMergeLine,
  RiRobot2Line,
} from 'react-icons/ri';

// The runtime's built-ins — the "you don't need 25 tools" argument.
const BATTERIES = [
  {
    icon: RiShieldKeyholeLine,
    title: 'Unified Auth',
    desc: 'One identity layer across AWS, Azure, and GCP — SSO, OIDC, and federation. EKS and ECR login happen automatically.',
    tag: 'SSO · OIDC · EKS · ECR',
    to: '/cli/configuration/auth',
  },
  {
    icon: RiKey2Line,
    title: 'Secrets Management',
    desc: 'Declare secrets per environment, source them from 10+ backends, and mask them across every channel.',
    tag: '1PASSWORD · SSM · VAULT · SOPS',
    to: '/cli/configuration/secrets',
  },
  {
    icon: RiBox3Line,
    title: 'Vendoring',
    desc: 'Pull every dependency just-in-time with version pinning and retries. No separate vendor step.',
    tag: 'JIT · VERSION-PINNED',
    to: '/cli/configuration/vendor',
  },
  {
    icon: RiDatabase2Line,
    title: 'Caching & Mirroring',
    desc: 'A native build cache plus a transparent Terraform provider and module registry mirror — warm in CI, instant on your laptop.',
    tag: 'BUILD CACHE · REGISTRY MIRROR',
    to: '/cli/configuration/ci/cache',
  },
  {
    icon: RiToolsLine,
    title: 'Toolchain',
    desc: 'Auto-installs the exact Terraform, OpenTofu, and Helmfile versions your stacks need — verified by checksum.',
    tag: 'AUTO-INSTALL · VERIFIED',
    to: '/cli/configuration/toolchain',
  },
  {
    icon: RiTerminalBoxLine,
    title: 'Workflows & Automation',
    desc: 'Run, automate, and chain anything — 25+ step types and custom commands orchestrate tasks across every component.',
    tag: '25+ STEP TYPES · CUSTOM COMMANDS',
    to: '/workflows',
  },
  {
    icon: RiGitMergeLine,
    title: 'GitOps & CI/CD',
    desc: 'Managed repositories, signed commits, and the same commands locally and in CI. Detect affected components, emit matrices, and catch drift.',
    tag: 'GITOPS · DESCRIBE AFFECTED · DRIFT',
    to: '/ci',
  },
  {
    icon: RiRobot2Line,
    title: 'AI + MCP',
    desc: 'Chat about your infrastructure, run 21+ skills, expose Atmos as an MCP server, or add --ai to any command.',
    tag: 'CHAT · SKILLS · MCP',
    to: '/ai',
  },
];

function Batteries() {
  return (
    <section className="lp-section">
      <div className="lp-section-head">
        <span className="lp-eyebrow">Batteries included</span>
        <h2>Everything you'd otherwise bolt on</h2>
        <p>Auth, secrets, vendoring, caching, toolchain, workflows, CI, and AI are part of the runtime — not a pile of plugins you wire together.</p>
      </div>
      <div className="lp-batteries-grid">
        {BATTERIES.map((b, i) => (
          <motion.div
            key={b.title}
            initial={{ opacity: 0, y: 20 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true, amount: 0.2 }}
            transition={{ duration: 0.45, delay: (i % 3) * 0.08, ease: 'easeOut' }}
          >
            <Link to={b.to} className="lp-battery">
              <div className="lp-battery-icon"><b.icon /></div>
              <h3>{b.title}</h3>
              <p>{b.desc}</p>
              <div className="lp-battery-tag">{b.tag}</div>
            </Link>
          </motion.div>
        ))}
      </div>
    </section>
  );
}

export default Batteries;
