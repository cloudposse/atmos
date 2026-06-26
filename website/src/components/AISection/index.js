import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiStackLine, RiGraduationCapLine, RiTerminalBoxLine } from 'react-icons/ri';
import './styles.css';

function AIBadge() {
  return (
    <Link to="/ai" className="ai-badge-wrapper">
      <div className="ai-badge-glow" />
      <div className="ai-badge">
        <span className="ai-badge-text">AI</span>
        <span className="ai-badge-subtitle">Native</span>
      </div>
    </Link>
  );
}

// Each card links to the doc section that proves its claim.
const MotionLink = motion(Link);

const capabilities = [
  {
    icon: RiStackLine,
    title: 'Declarative and consistent',
    desc: 'Every environment is the same stack configuration, resolved the same way. Agents reason about one predictable model instead of a dozen bespoke tools and scripts.',
    link: '/stacks',
    delay: 0,
  },
  {
    icon: RiGraduationCapLine,
    title: 'Skills that teach the agent',
    desc: '21+ pre-built skills hand agents exactly what they need to know about your stacks, components, and workflows — and you can publish your own.',
    link: '/ai/agent-skills',
    delay: 0.1,
  },
  {
    icon: RiTerminalBoxLine,
    title: 'A self-documenting CLI',
    desc: 'Every command ships usage examples and actionable hints, and Atmos exposes itself over MCP. Agents learn the system by running it.',
    link: '/ai/mcp-server',
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
            <span className="ai-section-eyebrow">Built for agents</span>
            <h2>Infrastructure agents can actually reason about</h2>
            <p>
              Frontend teams move fast because the framework is consistent and
              predictable. Atmos brings that to infrastructure. Everything is
              declarative — the same stack configuration, the same commands,
              everywhere. Skills teach agents your domain, and every command
              documents itself. So agents don't string together 25 tools and
              pray — they operate one provable, end-to-end framework.
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
            <MotionLink
              key={i}
              to={cap.link}
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
            </MotionLink>
          ))}
        </div>
      </div>
    </section>
  );
}

export default AISection;
