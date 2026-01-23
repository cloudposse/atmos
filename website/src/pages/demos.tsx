import React from 'react';
import Layout from '@theme/Layout';
import DemosAndExamples from '../components/DemosAndExamples';

export default function DemosPage(): JSX.Element {
  return (
    <Layout
      title="Demos & Examples"
      description="Watch Atmos in action with interactive terminal demos and explore complete example projects"
    >
      <main>
        <DemosAndExamples />
      </main>
    </Layout>
  );
}
