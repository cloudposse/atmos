import React from 'react';
import Layout from '@theme/Layout';
import DemoGallery from '../components/DemoGallery';

export default function DemosPage(): JSX.Element {
  return (
    <Layout
      title="Demos"
      description="Watch Atmos in action with interactive terminal demos"
    >
      <main>
        <div style={{ padding: '2rem 0' }}>
          <div style={{ maxWidth: '1400px', margin: '0 auto', padding: '0 1rem' }}>
            <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
              <h1 className="demos-title" style={{ fontSize: '2.5rem', marginBottom: '1rem' }}>
                Atmos Demos
              </h1>
              <p style={{ fontSize: '1.25rem', color: 'var(--ifm-color-emphasis-600)', maxWidth: '600px', margin: '0 auto' }}>
                Watch Atmos in action. Explore terminal recordings that showcase
                key features and workflows.
              </p>
            </div>
          </div>
          <DemoGallery />
        </div>
      </main>
    </Layout>
  );
}
