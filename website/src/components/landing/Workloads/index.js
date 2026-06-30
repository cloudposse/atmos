import React, { useRef } from 'react';
import { motion, useScroll, useTransform, useReducedMotion } from 'framer-motion';
import { SiGithubactions } from 'react-icons/si';
import DemoVideo from '@site/src/components/landing/DemoVideo';

// Job-to-be-done sections — the heart of the page. Each is a real job, proven
// with a terminal demo, a monospace feature list, and a product-proof line.
// `features` entries may be { label, isNew } to render a NEW badge, or
// { label, isPro } to render a PRO badge that links to Atmos Pro.
// `icon` renders a small accent glyph next to the eyebrow.
const WORKLOADS = [
  {
    eyebrow: 'Terraform & OpenTofu',
    title: 'Run Terraform like a platform team.',
    desc: 'Plan and apply across every component in dependency order, with bounded concurrency. Backends and providers are generated for you. Drift is caught automatically.',
    features: ['Dependency graph', 'Auto backends', 'Registry cache', { label: 'Drift detection', isPro: true }],
    proof: 'Atmos lists the Terraform instances and resolved stack values it will pass to Terraform.',
    slug: 'terraform',
    terminalTitle: 'Terraform, orchestrated',
  },
  {
    eyebrow: 'Kubernetes & Helm',
    title: 'Ship Kubernetes and Helm the same way.',
    desc: 'Helmfile is a first-class workload. Atmos models Kubernetes releases beside the rest of your stack, with the same CLI you already use for Terraform.',
    features: ['Helmfile native', 'Stack-aware releases', 'Toolchain aware'],
    proof: 'Atmos starts a local k3s sandbox off camera, installs declared Helm tooling, then deploys the release from the stack.',
    slug: 'kubernetes',
    terminalTitle: 'Helm, the platform way',
  },
  {
    eyebrow: 'Containers & Emulators',
    title: 'Build, run, and even emulate the cloud.',
    desc: 'Containers and dev containers are workloads too. Spin up cloud emulators locally so your whole stack runs on your laptop — no account required to iterate.',
    features: ['Container components', 'Dev containers', { label: 'Emulators', isNew: true }],
    proof: 'Develop against a local emulated cloud, then ship the identical config to prod.',
    slug: 'emulators',
    terminalTitle: 'A whole cloud on your laptop',
  },
  {
    eyebrow: 'Local = CI',
    title: 'Your laptop is the CI. CI is your laptop.',
    desc: 'Same command, same auth, same secrets, same toolchain — whether you run it locally or in a pipeline. And Atmos is git-aware: it detects what changed and plans or applies only the affected components, so CI does exactly the work that changed — nothing more.',
    features: ['Git-aware affected detection', 'Apply only what changed', 'Reusable across repos', 'Zero-config CI'],
    proof: 'What works on your machine works in CI — without any additional GitHub Actions or messy bash scripts, because it is literally the same runtime.',
    slug: 'local-ci',
    terminalTitle: 'atmos terraform apply --affected --ci',
    icon: SiGithubactions,
  },
  {
    eyebrow: 'Secrets & Stores',
    title: 'Manage secrets without glue code.',
    desc: 'Declare required secrets next to a component, initialize or rotate them through Atmos, and inject them into commands only when the component runs.',
    features: ['Declared secrets', 'Local and cloud stores', 'Masked reads', 'Runtime injection'],
    proof: 'Atmos initializes declared secrets, lists their status, rotates a value, and injects it into a component command without printing the secret.',
    slug: 'secrets',
    terminalTitle: 'Secrets without glue code',
  },
  {
    eyebrow: 'Developer experience',
    title: 'It tells you what to do next.',
    desc: 'Forget a flag and Atmos asks which stack and which component you meant. Every command follows the same verb-noun grammar, so the CLI stays discoverable as your platform grows.',
    features: ['Interactive prompts', 'Guided selection', 'Consistent CLI', 'Tab completion'],
    proof: 'A consistent, discoverable CLI that guides you instead of fighting you.',
    slug: 'dx',
    terminalTitle: 'Atmos asks what you meant',
  },
];

function FeatureList({ features }) {
  return (
    <ul className="lp-mono-list">
      {features.map((f) => {
        const label = typeof f === 'string' ? f : f.label;
        const isNew = typeof f === 'object' && f.isNew;
        const isPro = typeof f === 'object' && f.isPro;
        return (
          <li key={label}>
            {label}
            {isNew && <span className="lp-new">New</span>}
            {isPro && (
              <a
                className="lp-pro"
                href="https://atmos-pro.com"
                target="_blank"
                rel="noreferrer"
              >
                Pro
              </a>
            )}
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
        <DemoVideo title={w.terminalTitle} slug={w.slug} />
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
