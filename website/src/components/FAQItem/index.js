import React, { useState } from 'react';
import useBrokenLinks from '@docusaurus/useBrokenLinks';
import { motion, AnimatePresence } from 'framer-motion';
import './index.css';

const LegacyFAQItem = ({ question, children }) => {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <div className="faq-item">
      <button
        className="faq-question"
        onClick={() => setIsOpen(!isOpen)}
        aria-expanded={isOpen}
      >
        <span>{question}</span>
        <motion.span
          className="faq-icon"
          animate={{ rotate: isOpen ? 45 : 0 }}
          transition={{ type: "spring", stiffness: 300, damping: 20 }}
        >
          +
        </motion.span>
      </button>
      <AnimatePresence initial={false}>
        {isOpen && (
          <motion.div
            className="faq-answer"
            initial={{ height: 0, opacity: 0 }}
            animate={{
              height: "auto",
              opacity: 1,
              transition: {
                height: { type: "spring", stiffness: 100, damping: 15 },
                opacity: { duration: 0.2, delay: 0.1 }
              }
            }}
            exit={{
              height: 0,
              opacity: 0,
              transition: {
                height: { type: "spring", stiffness: 100, damping: 15 },
                opacity: { duration: 0.15 }
              }
            }}
          >
            <div className="faq-answer-content">
              {children}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
};

export default function FAQItem({ variant, id, question, children }) {
  const brokenLinks = useBrokenLinks();
  if (variant !== 'minimal') {
    return <LegacyFAQItem question={question}>{children}</LegacyFAQItem>;
  }

  brokenLinks.collectAnchor(id);
  return (
    <details id={id} className="faq-disclosure" data-faq-item>
      <summary>
        <span data-faq-question>{question}</span>
        <span className="faq-disclosure-icon" aria-hidden="true" />
      </summary>
      <div className="faq-disclosure-answer">
        <div data-faq-body>{children}</div>
        <a className="faq-permalink" href={`#${id}`} aria-label={`Link to: ${question}`}>
          Link to this answer <span aria-hidden="true">↗</span>
        </a>
      </div>
    </details>
  );
}
