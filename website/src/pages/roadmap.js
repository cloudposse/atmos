import React from 'react';
import Layout from '@theme/Layout';
import Roadmap from '@site/src/components/Roadmap';

export default function RoadmapPage() {
  return (
    <Layout
      title="Roadmap"
      description="Atmos development roadmap - see what we've shipped, what's in progress, and what's planned"
    >
      <Roadmap />
    </Layout>
  );
}
