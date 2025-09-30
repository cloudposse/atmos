import React, { useState, useEffect } from 'react';
import Link from '@docusaurus/Link';
import { useBaseUrlUtils } from '@docusaurus/useBaseUrl';
import './Term.css';

interface TermMetadata {
  id: string;
  title: string;
  hoverText: string;
  slug: string;
}

interface TermData {
  metadata: TermMetadata;
  content: string;
}

interface GlossaryData {
  [key: string]: TermData;
}

interface TermProps {
  termId: string;
  children: React.ReactNode;
}

// Global cache for glossary data.
declare global {
  interface Window {
    _cachedGlossary?: GlossaryData;
  }
}

/**
 * Term component that displays a link to a glossary term with hover tooltip.
 * Replaces @grnet/docusaurus-term-preview functionality.
 */
const Term: React.FC<TermProps> = ({ termId, children }) => {
  const [glossary, setGlossary] = useState<GlossaryData | null>(null);
  const [showTooltip, setShowTooltip] = useState(false);
  const { withBaseUrl } = useBaseUrlUtils();

  useEffect(() => {
    // Load glossary data from JSON file.
    if (typeof window !== 'undefined') {
      if (window._cachedGlossary) {
        setGlossary(window._cachedGlossary);
      } else {
        const glossaryUrl = withBaseUrl('/glossary.json');
        fetch(glossaryUrl)
          .then((res) => res.json())
          .then((data) => {
            setGlossary(data);
            window._cachedGlossary = data;
          })
          .catch((err) => {
            console.error('[Term] Failed to load glossary:', err);
            // Set empty object to prevent repeated fetch attempts.
            const emptyGlossary = {};
            setGlossary(emptyGlossary);
            window._cachedGlossary = emptyGlossary;
          });
      }
    }
  }, [withBaseUrl]);

  const termData = glossary?.[termId];

  if (!termData) {
    // Fallback: render as plain link if term not found.
    // Check if this is an external URL (http://, https://, mailto:, or other protocols).
    const isExternalUrl = /^[a-z]+:/i.test(termId);

    if (isExternalUrl) {
      // For external URLs, use plain anchor tag with security attributes.
      return (
        <a href={termId} target="_blank" rel="noopener noreferrer">
          {children}
        </a>
      );
    }

    // For internal routes, use Docusaurus Link.
    return <Link to={termId}>{children}</Link>;
  }

  const termUrl = withBaseUrl(termData.metadata.slug);
  const hoverText = termData.metadata.hoverText || termData.metadata.title;

  return (
    <span
      className="term-wrapper"
      onMouseEnter={() => setShowTooltip(true)}
      onMouseLeave={() => setShowTooltip(false)}
    >
      <Link to={termUrl} className="term-link">
        {children}
      </Link>
      {showTooltip && hoverText && (
        <span className="term-tooltip" role="tooltip">
          {hoverText}
        </span>
      )}
    </span>
  );
};

export default Term;
