import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiRobot2Line, RiStore2Line, RiPlugLine } from 'react-icons/ri';
import './styles.css';

function AIBadge() {
  return (
    <Link to="/ai" className="ai-badge-wrapper">
      <div className="ai-badge-glow" />
      <div className="ai-badge">
        <span className="ai-badge-text">AI</span>
        <span className="ai-badge-subtitle">Assisted</span>
      </div>
    </Link>
  );
}

const capabilities = [
  {
    icon: RiRobot2Line,
    title: 'Interactive AI Chat',
    desc: 'Chat with AI about your infrastructure. Get answers about stacks, troubleshoot issues, and automate workflows.',
    delay: 0,
  },
  {
    icon: RiStore2Line,
    title: 'Skills Marketplace',
    desc: '21+ pre-built skills for common infrastructure tasks. Install, configure, and extend with custom skills.',
    delay: 0.1,
  },
  {
    icon: RiPlugLine,
    title: 'MCP Server',
    desc: 'Expose Atmos as an MCP server for Claude Code, Cursor, and other AI-powered development tools.',
    delay: 0.2,
  },
];

function AISection() {
  return (
    <section className="ai-section">
      <div className="ai-section-inner">
        <motion.div
          className="ai-section-content"
          initial={{ opacity: 0, y: 30 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ amount: 0.3 }}
          transition={{ duration: 0.6, ease: 'easeOut' }}
        >
          <div className="ai-section-left">
            <span className="ai-section-eyebrow">AI-Powered</span>
            <h2>Intelligent Infrastructure Management</h2>
            <p>
              Atmos integrates AI directly into your workflow with multi-provider
              support, a skills marketplace, and MCP server integration. Ask
              questions, automate tasks, and troubleshoot issues naturally.
            </p>
            <div className="ai-section-cta">
              <Link to="/ai" className="button button--lg button--primary">Explore Atmos AI</Link>
            </div>
          </div>
          <div className="ai-section-right">
            <AIBadge />
          </div>
        </motion.div>

        <div className="ai-capabilities">
          {capabilities.map((cap, i) => (
            <motion.div
              key={i}
              className="ai-capability-card"
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ amount: 0.3 }}
              transition={{ duration: 0.5, delay: cap.delay, ease: 'easeOut' }}
            >
              <div className="ai-capability-icon">
                <cap.icon />
              </div>
              <h3>{cap.title}</h3>
              <p>{cap.desc}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  );
}

export default AISection;
