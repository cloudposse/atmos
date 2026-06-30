import React, { useEffect, useState } from 'react';
import { MotionConfig } from 'framer-motion';
import Layout from '@theme/Layout';
import Hero from '@site/src/components/landing/Hero';
import AISection from '@site/src/components/AISection';
import HowItWorks from '@site/src/components/landing/HowItWorks';
import EnvAsConfig from '@site/src/components/landing/EnvAsConfig';
import Batteries from '@site/src/components/landing/Batteries';
import Workloads from '@site/src/components/landing/Workloads';
import Extensibility from '@site/src/components/landing/Extensibility';
import FinalCta from '@site/src/components/landing/FinalCta';
import '../css/landing-page.css';
import '../css/landing-redesign.css';

function Home() {
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const updateScrolled = () => setScrolled(window.scrollY > 0);

    updateScrolled();
    window.addEventListener('scroll', updateScrolled, { passive: true });
    return () => window.removeEventListener('scroll', updateScrolled);
  }, []);

  return (
    <MotionConfig reducedMotion="user">
      <div className={`landing-page${scrolled ? ' landing-page--scrolled' : ''}`}>
        <Layout
          title="The runtime for infrastructure"
          description="Atmos is the runtime for infrastructure - one consistent way to build, ship, and run Terraform, Kubernetes, and containers, identically on your laptop and in CI. Auth, secrets, vendoring, and CI included."
        >
          <Hero />
          <AISection />
          <main>
            <HowItWorks />
            <EnvAsConfig />
            <Batteries />
            <Workloads />
            <Extensibility />
          </main>
          <FinalCta />
        </Layout>
      </div>
    </MotionConfig>
  );
}

export default Home;
