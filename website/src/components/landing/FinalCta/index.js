import React from 'react';
import PrimaryCTA from '@site/src/components/PrimaryCTA';
import SecondaryCTA from '@site/src/components/SecondaryCTA';

// Closing call-to-action. The site-wide Docusaurus theme footer renders below
// this automatically — we do NOT add a second footer here.
function FinalCta() {
  return (
    <section className="lp-finalcta">
      <h2>One runtime for everything you ship.</h2>
      <p>Free and open source. Run your first stack in minutes.</p>
      <div className="lp-finalcta-cta">
        <PrimaryCTA to="/install">Install Atmos</PrimaryCTA>
        <SecondaryCTA to="/quick-start">Quick Start</SecondaryCTA>
      </div>
    </section>
  );
}

export default FinalCta;
