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
      <Layout title={`Hello from ${siteConfig.title}`} description="Manage Environments Easily in Terraform, OpenTofu, Packer using Atmos">
        <header className="hero hero--full-height">
          <div className="intro">
            <h1>Manage Environments Easily<br/>in <TypingAnimation words={['Terraform', 'OpenTofu', 'Packer', 'Helmfile', 'and more...']} /></h1>
          </div>
          <img src={useBaseUrl('/img/demo.gif')} alt="Product Screenshot" className="screenshot" />
          <div className="hero__cta">
            <Link to="/quick-start/" className="button button--lg button--primary"><p>Try the Quick Start</p></Link>
            <Link to="/introduction" className="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
          </div>
          <h3>Use a Modern <strong className="underline">Composable Framework</strong> for Terraform, OpenTofu, Packer and Helmfile backed by YAML</h3>
        </header>
        <main>
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
