import React from 'react';
import Layout from '@theme/Layout';
import CastPlayer from '../components/CastPlayer';
import styles from './cast-demo.module.css';

export default function CastDemoPage() {
  return (
    <Layout title="Cast Demo" description="Atmos asciicast player demo">
      <main className={styles.page}>
        <section className={styles.hero}>
          <CastPlayer
            src="/casts/demo/casts/fixtures/basic/list-stacks.cast"
            title="Config in, infrastructure out"
            chrome
            controls
            scrubber
            autoplay
            loopDelay={5}
          />
        </section>
        <section className={styles.gallery} aria-label="Cast gallery">
          <div className={styles.galleryItem}>
            <CastPlayer
              src="/casts/demo/casts/fixtures/basic/list-stacks.cast"
              title="List stacks"
              chrome
              autoplay
              loopDelay={5}
              thumbnail
            />
          </div>
          <div className={styles.galleryItem}>
            <CastPlayer
              src="/casts/demo/casts/fixtures/native-terraform/plan.cast"
              title="Terraform plan"
              chrome
              autoplay
              loopDelay={5}
              thumbnail
            />
          </div>
          <div className={styles.galleryItem}>
            <CastPlayer
              src="/casts/demo/casts/fixtures/demo-vendoring/pull.cast"
              command="atmos vendor pull --everything"
              title="Vendor pull"
              chrome
              autoplay
              loopDelay={5}
              thumbnail
            />
          </div>
        </section>
      </main>
    </Layout>
  );
}
