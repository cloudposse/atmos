import React, { useEffect, useId, useRef, useState } from 'react';
import useBrokenLinks from '@docusaurus/useBrokenLinks';
import { createFAQController } from './search.mjs';
import './styles.css';

export function FAQSection({ id, title, children }) {
  const brokenLinks = useBrokenLinks();
  brokenLinks.collectAnchor(id);
  brokenLinks.collectAnchor(`${id}-heading`);
  return (
    <section id={id} className="faq-section" data-faq-section aria-labelledby={`${id}-heading`}>
      <h2 id={`${id}-heading`}>{title}</h2>
      {children}
    </section>
  );
}

export default function FAQ({ children }) {
  const root = useRef(null);
  const input = useRef(null);
  const controller = useRef(null);
  const inputId = useId();
  const [query, setQuery] = useState('');
  const [count, setCount] = useState(0);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    controller.current = createFAQController(root.current);
    setCount(controller.current.total);
    setReady(true);
    let frame;
    function revealFragment(hash) {
      const target = controller.current.reveal(hash);
      if (!target) return;
      setQuery('');
      setCount(controller.current.total);
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        target.scrollIntoView({ block: 'start', behavior: 'instant' });
      });
    }
    const onHashChange = () => revealFragment(window.location.hash);
    // A link to the current fragment does not fire hashchange.
    const onClick = (event) => {
      const link = event.target.closest('a[href^="#"]');
      if (link && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) {
        revealFragment(link.getAttribute('href'));
      }
    };
    const element = root.current;
    element.addEventListener('click', onClick);
    window.addEventListener('hashchange', onHashChange);
    onHashChange();
    return () => {
      cancelAnimationFrame(frame);
      element.removeEventListener('click', onClick);
      window.removeEventListener('hashchange', onHashChange);
    };
  }, []);

  function search(value) {
    setQuery(value);
    setCount(controller.current.search(value));
  }

  return (
    <div className="faq-explorer" ref={root}>
      <div className="faq-search" hidden={!ready}>
        <label htmlFor={inputId}>Find an answer</label>
        <div className="faq-search-field">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
            <circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" />
          </svg>
          <input ref={input} id={inputId} type="search" value={query} onChange={(event) => search(event.target.value)} placeholder="Search questions and answers…" autoComplete="off" />
          {query && <button type="button" onClick={() => { search(''); input.current.focus(); }}>Clear</button>}
        </div>
        <p className="faq-search-status" role="status" aria-live="polite" aria-atomic="true">
          {query.trim() ? `${count} ${count === 1 ? 'answer' : 'answers'} found` : ''}
        </p>
      </div>
      {children}
      {ready && count === 0 && (
        <div className="faq-empty">
          <h2>No answers found</h2>
          <p>Try a broader term like “Terraform”, “migration”, or “skills”.</p>
          <button type="button" onClick={() => { search(''); input.current.focus(); }}>Clear search</button>
        </div>
      )}
    </div>
  );
}
