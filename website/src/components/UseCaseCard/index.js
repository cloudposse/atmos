import React, { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiArrowDownSLine, RiArrowRightLine } from 'react-icons/ri';
import './index.css';

const UseCaseCard = ({
  icon,
  title,
  description,
  highlights = [],
  enabledBy = [],
  docLinks = [],
  index = 0
}) => {
  const [isExpanded, setIsExpanded] = useState(false);

  return (
    <motion.div
      className="use-case-card-wrapper"
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-50px" }}
      transition={{ duration: 0.5, delay: index * 0.1, ease: "easeOut" }}
    >
      <div
        className={`use-case-card ${isExpanded ? 'use-case-card--expanded' : ''}`}
        onClick={() => setIsExpanded(!isExpanded)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            setIsExpanded(!isExpanded);
          }
        }}
      >
        <div className="use-case-card__header">
          <div className="use-case-card__icon">{icon}</div>
          <h3 className="use-case-card__title">{title}</h3>
          <motion.div
            className="use-case-card__expand-icon"
            animate={{ rotate: isExpanded ? 180 : 0 }}
            transition={{ duration: 0.2 }}
          >
            <RiArrowDownSLine />
          </motion.div>
        </div>

        <p className="use-case-card__description">{description}</p>

        <AnimatePresence>
          {isExpanded && (
            <motion.div
              className="use-case-card__content"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.3, ease: "easeInOut" }}
            >
              {highlights.length > 0 && (
                <div className="use-case-card__highlights">
                  <span className="use-case-card__section-label">Key Benefits:</span>
                  <ul className="use-case-card__highlights-list">
                    {highlights.map((highlight, i) => (
                      <li key={i}>
                        <RiArrowRightLine className="use-case-card__bullet-icon" />
                        <span>{highlight}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {enabledBy.length > 0 && (
                <div className="use-case-card__enabled-by">
                  <span className="use-case-card__section-label">Enabled By:</span>
                  <div className="use-case-card__features-list">
                    {enabledBy.map((feature, i) => (
                      <span key={i} className="use-case-card__feature-tag">
                        {feature}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              {docLinks.length > 0 && (
                <div className="use-case-card__links">
                  {docLinks.map((link, i) => (
                    <Link
                      key={i}
                      to={link.to}
                      className="use-case-card__link"
                      onClick={(e) => e.stopPropagation()}
                    >
                      {link.label} →
                    </Link>
                  ))}
                </div>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </motion.div>
  );
};

export default UseCaseCard;
