import React from 'react';
import Layout from '@theme/Layout';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl'
import Screengrab from '@site/src/components/Screengrab'

function Home() {
  const context = useDocusaurusContext();
  const {siteConfig = {}} = context;

  return (
    <div class="landing-page">
      <Layout title={`Hello from ${siteConfig.title}`} description="Manage Environments Easily in Terraform using Atmos">
        <header class="hero hero--full-height">
          <div class="intro">
            <h1>Manage Environments Easily in Terraform</h1>
          </div>
          <img src={useBaseUrl('/img/demo.gif')} alt="Product Screenshot" class="screenshot" />
          <div class="hero__cta">
            <Link to="/quick-start/" class="button button--lg button--primary"><p>Try the Quick Start</p></Link>
            <Link to="/introduction" class="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
          </div>
          <h3>Frustrated using Terraform the <i>old fashion way</i>? <strong class="underline">There's a smarter option.</strong></h3>
        </header>
        <main>
          <h2 class="section">Simplify complex architectures with <strong class="atmos__text">DRY configuration</strong></h2>
          <section class="alternate-section section--image-right">
            <Screengrab title="Start your Project" command="# here's an example of what your folder structure will like..." slug="demo-stacks/start-your-project" />
            <div class="section__description">
              <h2>Start Your Project</h2>
              <p>Create a solid foundation with a well-structured folder layout, embracing best practices and conventions for a consistently organized project.</p>
              <Link to="/core-concepts/stacks/catalogs" class="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
          <section class="alternate-section section--image-left">
            <Screengrab title="Write your Components" command="# Then write your terraform root modules..." slug="demo-stacks/write-your-components" />
            <div class="section__description">
              <h2>Write your Components</h2>
              <p>Use your existing Terraform root modules or create new ones. Component libraries make sharing easy. 
                 Use vendoring to pull down remote dependencies.</p>
              <Link to="/core-concepts/components" class="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
          <section class="alternate-section section--image-right">
            <Screengrab title="Define your Stacks" command="# Configure your stacks using YAML... easily import and inherit settings" slug="demo-stacks/define-your-stacks" />
            <div class="section__description">
              <h2>Define your Stacks</h2>
              <p>Configure your environments—development, staging, production—each tailored to different stages of the lifecycle, ensuring smooth transitions and robust deployment strategies.
                 Inherit from a common baseline to keep it DRY.</p>
              <Link to="/core-concepts/stacks" class="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
          <section class="alternate-section section--image-left">
            <Screengrab title="Atmos Stacks" command="# Deploy your stacks with the console UI or using GitHub Actions" slug="demo-stacks/deploy" />
            <div class="section__description">
              <h2>Deploy 🚀</h2>
              <p>Execute deployments with precision using Terraform's plan and apply commands, fully integrated with native GitOps workflows through GitHub Actions for seamless automation.</p>
              <Link to="/cli/commands/terraform/usage" class="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
            </div>
          </section>
        </main>
        <footer>
            <h2>What are you waiting for? <strong class="atmos__text">It's FREE and Open Source</strong></h2>
            <h3><strong class="underline">Your team can succeed</strong> with Terraform today.</h3>
            <section class="cta-section">
              <Link to="/quick-start/" class="button button--lg button--primary"><p>Try the Quick Start</p></Link>
            </section>
        </footer>
      </Layout>
    </div>
  );
}

export default Home;
