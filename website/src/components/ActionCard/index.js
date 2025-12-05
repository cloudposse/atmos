import React from 'react'
import Link from '@docusaurus/Link'
import PrimaryCTA from '@site/src/components/PrimaryCTA'
import SecondaryCTA from '@site/src/components/SecondaryCTA'
import './index.css'

const ActionCard = ({ title = "Ready to learn this topic?",
                      ctaText,
                      ctaLink,
                      primaryCtaText,
                      primaryCtaLink,
                      secondaryCtaText,
                      secondaryCtaLink,
                      children }) => {
  // Determine primary CTA text and link
  const primaryText = ctaText || primaryCtaText;
  const primaryLink = ctaLink || primaryCtaLink;

  return (
    <div className="action-card">
      <h2>{title}</h2>
      <div>{children}</div>
      <div className="action-card__cta-group">
        {primaryLink && (
          <PrimaryCTA to={primaryLink}>
            {primaryText || "Read More"}
          </PrimaryCTA>
        )}
        {secondaryCtaLink && (
          <SecondaryCTA to={secondaryCtaLink}>
            {secondaryCtaText || "Read More"}
          </SecondaryCTA>
        )}
      </div>
    </div>
  );
};

export default ActionCard;
