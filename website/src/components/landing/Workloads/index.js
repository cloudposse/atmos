import React, { useRef } from 'react';
import { motion, useScroll, useTransform, useReducedMotion } from 'framer-motion';
import { SiGithubactions } from 'react-icons/si';
import Screengrab from '@site/src/components/Screengrab';

// Job-to-be-done sections — the heart of the page. Each is a real job, proven
// with a terminal demo, a monospace feature list, and a product-proof line.
// `features` entries may be { label, isNew } to render a NEW badge.
// `icon` renders a small accent glyph next to the eyebrow.
const WORKLOADS = [
  {
    eyebrow: 'Terraform & OpenTofu',
    title: 'Run Terraform like a platform team.',
    desc: 'Plan and apply across every component in dependency order, with bounded concurrency. Backends and providers are generated for you. Drift is caught automatically.',
    features: ['Dependency-aware apply', 'Auto backends', 'Registry cache', 'Drift detection'],
    proof: 'atmos terraform apply --all walks the graph so a VPC lands before the cluster that needs it.',
    slug: 'demo-stacks/deploy-prod',
    terminalTitle: 'atmos terraform apply --all -s plat-ue2-prod',
  },
  {
    eyebrow: 'Kubernetes & Helm',
    title: 'Ship Kubernetes and Helm the same way.',
    desc: 'Helmfile is a first-class workload. Atmos builds your kubeconfig and authenticates to EKS for you — the same CLI you already use for Terraform.',
    features: ['Helmfile native', 'Automatic EKS auth', 'Same CLI'],
    proof: 'No more juggling aws eks update-kubeconfig before every deploy.',
    slug: 'demo-stacks/deploy-staging',
    terminalTitle: 'atmos helmfile apply nginx -s dev',
  },
  {
    eyebrow: 'Containers & Emulators',
    title: 'Build, run, and even emulate the cloud.',
    desc: 'Containers and dev containers are workloads too. Spin up cloud emulators locally so your whole stack runs on your laptop — no account required to iterate.',
    features: ['Container components', 'Dev containers', { label: 'Emulators', isNew: true }],
    proof: 'Develop against a local emulated cloud, then ship the identical config to prod.',
    slug: 'demo-stacks/start-your-project',
    terminalTitle: 'atmos devcontainer up',
  },
  {
    eyebrow: 'Local = CI',
    title: 'Your laptop is the CI. CI is your laptop.',
    desc: 'Same command, same auth, same secrets, same toolchain — whether you run it locally or in a pipeline. And Atmos is git-aware: it detects what changed and plans or applies only the affected components, so CI does exactly the work that changed — nothing more.',
    features: ['Git-aware affected detection', 'Apply only what changed', 'Reusable across repos', 'Zero-config CI'],
    proof: 'What works on your machine works in CI — without any additional GitHub Actions or messy bash scripts, because it is literally the same runtime.',
    slug: 'demo-stacks/define-your-stacks',
    terminalTitle: 'atmos describe affected',
    icon: SiGithubactions,
  },
  {
    eyebrow: 'Developer experience',
    title: 'It tells you what to do next.',
    desc: 'Forget a flag and Atmos asks which stack and which component you meant. Hit an error and you get an actionable hint, not a stack trace. Every command follows the same verb-noun grammar.',
    features: ['Interactive prompts', 'Actionable hints', 'Consistent CLI', 'Tab completion'],
    proof: 'A consistent, discoverable CLI that guides you instead of fighting you.',
    slug: 'demo-stacks/write-your-components',
    terminalTitle: 'atmos terraform plan',
  },
];

function FeatureList({ features }) {
  return (
    <ul className="lp-mono-list">
      {features.map((f) => {
        const label = typeof f === 'string' ? f : f.label;
        const isNew = typeof f === 'object' && f.isNew;
        return (
          <li key={label}>
            {label}
            {isNew && <span className="lp-new">New</span>}
          </li>
        );
      })}
    </ul>
  );
}

// A single job-to-be-done section. The whole section fades to a dim resting
// state when it is away from the viewport center and rises to full opacity as it
// passes through the middle — so only the section you are reading is in focus.
// Subtle by design; disabled entirely when the user prefers reduced motion.
function WorkloadSection({ w, i }) {
  const ref = useRef(null);
  const reduceMotion = useReducedMotion();
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ['start end', 'end start'],
  });
  const focusedOpacity = useTransform(
    scrollYProgress,
    [0, 0.28, 0.5, 0.72, 1],
    [0.35, 1, 1, 1, 0.35],
  );
  const opacity = reduceMotion ? 1 : focusedOpacity;

  return (
    <motion.section
      ref={ref}
      style={{ opacity }}
      className={`lp-workload${i % 2 === 1 ? ' lp-workload--reverse' : ''}`}
    >
      <motion.div
        className="lp-workload-copy"
        initial={{ opacity: 0, y: 24 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.25 }}
        transition={{ duration: 0.5, ease: 'easeOut' }}
      >
        <span className="lp-eyebrow">
          {w.icon && <w.icon className="lp-eyebrow-icon" aria-hidden="true" />}
          {w.eyebrow}
        </span>
        <h2>{w.title}</h2>
        <p>{w.desc}</p>
        <FeatureList features={w.features} />
        <p className="lp-workload-proof">{w.proof}</p>
      </motion.div>
      <motion.div
        className="lp-workload-visual"
        initial={{ opacity: 0, y: 24 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.25 }}
        transition={{ duration: 0.6, delay: 0.1, ease: 'easeOut' }}
      >
        <Screengrab title={w.terminalTitle} slug={w.slug} />
      </motion.div>
    </motion.section>
  );
}

function Workloads() {
  return (
    <>
      {WORKLOADS.map((w, i) => (
        <WorkloadSection key={w.title} w={w} i={i} />
      ))}
    </>
  );
}

export default Workloads;
