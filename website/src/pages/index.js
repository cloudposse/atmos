import React from 'react';
import Layout from '@theme/Layout';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl'

function Home() {
  const context = useDocusaurusContext();
  const {siteConfig = {}} = context;

  return (
    <div class="landing-page">
      <Layout
        title={`Hello from ${siteConfig.title}`}
        description="Description will go into a meta tag in <head />">
        
        <header class="hero hero--full-height">
          <div class="intro">
            <h1>Manage Environments Easily in Terraform</h1>
            <h2>Simplify complex architectures with DRY configuration with Atmos</h2>
          </div>
          <img src={useBaseUrl('/img/demo.gif')} alt="Product Screenshot" class="screenshot" />
          <div class="hero__cta">
            <h3>Are you tired of doing Terraform the old way? <strong class="underline">There's a better way.</strong></h3>
            <Link to="/quick-start/" class="button button--lg button--primary"><p>Try the Quick Start</p></Link>
            <Link to="/introduction" class="button button--lg button--outline button--primary ml20"><p>Learn More</p></Link>
          </div>
        </header>
        <main>
          <section class="alternate-section section--image-right">
            <img src={useBaseUrl('/img/cli/atmos/atmos-cli-command-1.png')} alt="Screenshot 1" class="screenshot" />
            <div class="section__description">
              <h2>Start Your Project</h2>
              <p>Section Description 1</p>
              <Link to="/" class="button button--lg button--outline button--primary ml20"><p>Read Related Docs</p></Link>
            </div>
          </section>
          <section class="alternate-section section--image-left">
            <img src={useBaseUrl('/img/cli/atmos/atmos-cli-command-1.png')} alt="Screenshot 2" class="screenshot" />
            <div class="section__description">
              <h2>Write your Components</h2>
              <p>Section Description 2</p>
              <Link to="/" class="button button--lg button--outline button--primary ml20"><p>Read Related Docs</p></Link>
            </div>
          </section>
          <section class="alternate-section section--image-right">
            <img src={useBaseUrl('/img/cli/atmos/atmos-cli-command-1.png')} alt="Screenshot 3" class="screenshot" />
            <div class="section__description">
              <h2>Define your Stacks</h2>
              <p>Section Description 3</p>
              <Link to="/" class="button button--lg button--outline button--primary ml20"><p>Read Related Docs</p></Link>
            </div>
          </section>
          <section class="alternate-section section--image-left">
            <img src={useBaseUrl('/img/cli/atmos/atmos-cli-command-1.png')} alt="Screenshot 4" class="screenshot" />
            <div class="section__description">
              <h2>Deploy 🚀</h2>
              <p>Deploy</p>
              <Link to="/" class="button button--lg button--outline button--primary ml20"><p>Read Related Docs</p></Link>
            </div>
          </section>
          <section class="cta-section">
            <Link to="/" class="button button--lg button--primary"><p>Try the Quick Start</p></Link>
          </section>
        </main>
      </Layout>
    </div>
  );
}

export default Home;
