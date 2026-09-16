import React from 'react';
import Layout from '@theme/Layout';
import ProductUpdatesHeader from '@site/src/components/ProductUpdatesHeader';
import Roadmap from '@site/src/components/Roadmap';

/** Provide roadmap page metadata and the shared navigation above roadmap content. */
export default function RoadmapPage() {
  return (
    <Layout
      title="Roadmap"
      description="Atmos development roadmap - see what we've shipped, what's in progress, and what's planned"
    >
      <ProductUpdatesHeader activeView="roadmap" />
      <Roadmap />
    </Layout>
  );
}
