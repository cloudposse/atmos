import React from 'react';
import { motion } from 'framer-motion';
import { RiArrowRightLine } from 'react-icons/ri';
import type { Tool } from '../ToolDrawer';
import './index.css';

interface ToolCardProps {
  tool: Tool;
  onClick: (tool: Tool) => void;
  index?: number;
}

const relationshipLabels: Record<Tool['relationship'], string> = {
  supported: 'Supported',
  wrapper: 'Wrapper',
  delivery: 'Delivery',
  commands: 'Commands Alt',
  workflows: 'Workflows Alt',
  ecosystem: 'Ecosystem',
  inspiration: 'Inspiration',
};

const ToolCard: React.FC<ToolCardProps> = ({ tool, onClick, index = 0 }) => {
  return (
    <motion.button
      className="tool-card"
      onClick={() => onClick(tool)}
      initial={{ opacity: 0, y: 20 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-30px" }}
      transition={{ duration: 0.3, delay: index * 0.05 }}
      whileHover={{ scale: 1.02 }}
      whileTap={{ scale: 0.98 }}
    >
      <div className="tool-card__content">
        <div className="tool-card__header">
          <h3 className="tool-card__name">{tool.name}</h3>
          <span className={`tool-card__badge tool-card__badge--${tool.relationship}`}>
            {relationshipLabels[tool.relationship]}
          </span>
        </div>
        <p className="tool-card__description">{tool.description}</p>
      </div>
      <div className="tool-card__arrow">
        <RiArrowRightLine />
      </div>
    </motion.button>
  );
};

export default ToolCard;
