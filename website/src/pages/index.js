import React from 'react';
import Layout from '@theme/Layout';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl'
import Screengrab from '@site/src/components/Screengrab'
import TypingAnimation from '@site/src/components/TypingAnimation'
import '../css/landing-page.css';

function Home() {
  const context = useDocusaurusContext();
  const {siteConfig = {}} = context;

  return (
    <div className="landing-page">
      <Layout title={`Hello from ${siteConfig.title}`} description="Atmos: Sanity for the Modern Platform Engineer - An IaC Framework that unifies your toolchain">
        <header className="hero hero--full-height">
          <div className="intro">
            <p className="hero__subtitle">Atmos is an IaC Framework</p>
            <h1>Manage Environments Easily<br/>in <TypingAnimation words={['Terraform', 'OpenTofu', 'Packer', 'Helmfile', 'and more...']} /></h1>
            <h2 className="hero__tagline">Atmos: <strong className="underline">Sanity for the Modern Platform Engineer</strong></h2>
          </div>
          <img src={useBaseUrl('/img/demo.gif')} alt="Product Screenshot" className="screenshot" />
          <div className="hero__cta">
            <Link to="/quick-start/" className="button button--lg button--primary"><p>Try the Quick Start</p></Link>
            <Link to="/introduction" className="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
          </div>
        </header>
        <main>
          <section className="hero-narrative">
            <div className="narrative-content">
              <p className="narrative-intro">Developers complain about tool fatigue — and they're right. Every project turns into a patchwork of tools duct-taped together with fragile Bash scripts, no tests, and inconsistent conventions.</p>

              <p className="narrative-solution"><strong>Atmos fixes that.</strong></p>

              <p>Atmos replaces the chaos with a cohesive, comprehensive framework that unifies your toolchain — combining the power of 25 different tools into one system that doesn't lock you in. (You do that to yourself. Kidding… mostly.)</p>

              <h3>With Atmos, you get:</h3>
              <ul className="feature-list">
                <li><strong>Vendoring built in.</strong> It combines the best of Vendyr by Caraval into a purpose-built vendoring engine for Terraform and all your dependencies.</li>
                <li><strong>Authentication reimagined.</strong> Replace a dozen tools — Granted, AWS Vault, aws-saml, saml2aws, and more — with a single consistent identity layer.</li>
                <li><strong>Workflow automation.</strong> Skip GoTask and Makefiles. Atmos includes a native task runner with built-in identity and context.</li>
                <li><strong>Templating & scaffolding.</strong> Configuration inheritance, composable stacks, and project scaffolding that scale with you.</li>
                <li><strong>Multi-identity support.</strong> Manage so many identities you might start to wonder if you have multiple personality disorder (we call it multi-account mastery).</li>
              </ul>

              <p>Atmos gives you repeatable processes, best practices, and proven design patterns — so your whole team can finally get back on the same page and stop reinventing the wheel.</p>

              <p className="narrative-closer"><strong>Atmos doesn't just reduce tool fatigue — it cures tool insanity.</strong></p>

              <p className="narrative-final">It makes you wonder why you didn't start this way from the beginning.</p>
            </div>
          </section>
          <h2 className="section">Simplify complex architectures with <strong className="atmos__text">DRY configuration</strong></h2>
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
