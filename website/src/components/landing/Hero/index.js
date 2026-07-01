import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import { SiAmazonwebservices, SiGooglecloud } from 'react-icons/si';
import { VscAzure } from 'react-icons/vsc';
import PrimaryCTA from '@site/src/components/PrimaryCTA';
import DemoVideo from '@site/src/components/landing/DemoVideo';

// Three-line monospace value prop, Vercel-style.
const VALUE_PROPS = ['Run it on your laptop', 'Run it the same in CI', 'Run it with agents'];

// The workloads the runtime runs — concrete tools so it reads as real, not vaporware.
// `to` links the tool to its doc page where one exists; tools without a page stay plain text.
const TOOLS = [
  { label: 'Terraform', to: '/components/terraform' },
  { label: 'OpenTofu', to: '/components/terraform' },
  { label: 'Packer', to: '/components/packer' },
  { label: 'Ansible', to: '/components/ansible' },
  { label: 'Helmfile', to: '/components/helmfile' },
  { label: 'Helm', to: '/components/helm' },
  { label: 'Kubernetes', to: '/components/kubernetes' },
  { label: 'Containers', to: '/components/container' },
  { label: 'Emulators', to: '/components/emulator' },
  { label: 'Bring your own', to: '/components/custom' },
];

function Hero() {
  return (
    <header className="lp-hero">
      <div className="lp-hero-inner">
        <div className="lp-hero-grid">
          <motion.div
            className="lp-hero-copy"
            initial={{ opacity: 0, y: 24 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, ease: 'easeOut' }}
          >
            <span className="lp-eyebrow">Declarative Infrastructure Runtime</span>
            <h1>Run your infrastructure anywhere.</h1>
            <p className="lp-hero-sub">
              Atmos is the open-source runtime that builds, authenticates, and ships Terraform,
              Kubernetes, and containers — the same way everywhere. Auth, secrets, vendoring, and CI
              are built in. Stop stringing together 25 tools.
            </p>
            {/* Value props and the cloud logos share one row: the "run it…" lines on
                the left, the clouds we run on the right. */}
            <div className="lp-hero-runit-row">
              <div className="lp-hero-valueprops">
                {VALUE_PROPS.map((prop) => (
                  <span key={prop}>{prop}</span>
                ))}
              </div>
              <Link
                to="/multi-cloud"
                className="hero__cloud-logos lp-hero-clouds"
                aria-label="Multi-cloud: AWS, Azure, GCP"
              >
                <div className="hero__cloud-logo"><SiAmazonwebservices /></div>
                <div className="hero__cloud-logo"><VscAzure /></div>
                <div className="hero__cloud-logo"><SiGooglecloud /></div>
              </Link>
            </div>
            <div className="lp-hero-cta">
              <PrimaryCTA to="/install">Install Atmos</PrimaryCTA>
              <Link to="/intro" className="lp-hero-link">Read the docs →</Link>
            </div>
          </motion.div>
          <motion.div
            className="lp-hero-visual"
            initial={{ opacity: 0, y: 24 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7, delay: 0.15, ease: 'easeOut' }}
          >
            <DemoVideo title="Discover your whole platform" slug="hero" />
          </motion.div>
        </div>
        {/* Full-width band below the hero grid so the tools have real horizontal
            room and read as one comfortable row. */}
        <div className="lp-runs">
          <div className="lp-runs-label">Runs everything</div>
          <div className="lp-runs-content">
            <div className="lp-runs-row">
              {TOOLS.map((item, i) => (
                <React.Fragment key={item.label}>
                  {item.to ? (
                    <Link to={item.to} className="lp-runs-link">{item.label}</Link>
                  ) : (
                    <span>{item.label}</span>
                  )}
                  {i < TOOLS.length - 1 && <span className="lp-runs-dot" aria-hidden="true">·</span>}
                </React.Fragment>
              ))}
            </div>
          </div>
        </div>
      </div>
    </header>
  );
}

export default Hero;
