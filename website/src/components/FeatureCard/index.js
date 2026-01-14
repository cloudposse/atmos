import React, { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiArrowDownSLine, RiCheckLine } from 'react-icons/ri';
import './index.css';

const FeatureCard = ({
  icon,
  title,
  tagline,
  description,
  benefits = [],
  docLink,
  index = 0
}) => {
  const [isExpanded, setIsExpanded] = useState(false);

  return (
    <motion.div
      className="feature-card-wrapper"
      initial={{ opacity: 0, y: 30 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-50px" }}
      transition={{ duration: 0.5, delay: index * 0.1, ease: "easeOut" }}
    >
      <div
        className={`feature-card ${isExpanded ? 'feature-card--expanded' : ''}`}
        onClick={() => setIsExpanded(!isExpanded)}
        role="button"
        tabIndex={0}
        aria-expanded={isExpanded}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            setIsExpanded(!isExpanded);
          }
        }}
      >
        <div className="feature-card__header">
          <div className="feature-card__icon">{icon}</div>
          <div className="feature-card__title-group">
            <h3 className="feature-card__title">{title}</h3>
            <p className="feature-card__tagline">{tagline}</p>
          </div>
          <motion.div
            className="feature-card__expand-icon"
            animate={{ rotate: isExpanded ? 180 : 0 }}
            transition={{ duration: 0.2 }}
          >
            <RiArrowDownSLine />
          </motion.div>
        </div>

        <AnimatePresence>
          {isExpanded && (
            <motion.div
              className="feature-card__content"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.3, ease: "easeInOut" }}
            >
              <p className="feature-card__description">{description}</p>

              {benefits.length > 0 && (
                <div className="feature-card__benefits">
                  <span className="feature-card__benefits-label">Why it matters:</span>
                  <ul className="feature-card__benefits-list">
                    {benefits.map((benefit, i) => (
                      <li key={i}>
                        <RiCheckLine className="feature-card__check-icon" />
                        <span>{benefit}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              {docLink && (
                <Link
                  to={docLink}
                  className="feature-card__link"
                  onClick={(e) => e.stopPropagation()}
                >
                  Learn more →
                </Link>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </motion.div>
  );
};

export default FeatureCard;
