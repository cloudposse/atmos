import React, { useState, useEffect, useId } from 'react';
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
    _glossaryPromise?: Promise<GlossaryData>;
  }
}

/**
 * Term component that displays a link to a glossary term with hover tooltip.
 *
 * This is a custom implementation inspired by the functionality of
 * @grnet/docusaurus-terminology (https://github.com/grnet/docusaurus-terminology)
 * but written from scratch for better control and to avoid dependency vulnerabilities.
 *
 * Original plugin: BSD-2-Clause License, Copyright (c) National Infrastructures
 * for Research and Technology (GRNET).
 */
const Term: React.FC<TermProps> = ({ termId, children }) => {
  const [glossary, setGlossary] = useState<GlossaryData | null>(null);
  const [showTooltip, setShowTooltip] = useState(false);
  const { withBaseUrl } = useBaseUrlUtils();
  const tooltipId = useId();

  useEffect(() => {
    // Load glossary data from JSON file.
    if (typeof window !== 'undefined') {
      if (window._cachedGlossary) {
        setGlossary(window._cachedGlossary);
      } else {
        const glossaryUrl = withBaseUrl('/glossary.json');
        if (!window._glossaryPromise) {
          window._glossaryPromise = fetch(glossaryUrl)
            .then((res) => {
              if (!res.ok) throw new Error(`HTTP ${res.status}`);
              return res.json();
            })
            .then((data: GlossaryData) => {
              window._cachedGlossary = data;
              return data;
            });
        }
        window._glossaryPromise
          .then((data) => setGlossary(data))
          .catch((err) => {
            console.error('[Term] Failed to load glossary:', err);
            const emptyGlossary: GlossaryData = {};
            window._cachedGlossary = emptyGlossary;
            setGlossary(emptyGlossary);
          });
      }
    }
  }, [withBaseUrl]);

  // Try multiple normalized keys to handle encoding, casing, and custom slugs.
  let termData = null;
  if (glossary) {
    // Try decoded URI first (handles %20, %2F, etc.).
    // Wrap in try/catch to handle malformed percent-escape sequences.
    try {
      const decodedTermId = decodeURIComponent(termId);
      termData = glossary[decodedTermId];
    } catch {
      // Skip decoded lookup if percent-escape is malformed (e.g., %E0%A4%A or %%).
      // Continue to lowercase normalization and original termId fallbacks.
    }

    // Try lowercase normalized form.
    if (!termData) {
      const normalizedLower = termId.toLowerCase();
      termData = glossary[normalizedLower];
    }

    // Fallback to original termId.
    if (!termData) {
      termData = glossary[termId];
    }
  }

  if (!termData) {
    // Fallback: render as plain link if term not found.
    // Check if this is an external URL (http://, https://, mailto:, protocol-relative //, etc.).
    const isExternalUrl = /^[a-z]+:/i.test(termId) || termId.startsWith('//');

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
      tabIndex={0}
      onMouseEnter={() => setShowTooltip(true)}
      onMouseLeave={() => setShowTooltip(false)}
      onFocus={() => setShowTooltip(true)}
      onBlur={() => setShowTooltip(false)}
    >
      <Link to={termUrl} className="term-link" aria-describedby={hoverText ? tooltipId : undefined}>
        {children}
      </Link>
      {showTooltip && hoverText && (
        <span className="term-tooltip" role="tooltip" id={tooltipId}>
          {hoverText}
        </span>
      )}
    </span>
  );
};

export default Term;
