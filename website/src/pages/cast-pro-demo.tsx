import React from 'react';
import Layout from '@theme/Layout';

import CastProDownload from '../components/CastProDownload';
import CastProEmbed from '../components/CastProEmbed';

import styles from './cast-pro-demo.module.css';

// A real path committed to cloudposse/atmos so the Atmos Pro rendering
// service (which fetches the .cast source from GitHub) has something to render.
const DEMO_PATH = 'website/static/casts/screengrabs/atmos-mcp-start--help.cast';

export default function CastProDemoPage() {
  return (
    <Layout title="Cast Pro Demo" description="Atmos Pro cast download/embed demo">
      <main className={styles.page}>
        <section className={styles.section}>
          <div className={styles.sectionHeader}>
            <h2>Download rendered artifacts</h2>
            <CastProDownload gitRef="main" path={DEMO_PATH} />
          </div>
          <p>
            Renders GIF/MP4/SVG/WEBM on demand via{' '}
            <code>https://atmos-pro.com/casts/cloudposse/atmos/main/{DEMO_PATH}.&#123;format&#125;</code>.
            Handles the render service&apos;s three response shapes: an already-rendered artifact
            (downloads immediately), a still-rendering one (polls on <code>Retry-After</code>, capped
            at ~60s), and a hard error (surfaces the JSON error message above).
          </p>
        </section>

        <section className={styles.section}>
          <h2>Embed the hosted player</h2>
          <CastProEmbed gitRef="main" path={DEMO_PATH} title="Atmos MCP start --help" />
        </section>
      </main>
    </Layout>
  );
}
