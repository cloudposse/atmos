import React, { useEffect } from 'react';
import { useLocation } from '@docusaurus/router';

export default function DefinitionLinkHandler() {
  const location = useLocation();

  useEffect(() => {
    // Select all <dt> elements on the current page
    const dtElements = document.querySelectorAll('dt');

    dtElements.forEach((dt) => {
      // Generate a slug based on the content of the <dt> tag
      const slug = dt.textContent.trim().toLowerCase().replace(/\s+/g, '-').replace(/[^\w-]/g, '');

      // Check if the dt element already has an ID
      if (!dt.id) {
        dt.id = slug;

        // Create a hash-link anchor
        const anchor = document.createElement('a');
        anchor.href = `#${slug}`;
        anchor.className = 'hash-link';
        anchor.setAttribute('aria-label', `Direct link to ${dt.textContent}`);
        anchor.setAttribute('title', `Direct link to ${dt.textContent}`);
        anchor.innerHTML = '&ZeroWidthSpace;';

        // Append the anchor to the dt element
        dt.appendChild(anchor);
      }
    });

    // Handle scrolling to hash anchor after DOM is ready
    if (location.hash) {
      // Use requestAnimationFrame to ensure DOM is fully updated
      requestAnimationFrame(() => {
        const id = location.hash.slice(1); // Remove the '#'
        const element = document.getElementById(id);
        if (element) {
          element.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
      });
    }
  }, [location.pathname, location.hash]); // Re-run on route or hash changes

  return null; // No UI to render
}
