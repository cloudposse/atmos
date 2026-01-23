import React, { useEffect, useCallback } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { RiCloseLine, RiExternalLinkLine, RiCheckLine, RiCloseFill } from 'react-icons/ri';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import CodeBlock from '@theme/CodeBlock';
import Link from '@docusaurus/Link';
import { featureLinks } from '@site/src/data/tools';
import './index.css';

/**
 * Markdown components for rendering tool details and comparison content.
 */
const markdownComponents = {
  code({ className, children, ...props }: { className?: string; children?: React.ReactNode }) {
    const match = /language-(\w+)/.exec(className || '');
    const isInline = !match;
    return isInline ? (
      <code className={className} {...props}>
        {children}
      </code>
    ) : (
      <CodeBlock language={match[1]}>{String(children).replace(/\n$/, '')}</CodeBlock>
    );
  },
};

export interface FeatureComparison {
  feature: string;
  atmos: boolean;
  tool: boolean;
}

export interface Tool {
  id: string;
  name: string;
  url: string;
  description: string;
  category: string;
  relationship: 'supported' | 'wrapper' | 'delivery' | 'commands' | 'workflows' | 'ecosystem' | 'inspiration';
  details: string;
  atmosComparison?: string;
  featureComparison?: FeatureComparison[];
}

interface ToolDrawerProps {
  tool: Tool | null;
  isOpen: boolean;
  onClose: () => void;
}

const relationshipLabels: Record<Tool['relationship'], string> = {
  supported: 'Supported by Atmos',
  wrapper: 'Terraform Wrapper',
  delivery: 'Delivery Tool',
  commands: 'Alternative to Custom Commands',
  workflows: 'Alternative to Workflows',
  ecosystem: 'Ecosystem Tool',
  inspiration: 'Conceptual Inspiration',
};

const relationshipColors: Record<Tool['relationship'], string> = {
  supported: 'badge--supported',
  wrapper: 'badge--wrapper',
  delivery: 'badge--delivery',
  commands: 'badge--commands',
  workflows: 'badge--workflows',
  ecosystem: 'badge--ecosystem',
  inspiration: 'badge--inspiration',
};

const ToolDrawer: React.FC<ToolDrawerProps> = ({ tool, isOpen, onClose }) => {
  const handleEscape = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose();
    }
  }, [onClose]);

  useEffect(() => {
    if (isOpen) {
      document.addEventListener('keydown', handleEscape);
      document.body.style.overflow = 'hidden';
    }
    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.body.style.overflow = '';
    };
  }, [isOpen, handleEscape]);

  return (
    <AnimatePresence>
      {isOpen && tool && (
        <>
          <motion.div
            className="tool-drawer-backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.2 }}
            onClick={onClose}
            aria-hidden="true"
          />
          <motion.aside
            className="tool-drawer"
            initial={{ x: '100%' }}
            animate={{ x: 0 }}
            exit={{ x: '100%' }}
            transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            role="dialog"
            aria-modal="true"
            aria-labelledby="drawer-title"
          >
            <div className="tool-drawer__header">
              <button
                className="tool-drawer__close"
                onClick={onClose}
                aria-label="Close drawer"
              >
                <RiCloseLine />
              </button>
            </div>

            <div className="tool-drawer__content">
              <h2 id="drawer-title" className="tool-drawer__title">
                {tool.name}
              </h2>

              <div className="tool-drawer__meta">
                <span className={`tool-drawer__badge ${relationshipColors[tool.relationship]}`}>
                  {relationshipLabels[tool.relationship]}
                </span>
                <span className="tool-drawer__category">{tool.category}</span>
              </div>

              <section className="tool-drawer__section">
                <h3>About</h3>
                <p className="tool-drawer__description">{tool.description}</p>
                {tool.details && (
                  <div className="tool-drawer__details">
                    <Markdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                      {tool.details}
                    </Markdown>
                  </div>
                )}
              </section>

              {tool.atmosComparison && (
                <section className="tool-drawer__section">
                  <h3>How It Relates to Atmos</h3>
                  <div className="tool-drawer__comparison">
                    <Markdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                      {tool.atmosComparison}
                    </Markdown>
                  </div>
                </section>
              )}

              {tool.featureComparison && tool.featureComparison.length > 0 && (
                <section className="tool-drawer__section">
                  <h3>Feature Comparison</h3>
                  <table className="tool-drawer__feature-table">
                    <thead>
                      <tr>
                        <th>Feature</th>
                        <th className="tool-drawer__atmos-col">Atmos</th>
                        <th>{tool.relationship === 'supported' ? 'Native Only' : tool.name}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {tool.featureComparison.map((item) => (
                        <tr key={item.feature}>
                          <td>
                            {featureLinks[item.feature] ? (
                              <Link to={featureLinks[item.feature]}>{item.feature}</Link>
                            ) : (
                              item.feature
                            )}
                          </td>
                          <td>
                            {item.atmos ? (
                              <span className="tool-drawer__check" role="img" aria-label="Supported by Atmos">
                                <RiCheckLine />
                              </span>
                            ) : (
                              <span className="tool-drawer__cross" role="img" aria-label="Not supported by Atmos">
                                <RiCloseFill />
                              </span>
                            )}
                          </td>
                          <td>
                            {item.tool ? (
                              <span className="tool-drawer__check" role="img" aria-label={`Supported by ${tool.name}`}>
                                <RiCheckLine />
                              </span>
                            ) : (
                              <span className="tool-drawer__cross" role="img" aria-label={`Not supported by ${tool.name}`}>
                                <RiCloseFill />
                              </span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </section>
              )}

              <a
                href={tool.url}
                target="_blank"
                rel="noopener noreferrer"
                className="tool-drawer__cta"
              >
                View Project <RiExternalLinkLine />
              </a>
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
};

export default ToolDrawer;
