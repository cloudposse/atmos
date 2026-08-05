import React from 'react';
import { motion } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiStackLine, RiGraduationCapLine, RiPlugLine } from 'react-icons/ri';
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
    title: 'Declared, not scripted',
    desc: 'The tools, workflows, dependencies, and validation — everything you used to script together — is declared and wired end to end. Agents drive one complete system, not a pile of glue scripts.',
    link: '/stacks',
    delay: 0,
  },
  {
    icon: RiGraduationCapLine,
    title: 'Agent Skills',
    desc: '22 portable skills in the open Agent Skills format hand agents exactly what they need about your stacks, components, and workflows — working across Claude Code, Cursor, Gemini, and Copilot. Publish your own.',
    link: '/ai/agent-skills',
    delay: 0.1,
  },
  {
    icon: RiPlugLine,
    title: 'MCP Server',
    desc: 'Atmos exposes itself over the Model Context Protocol, so any MCP client — Claude Code, Cursor, VS Code — can query and drive your infrastructure as native tools. No custom integration.',
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
