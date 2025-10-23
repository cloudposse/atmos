import React from 'react';
import { motion } from 'framer-motion';
import Layout from '@theme/Layout';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Link from '@docusaurus/Link';
import Screengrab from '@site/src/components/Screengrab'
import TypingAnimation from '@site/src/components/TypingAnimation'
import LazyDemo from '@site/src/components/LazyDemo'
import ScrollFadeIn from '@site/src/components/ScrollFadeIn'
import { RiLockLine, RiBox3Line, RiFlashlightLine, RiStackLine } from 'react-icons/ri';
import '../css/landing-page.css';

function Home() {
  const context = useDocusaurusContext();
  const {siteConfig = {}} = context;

  return (
    <div className="landing-page">
      <Layout title={`Hello from ${siteConfig.title}`} description="Atmos: Sanity for the Modern Platform Engineer - An IaC Framework that unifies your toolchain">
        <header className="hero hero--full-height">
          <div className="intro">
            <p className="hero__eyebrow">Infrastructure as Code Framework</p>
            <h1>One Tool to Orchestrate <span className="typing-container"><TypingAnimation words={['Terraform', 'OpenTofu', 'Packer', 'Helmfile', 'and more...']} /></span></h1>
            <p className="hero__description">Unified workflows, authentication, and vendoring for all your IaC tools</p>
            <div className="hero__cta">
              <Link to="/quick-start/" className="button button--lg button--primary"><p>Try the Quick Start</p></Link>
              <Link to="/introduction" className="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </div>
          <ScrollFadeIn className="hero-demo-intro">
            <h2>See Atmos in Action</h2>
            <p>Watch how Atmos simplifies infrastructure orchestration with an intuitive workflow</p>
          </ScrollFadeIn>
        </header>
        <section className="hero-demo">
          <LazyDemo />
        </section>
        <main>
          <section className="features-grid">
            <motion.div
              initial={{ opacity: 0, y: 30 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true, margin: "-100px" }}
              transition={{ duration: 0.5, delay: 0, ease: "easeOut" }}
            >
              <div className="feature-card">
                <div className="feature-header">
                  <div className="feature-icon"><RiLockLine /></div>
                  <h3>Unified Authentication</h3>
                </div>
                <p>Replace a dozen auth tools with one consistent identity layer</p>
              </div>
            </motion.div>
            <motion.div
              initial={{ opacity: 0, y: 30 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true, margin: "-100px" }}
              transition={{ duration: 0.5, delay: 0.1, ease: "easeOut" }}
            >
              <div className="feature-card">
                <div className="feature-header">
                  <div className="feature-icon"><RiBox3Line /></div>
                  <h3>Built-in Vendoring</h3>
                </div>
                <p>Purpose-built engine for Terraform and all your dependencies</p>
              </div>
            </motion.div>
            <motion.div
              initial={{ opacity: 0, y: 30 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true, margin: "-100px" }}
              transition={{ duration: 0.5, delay: 0.2, ease: "easeOut" }}
            >
              <div className="feature-card">
                <div className="feature-header">
                  <div className="feature-icon"><RiFlashlightLine /></div>
                  <h3>Workflow Automation</h3>
                </div>
                <p>Native task runner with built-in identity and context</p>
              </div>
            </motion.div>
            <motion.div
              initial={{ opacity: 0, y: 30 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true, margin: "-100px" }}
              transition={{ duration: 0.5, delay: 0.3, ease: "easeOut" }}
            >
              <div className="feature-card">
                <div className="feature-header">
                  <div className="feature-icon"><RiStackLine /></div>
                  <h3>Smart Scaffolding</h3>
                </div>
                <p>Configuration inheritance and composable stacks that scale</p>
              </div>
            </motion.div>
          </section>
          <section className="alternate-section section--image-right">
            <Screengrab title="Start your Project" command="# here's an example of what your folder structure will like..." slug="demo-stacks/start-your-project" />
            <div className="section__description">
              <h2>Start Your Project</h2>
              <p>Create a solid foundation with a well-structured folder layout, embracing best practices and conventions for a consistently organized project.</p>
              <Link to="/core-concepts/stacks/catalogs" className="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
          <section className="alternate-section section--image-left">
            <Screengrab title="Write your Components" command="# Then write your terraform root modules..." slug="demo-stacks/write-your-components" />
            <div className="section__description">
              <h2>Write your Components</h2>
              <p>Use your existing Terraform root modules or create new ones. Component libraries make sharing easy.
                 Use vendoring to pull down remote dependencies.</p>
              <Link to="/core-concepts/components" className="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
          <section className="alternate-section section--image-right">
            <Screengrab title="Define your Stacks" command="# Configure your stacks using YAML... easily import and inherit settings" slug="demo-stacks/define-your-stacks" />
            <div className="section__description">
              <h2>Define your Stacks</h2>
              <p>Configure your environments—development, staging, production—each tailored to different stages of the lifecycle, ensuring smooth transitions and robust deployment strategies.
                 Inherit from a common baseline to keep it DRY.</p>
              <Link to="/core-concepts/stacks" className="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
          <section className="alternate-section section--image-left">
            <Screengrab title="Atmos Stacks" command="# Deploy your stacks with the console UI or using GitHub Actions" slug="demo-stacks/deploy" />
            <div className="section__description">
              <h2>Deploy 🚀</h2>
              <p>Execute deployments with precision using Terraform's plan and apply commands, fully integrated with native GitOps workflows through GitHub Actions for seamless automation.</p>
              <Link to="/cli/commands/terraform/usage" className="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
        </main>
        <footer>
            <h2>What are you waiting for? <strong className="atmos__text">It's FREE and Open Source</strong></h2>
            <h3><strong className="underline">Your team can succeed</strong> with Terraform/OpenTofu and Packer today.</h3>
            <section className="cta-section">
              <Link to="/quick-start/" className="button button--lg button--primary"><p>Try the Quick Start</p></Link>
            </section>
        </footer>
      </Layout>
    </div>
  );
}

export default Home;
